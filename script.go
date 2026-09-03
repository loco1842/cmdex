package main

import (
	"regexp"
	"sort"
	"strings"
)

var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// GenerateScript returns the script body trimmed of surrounding whitespace with a trailing newline.
// No shebang is prepended — the executor adds a platform-appropriate shebang at execution time.
func GenerateScript(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	return body + "\n"
}

// ParseScriptBody strips any shebang line (#!...) from the beginning of stored script content.
// Handles both old format (scripts stored with #!/bin/bash in the DB) and new format
// (scripts stored without a shebang) transparently.
func ParseScriptBody(scriptContent string) string {
	s := strings.TrimSpace(scriptContent)
	if strings.HasPrefix(s, "#!") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		} else {
			// Entire content is just a shebang line — no body
			return ""
		}
	}
	return strings.TrimSpace(s)
}

// ExtractTemplateVars returns unique variable names from {{var}} patterns, in order of first appearance.
func ExtractTemplateVars(text string) []string {
	matches := templateVarRe.FindAllStringSubmatch(text, -1)
	seen := map[string]int{}
	for _, m := range matches {
		name := m[1]
		if _, ok := seen[name]; !ok {
			seen[name] = len(seen)
		}
	}
	result := make([]string, len(seen))
	for name, idx := range seen {
		result[idx] = name
	}
	return result
}

// ReplaceTemplateVars replaces {{var}} placeholders when a value is present.
// Placeholders without a value remain unchanged here; resolveScript validates
// referenced required variables and applies evaluated defaults before dispatch.
// Values are intentionally inserted verbatim: command variables may represent
// shell fragments (for example, a generated flag list or a pipeline). Callers
// that need shell-word safety should include the appropriate quoting in the
// command template for its target shell.
func ReplaceTemplateVars(content string, values map[string]string) string {
	return templateVarRe.ReplaceAllStringFunc(content, func(match string) string {
		name := match[2 : len(match)-2] // strip {{ and }}
		if val, ok := values[name]; ok {
			return val
		}
		return match // leave unreplaced if no value
	})
}

// MergeDetectedVars merges auto-detected variable names with existing variable definitions.
// Detected vars not in existing list are added; existing vars not detected are kept (manual vars).
// Order: detected vars first (in detection order), then manual-only vars.
func MergeDetectedVars(detected []string, existing []VariableDefinition) []VariableDefinition {
	existingMap := map[string]VariableDefinition{}
	for _, v := range existing {
		existingMap[v.Name] = v
	}

	detectedSet := map[string]bool{}
	for _, name := range detected {
		detectedSet[name] = true
	}

	var result []VariableDefinition

	// First: detected vars in order
	for i, name := range detected {
		if v, ok := existingMap[name]; ok {
			v.SortOrder = i
			result = append(result, v)
		} else {
			result = append(result, VariableDefinition{
				Name:      name,
				SortOrder: i,
			})
		}
	}

	// Then: manual vars not in detected set, preserving relative order
	var manualVars []VariableDefinition
	for _, v := range existing {
		if !detectedSet[v.Name] {
			manualVars = append(manualVars, v)
		}
	}
	sort.SliceStable(manualVars, func(i, j int) bool {
		return manualVars[i].SortOrder < manualVars[j].SortOrder
	})
	for _, v := range manualVars {
		v.SortOrder = len(result)
		result = append(result, v)
	}

	return result
}
