package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Shell integration activates OSC 133 semantic-prompt markers ("C" before a
// command's output, "D;<exit code>" after) in the session's shell, so
// terminal_capture.go can record exactly what a command printed — see
// TerminalService.GetLastOutput. It works by pointing the shell at extra
// startup files embedded below (materialized once to disk, since shells read
// their config from real paths, not in-memory data) rather than editing the
// user's own dotfiles.
//
// all: is required, not optional, here: a plain `//go:embed shell-integration`
// silently excludes any file or directory whose name starts with "." or
// "_" — which is exactly the zsh integration files (.zshenv, .zprofile,
// .zshrc). Without it, zsh sessions would get an empty ZDOTDIR and silently
// never activate shell integration at all (this bit the first draft of this
// file and was caught by TestMaterializeShellIntegration_WritesExpectedFiles).
//
//go:embed all:shell-integration
var shellIntegrationFS embed.FS

// shellIntegrationDirName is the subdirectory of the app's data directory
// (~/.cmdex) the embedded scripts are written to.
const shellIntegrationDirName = "shell-integration"

// shellIntegrationEnvFlag is set in every integrated session's environment
// so the integration scripts (and a curious user, via `echo
// $CMDEX_SHELL_INTEGRATION`) can detect that it's active.
const shellIntegrationEnvFlag = "CMDEX_SHELL_INTEGRATION=1"

// setupShellIntegrationDir materializes the embedded shell-integration
// scripts to ~/.cmdex/shell-integration, overwriting whatever is already
// there so upgrades take effect on every app launch, and returns that
// directory's path. It resolves the home directory independently of the DB
// layer's own ~/.cmdex handling (db.go) rather than depending on it, since
// TerminalService.ServiceStartup may run before or after App's — this way
// neither service needs the other to have started first.
func setupShellIntegrationDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".cmdex", shellIntegrationDirName)
	if err := materializeShellIntegration(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// materializeShellIntegration writes the embedded shell-integration tree to
// dir.
func materializeShellIntegration(dir string) error {
	const embedRoot = "shell-integration"
	return fs.WalkDir(shellIntegrationFS, embedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, embedRoot), "/")
		target := filepath.Join(dir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		data, err := shellIntegrationFS.ReadFile(path)
		if err != nil {
			return err
		}
		//nolint:gosec // G306: these are non-secret shell startup scripts, meant to be readable like any dotfile.
		return os.WriteFile(target, data, 0o644)
	})
}

// shellIntegrationEnabled reports whether shell integration should be
// activated for a new session, honoring the user's settings toggle
// (AppSettings.ShellIntegration, default enabled when unset — see
// models.go). It tolerates db being nil or GetSettings failing: the very
// first session is created from TerminalService.ServiceStartup, which is
// not guaranteed to run after App.ServiceStartup has initialized db (see
// main.go's Services list), and — same as every other settings-read
// failure elsewhere in this codebase — defaulting to "on" is the safer
// failure mode than silently disabling a feature the user never touched.
func shellIntegrationEnabled() bool {
	if db == nil {
		return true
	}
	settings, err := db.GetSettings()
	if err != nil {
		return true
	}
	if settings.ShellIntegration == nil {
		return true
	}
	return *settings.ShellIntegration
}

// integrationFor returns the launch tweaks needed to activate OSC 133 shell
// integration for shellPath, dispatching on its basename (case-insensitive,
// ".exe" stripped so "pwsh" and "pwsh.exe" match the same case).
//
// When ok is true, effectiveFlag must be used in place of shellFlag for this
// launch — it may equal shellFlag unchanged (zsh, fish, pwsh all keep their
// normal flag) or be emptied out (bash, which ignores --rcfile whenever -l
// is also present, so -l must be dropped for integration to take effect) —
// and opts carries the additional args/env needed.
//
// When ok is false (an unrecognized shell — cmd.exe, /bin/sh, dash, a shell
// this app has no integration for, ...), the caller should launch exactly as
// it would have without shell integration; effectiveFlag/opts are zero
// values in that case.
func integrationFor(shellPath, shellFlag, intDir string) (string, shellLaunchOpts, bool) {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(shellPath)), ".exe")

	var flag string
	var opts shellLaunchOpts
	var ok bool
	switch base {
	case "zsh":
		flag, opts, ok = integrationForZsh(shellFlag, intDir)
	case "bash":
		flag, opts, ok = integrationForBash(intDir)
	case "fish":
		flag, opts, ok = integrationForFish(shellFlag, intDir)
	case "pwsh", "powershell":
		flag, opts, ok = integrationForPwsh(shellFlag, intDir)
	default:
		return "", shellLaunchOpts{}, false
	}

	// Every integrated shell needs this flag; prepending it here once means
	// the per-shell functions below only supply their shell-specific entries.
	opts.ExtraEnv = append([]string{shellIntegrationEnvFlag}, opts.ExtraEnv...)
	return flag, opts, ok
}

