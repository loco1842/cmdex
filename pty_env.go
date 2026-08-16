package main

import (
	"sort"
	"strings"
)

// ptyEnvExtraCapacity is a sizing hint for the map buildPtyEnv populates:
// TERM, COLORTERM, TERM_PROGRAM, and LANG/LC_CTYPE may be added beyond
// whatever base already contains.
const ptyEnvExtraCapacity = 5

// buildPtyEnv returns an environment slice (KEY=VALUE entries, same shape as
// os.Environ()) suitable for the PTY child shell.
//
// GUI-launched apps (double-clicked from Finder, or opened from a mounted
// dmg) are started by launchd, which supplies none of TERM, COLORTERM, or
// LANG/LC_ALL — unlike a shell-launched `wails3 dev` process, which inherits
// the developer's terminal environment. Without TERM, zsh's line editor
// (ZLE) has no terminfo entry and cannot emit cursor-movement/clear
// sequences, so redraws (e.g. zsh-autosuggestions) append instead of
// overwriting — the visible "llsll" corruption this fixes.
//
// base is rewritten in place by key (never duplicated) so this function is
// idempotent and safe to call on an os.Environ() that may already define
// any of these variables.
//
// extraEnv (KEY=VALUE entries, same shape as base) is merged in by the same
// by-key-overwrite rule before TERM/COLORTERM/LANG are applied — currently
// used to activate OSC 133 shell integration (see shell_integration.go),
// e.g. zsh's ZDOTDIR override. Pass nil when there's nothing to add.
func buildPtyEnv(base []string, extraEnv []string) []string {
	env := make(map[string]string, len(base)+len(extraEnv)+ptyEnvExtraCapacity)
	for _, kv := range base {
		if key, value, ok := strings.Cut(kv, "="); ok {
			env[key] = value
		}
	}
	for _, kv := range extraEnv {
		if key, value, ok := strings.Cut(kv, "="); ok {
			env[key] = value
		}
	}

	// Force a modern, color-capable terminal type: the frontend is xterm.js,
	// so this is correct regardless of what (if anything) was inherited, and
	// it makes packaged and dev builds behave identically.
	env["TERM"] = "xterm-256color"
	env["COLORTERM"] = "truecolor"
	env["TERM_PROGRAM"] = "cmdex"

	// Default to a UTF-8 locale only if the shell wasn't already going to
	// have one — never override a user's real locale.
	if env["LANG"] == "" && env["LC_ALL"] == "" {
		env["LANG"] = "en_US.UTF-8"
		if env["LC_CTYPE"] == "" {
			env["LC_CTYPE"] = "en_US.UTF-8"
		}
	}

	// Stale inherited dimensions fight the real PTY winsize and confuse ZLE.
	delete(env, "COLUMNS")
	delete(env, "LINES")

	result := make([]string, 0, len(env))
	for key, value := range env {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}
