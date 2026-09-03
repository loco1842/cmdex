package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ExecutionService handles running commands and execution history.
type ExecutionService struct{}

func (s *ExecutionService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// resolveWorkingDir determines the working directory for a command using the fallback chain:
// 1. Command-specific working dir for the current OS
// 2. Global default working dir for the current OS
// 3. OS home directory
// 4. Current working directory
// 5. OS temporary directory
// The function never returns an empty string.
func (s *ExecutionService) resolveWorkingDir(cmd Command) string {
	// Step 1: use per-command working directory if set
	if path := cmd.WorkingDir.GetCurrentOS(); path != "" {
		return path
	}

	// Step 2: fall back to global default working directory for current OS
	settings, err := db.GetSettings()
	if err != nil {
		fmt.Printf("resolveWorkingDir: GetSettings failed: %v\n", err)
	} else if settings.DefaultWorkingDir != nil {
		if path := settings.DefaultWorkingDir.GetCurrentOS(); path != "" {
			return path
		}
	}

	// Step 3: final fallback to user home directory
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		if err != nil {
			fmt.Printf("resolveWorkingDir: UserHomeDir failed: %v, trying Getwd\n", err)
		}
		cwd, err := os.Getwd()
		if err != nil || cwd == "" {
			if err != nil {
				fmt.Printf("resolveWorkingDir: Getwd failed: %v, falling back to TempDir\n", err)
			}
			return os.TempDir()
		}
		return cwd
	}
	return home
}

// GetVariables returns variable prompts for a command.
func (s *ExecutionService) GetVariables(commandID string) []VariablePrompt {
	cmd, err := db.GetCommand(commandID)
	if err != nil {
		return []VariablePrompt{}
	}

	if len(cmd.Variables) == 0 {
		return []VariablePrompt{}
	}

	evaluated := executor.EvalDefaults(cmd.Variables)

	var prompts []VariablePrompt
	for _, v := range cmd.Variables {
		p := VariablePrompt{
			Name:        v.Name,
			Description: v.Description,
			Example:     v.Example,
			DefaultExpr: v.Default,
		}
		if val, exists := evaluated[v.Name]; exists {
			p.DefaultValue = val
		}
		prompts = append(prompts, p)
	}
	if prompts == nil {
		prompts = []VariablePrompt{}
	}
	return prompts
}

// hasExplicitWorkingDir returns true when either the command or the global
// settings define a working directory for the current OS. If neither have
// one, the shell stays in its current (home) directory — no cd sandwich needed.
func (s *ExecutionService) hasExplicitWorkingDir(cmd Command) bool {
	if cmd.WorkingDir.GetCurrentOS() != "" {
		return true
	}
	settings, err := db.GetSettings()
	if err != nil {
		return false
	}
	if settings.DefaultWorkingDir != nil {
		return settings.DefaultWorkingDir.GetCurrentOS() != ""
	}
	return false
}

// resolveScript applies the same default and required-variable rules to every
// execution entry point. A caller may omit a variable when its definition has
// a literal or CEL default; variables with no usable value are rejected before
// anything is written to a PTY. Definitions that are not referenced by the
// script are intentionally ignored, since manual variable lists may contain
// metadata for a command's other scripts or future edits.
func (s *ExecutionService) resolveScript(cmd Command, variables map[string]string) (string, error) {
	resolved := make(map[string]string, len(cmd.Variables)+len(variables))
	eval := executor
	if eval == nil {
		eval = NewExecutor()
	}
	maps.Copy(resolved, eval.EvalDefaults(cmd.Variables))
	maps.Copy(resolved, variables)
	usedVariables := make(map[string]struct{})
	for _, name := range ExtractTemplateVars(cmd.ScriptContent) {
		usedVariables[name] = struct{}{}
	}
	for _, definition := range cmd.Variables {
		if _, used := usedVariables[definition.Name]; !used {
			continue
		}
		// An explicitly configured default makes the variable optional even
		// when evaluating that default produces an empty string (for example,
		// env("UNSET_NAME")). An empty default field is the legacy marker for
		// a required variable, which preserves validation for definitions that
		// have no fallback at all.
		if strings.TrimSpace(definition.Default) == "" && strings.TrimSpace(resolved[definition.Name]) == "" {
			return "", fmt.Errorf("missing required variable: %s", definition.Name)
		}
	}

	resolvedScript := ReplaceTemplateVars(cmd.ScriptContent, resolved)
	resolvedScript = stripShebang(resolvedScript)
	return strings.TrimRight(resolvedScript, "\n"), nil
}