// integrationForZsh points ZDOTDIR at the integration scripts (zsh reads
// .zshenv/.zprofile/.zshrc/.zlogin from $ZDOTDIR, defaulting to $HOME when
// unset) and records the user's real zdotdir in CMDEX_USER_ZDOTDIR so those
// scripts can source the user's own config and restore ZDOTDIR once done —
// see shell-integration/zsh/.zshenv for the full mechanism. shellFlag (-l)
// is left untouched: zsh honors ZDOTDIR for login shells same as any other.
func integrationForZsh(shellFlag, intDir string) (string, shellLaunchOpts, bool) {
	userZDOTDIR := os.Getenv("ZDOTDIR")
	if userZDOTDIR == "" {
		userZDOTDIR, _ = os.UserHomeDir() // best-effort; scripts guard against "" too
	}

	return shellFlag, shellLaunchOpts{
		ExtraEnv: []string{
			"ZDOTDIR=" + filepath.Join(intDir, "zsh"),
			"CMDEX_USER_ZDOTDIR=" + userZDOTDIR,
		},
	}, true
}

// integrationForBash drops the shell's usual -l (login) flag and replaces
// it with --rcfile pointing at cmdex-bashrc.sh, plus -i to force interactive
// mode (--rcfile is otherwise ignored for a non-interactive shell). bash
// ignores --rcfile whenever -l/--login is also present, so -l cannot be
// kept — the rcfile itself replicates bash's normal login-file sourcing
// (see shell-integration/bash/cmdex-bashrc.sh) to avoid losing it.
func integrationForBash(intDir string) (string, shellLaunchOpts, bool) {
	return "", shellLaunchOpts{
		ExtraArgs: []string{"--rcfile", filepath.Join(intDir, "bash", "cmdex-bashrc.sh"), "-i"},
	}, true
}

// integrationForFish prepends the integration tree to XDG_DATA_DIRS so fish
// auto-loads shell-integration/fish-data/fish/vendor_conf.d/cmdex.fish
// alongside (not instead of) the user's own config.fish — see that file for
// why no restore step is needed here, unlike zsh's ZDOTDIR. shellFlag is
// left untouched.
func integrationForFish(shellFlag, intDir string) (string, shellLaunchOpts, bool) {
	dataDir := filepath.Join(intDir, "fish-data")
	xdg := dataDir
	if existing := os.Getenv("XDG_DATA_DIRS"); existing != "" {
		xdg = dataDir + string(os.PathListSeparator) + existing
	}

	return shellFlag, shellLaunchOpts{
		ExtraEnv: []string{
			"XDG_DATA_DIRS=" + xdg,
		},
	}, true
}

// integrationForPwsh keeps the shell's normal flag (e.g. "-NoLogo") — which
// still lets pwsh's own profile-processing startup phase run as usual — and
// appends -NoExit plus -Command to dot-source cmdex.ps1 afterward, so it can
// chain whatever `prompt` function the user's profile last defined. See
// shell-integration/pwsh/cmdex.ps1 for the marker mechanism itself
// (PowerShell has no preexec/precmd hooks to use directly) — this is the one
// piece of shell integration that could not be exercised on this
// darwin-only development machine and should be validated on a real Windows
// box before being relied upon.
func integrationForPwsh(shellFlag, intDir string) (string, shellLaunchOpts, bool) {
	script := filepath.Join(intDir, "pwsh", "cmdex.ps1")
	// PowerShell single-quoted strings escape an embedded ' by doubling it.
	// Without this, a home directory containing an apostrophe (Windows
	// permits them in usernames, e.g. C:\Users\O'Brien) would close the
	// quoted string early and break the generated -Command argument.
	quoted := "'" + strings.ReplaceAll(script, "'", "''") + "'"

	return shellFlag, shellLaunchOpts{
		ExtraArgs: []string{"-NoExit", "-Command", ". " + quoted},
	}, true
}