// RunCommand resolves the command's template variables and writes the
// resulting command line directly to the active terminal session's PTY
// via TerminalService.Write. Output streams back through the session's
// pty-output event (handled by Terminal.tsx) and Ctrl+C interrupts are
// handled by the PTY's foreground process group.
func (s *ExecutionService) RunCommand(commandID string, variables map[string]string) ExecutionRecord {
	cmd, err := db.GetCommand(commandID)
	if err != nil {
		return ExecutionRecord{
			ID:       uuid.New().String(),
			Error:    err.Error(),
			ExitCode: -1,
		}
	}

	resolvedScript, err := s.resolveScript(cmd, variables)
	if err != nil {
		return ExecutionRecord{ID: uuid.New().String(), CommandID: commandID, Error: err.Error(), ExitCode: -1}
	}

	if terminalSvc == nil {
		return ExecutionRecord{
			ID:       uuid.New().String(),
			Error:    "terminal service not initialized",
			ExitCode: -1,
		}
	}
	session := terminalSvc.GetActiveSession()
	if session == nil {
		return ExecutionRecord{
			ID:       uuid.New().String(),
			Error:    "no active terminal session",
			ExitCode: -1,
		}
	}

	// Both the cd syntax and the key that submits the line depend on which
	// shell this session is actually running. CreateSession starts the PTY
	// eagerly, so ShellPath is normally already populated; the fallback
	// covers a session whose PTY hasn't started yet (Write would start it
	// lazily a moment later) and resolves to the same detectShell() call
	// startSessionLocked would make.
	shellPath := session.ShellPath
	if shellPath == "" {
		shellPath, _ = detectShell()
	}

	// "" means no cd prefix — hasExplicitWorkingDir is what decides, since
	// resolveWorkingDir always returns something (home/cwd/temp fallbacks).
	workingDir := ""
	if s.hasExplicitWorkingDir(cmd) {
		workingDir = s.resolveWorkingDir(cmd)
	}

	cmdLine := buildCommandLine(shellPath, resolvedScript, workingDir)

	if err := terminalSvc.Write(session.ID, cmdLine); err != nil {
		return ExecutionRecord{
			ID:       uuid.New().String(),
			Error:    err.Error(),
			ExitCode: -1,
		}
	}

	return ExecutionRecord{
		ID:         uuid.New().String(),
		CommandID:  commandID,
		FinalCmd:   cmdLine,
		ExecutedAt: time.Now(),
	}
}

// RunCommandInSession is RunCommand targeted at an explicit terminal session
// rather than whichever session is active. The global quick launcher uses it
// so its output stays self-contained in its dedicated internal session. The
// ID-addressable behavior is deliberate: internal sessions are a UI
// visibility/lifecycle convenience, not an access-control boundary.
func (s *ExecutionService) RunCommandInSession(
	commandID string,
	variables map[string]string,
	sessionID string,
) ExecutionRecord {
	if terminalSvc == nil {
		return ExecutionRecord{
			ID:        uuid.New().String(),
			CommandID: commandID,
			Error:     "terminal service not initialized",
			ExitCode:  -1,
		}
	}
	if sessionID == "" {
		return ExecutionRecord{
			ID:        uuid.New().String(),
			CommandID: commandID,
			Error:     "no terminal session specified",
			ExitCode:  -1,
		}
	}

	cmd, err := db.GetCommand(commandID)
	if err != nil {
		return ExecutionRecord{ID: uuid.New().String(), CommandID: commandID, Error: err.Error(), ExitCode: -1}
	}
	ss, err := terminalSvc.resolveSession(sessionID)
	if err != nil {
		return ExecutionRecord{ID: uuid.New().String(), CommandID: commandID, Error: err.Error(), ExitCode: -1}
	}

	resolvedScript, err := s.resolveScript(cmd, variables)
	if err != nil {
		return ExecutionRecord{ID: uuid.New().String(), CommandID: commandID, Error: err.Error(), ExitCode: -1}
	}
	shellPath := ss.info().ShellPath
	if shellPath == "" {
		shellPath, _ = detectShell()
	}
	workingDir := ""
	if s.hasExplicitWorkingDir(cmd) {
		workingDir = s.resolveWorkingDir(cmd)
	}
	cmdLine := buildCommandLine(shellPath, resolvedScript, workingDir)
	if err := terminalSvc.Write(sessionID, cmdLine); err != nil {
		return ExecutionRecord{ID: uuid.New().String(), CommandID: commandID, Error: err.Error(), ExitCode: -1}
	}

	return ExecutionRecord{
		ID:         uuid.New().String(),
		CommandID:  commandID,
		FinalCmd:   cmdLine,
		ExecutedAt: time.Now(),
	}
}
