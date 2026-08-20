package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// --- integrationFor: pure dispatch logic, no PTY needed ---

func TestIntegrationFor_Zsh(t *testing.T) {
	t.Setenv("ZDOTDIR", "/Users/someone/.dotfiles/zsh")

	flag, opts, ok := integrationFor("/bin/zsh", "-l", "/tmp/cmdex-integration", "test-nonce")
	if !ok {
		t.Fatal("expected ok=true for zsh")
	}
	if flag != "-l" {
		t.Errorf("effectiveFlag = %q, want unchanged %q", flag, "-l")
	}
	if opts.ExtraArgs != nil {
		t.Errorf("ExtraArgs = %v, want nil (zsh needs no arg changes)", opts.ExtraArgs)
	}

	env := envSliceToMap(opts.ExtraEnv)
	if env["ZDOTDIR"] != filepath.Join("/tmp/cmdex-integration", "zsh") {
		t.Errorf("ZDOTDIR = %q, want %q", env["ZDOTDIR"], filepath.Join("/tmp/cmdex-integration", "zsh"))
	}
	if env["CMDEX_USER_ZDOTDIR"] != "/Users/someone/.dotfiles/zsh" {
		t.Errorf("CMDEX_USER_ZDOTDIR = %q, want the original $ZDOTDIR", env["CMDEX_USER_ZDOTDIR"])
	}
	if env["CMDEX_SHELL_INTEGRATION"] != "1" {
		t.Errorf("CMDEX_SHELL_INTEGRATION = %q, want \"1\"", env["CMDEX_SHELL_INTEGRATION"])
	}
	if env["CMDEX_OSC_NONCE_FILE"] != "test-nonce" {
		t.Errorf("CMDEX_OSC_NONCE_FILE = %q, want %q", env["CMDEX_OSC_NONCE_FILE"], "test-nonce")
	}
}

func TestGenerateOSCNonce_ReturnsUniqueValues(t *testing.T) {
	a, err := generateOSCNonce()
	if err != nil {
		t.Fatalf("generateOSCNonce failed: %v", err)
	}
	b, err := generateOSCNonce()
	if err != nil {
		t.Fatalf("generateOSCNonce failed: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("expected non-empty nonces")
	}
	if a == b {
		t.Error("expected two calls to generateOSCNonce to return different values")
	}
}

// TestWriteNonceFile_IsPrivateAndCleansUp guards the fix for a review finding:
// the nonce must never be reachable through /proc/<pid>/environ (which
// persists a shell's exec-time environment regardless of a later unset), so
// it has to travel to the shell as a private file rather than an env var
// value. This checks the file/dir permissions actually exclude group/other,
// and that cleanup removes it.
func TestWriteNonceFile_IsPrivateAndCleansUp(t *testing.T) {
	path, cleanup, err := writeNonceFile("abc123")
	if err != nil {
		t.Fatalf("writeNonceFile failed: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", path, err)
	}
	if string(data) != "abc123" {
		t.Errorf("file contents = %q, want %q", data, "abc123")
	}

	// Unix permission bits are meaningless on Windows (os.Stat reports a
	// fixed mode unrelated to the file's actual ACL there), so this part of
	// the check only makes sense on POSIX platforms.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) failed: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("nonce file mode = %v, want no group/other permission bits", perm)
		}

		dirInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatalf("Stat(dir) failed: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("nonce dir mode = %v, want no group/other permission bits", perm)
		}
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected nonce file to be removed after cleanup, stat err = %v", err)
	}
}

func TestIntegrationFor_ZshFallsBackToHomeWhenZDOTDIRUnset(t *testing.T) {
	t.Setenv("ZDOTDIR", "")
	home := t.TempDir()
	// os.UserHomeDir() reads $HOME on Unix but %USERPROFILE% on Windows; set
	// both so this test is meaningful on every CI platform, even though zsh
	// integration itself only ever runs on Unix.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, opts, ok := integrationFor("/bin/zsh", "-l", "/tmp/cmdex-integration", "test-nonce")
	if !ok {
		t.Fatal("expected ok=true for zsh")
	}
	env := envSliceToMap(opts.ExtraEnv)
	if env["CMDEX_USER_ZDOTDIR"] != home {
		t.Errorf("CMDEX_USER_ZDOTDIR = %q, want $HOME (%q) when $ZDOTDIR is unset", env["CMDEX_USER_ZDOTDIR"], home)
	}
}

func TestIntegrationFor_Bash(t *testing.T) {
	flag, opts, ok := integrationFor("/bin/bash", "-l", "/tmp/cmdex-integration", "test-nonce")
	if !ok {
		t.Fatal("expected ok=true for bash")
	}
	if flag != "" {
		t.Errorf("effectiveFlag = %q, want \"\" (bash must drop -l for --rcfile to take effect)", flag)
	}
	want := []string{"--rcfile", filepath.Join("/tmp/cmdex-integration", "bash", "cmdex-bashrc.sh"), "-i"}
	if !slices.Equal(opts.ExtraArgs, want) {
		t.Errorf("ExtraArgs = %v, want %v", opts.ExtraArgs, want)
	}
}

func TestIntegrationFor_Fish(t *testing.T) {
	t.Setenv("XDG_DATA_DIRS", "/usr/share")

	flag, opts, ok := integrationFor("/usr/local/bin/fish", "-l", "/tmp/cmdex-integration", "test-nonce")
	if !ok {
		t.Fatal("expected ok=true for fish")
	}
	if flag != "-l" {
		t.Errorf("effectiveFlag = %q, want unchanged %q", flag, "-l")
	}
	env := envSliceToMap(opts.ExtraEnv)
	wantPrefix := filepath.Join("/tmp/cmdex-integration", "fish-data")
	if !strings.HasPrefix(env["XDG_DATA_DIRS"], wantPrefix) {
		t.Errorf("XDG_DATA_DIRS = %q, want it to start with %q", env["XDG_DATA_DIRS"], wantPrefix)
	}
	if !strings.Contains(env["XDG_DATA_DIRS"], "/usr/share") {
		t.Errorf("XDG_DATA_DIRS = %q, want the existing value preserved", env["XDG_DATA_DIRS"])
	}
}

func TestIntegrationFor_Pwsh(t *testing.T) {
	flag, opts, ok := integrationFor("/usr/local/bin/pwsh", "-NoLogo", "/tmp/cmdex-integration", "test-nonce")
	if !ok {
		t.Fatal("expected ok=true for pwsh")
	}
	if flag != "-NoLogo" {
		t.Errorf("effectiveFlag = %q, want unchanged %q", flag, "-NoLogo")
	}
	if len(opts.ExtraArgs) < 2 || opts.ExtraArgs[0] != "-NoExit" || opts.ExtraArgs[1] != "-Command" {
		t.Errorf("ExtraArgs = %v, want to start with [-NoExit -Command ...]", opts.ExtraArgs)
	}
}

// A home directory containing an apostrophe (Windows permits them in
// usernames, e.g. C:\Users\O'Brien) must not break the generated
// -Command's PowerShell single-quoted string.
func TestIntegrationFor_PwshEscapesApostropheInPath(t *testing.T) {
	_, opts, ok := integrationFor("/usr/local/bin/pwsh", "-NoLogo", "/tmp/O'Brien/cmdex-integration", "test-nonce")
	if !ok {
		t.Fatal("expected ok=true for pwsh")
	}
	if len(opts.ExtraArgs) < 3 {
		t.Fatalf("ExtraArgs = %v, want at least 3 elements", opts.ExtraArgs)
	}
	command := opts.ExtraArgs[2]

	// The embedded path's apostrophe must be doubled ('') per PowerShell's
	// single-quote escaping rule, and the resulting string must still be
	// balanced (an even number of quote characters).
	if !strings.Contains(command, "O''Brien") {
		t.Errorf("-Command = %q, want the path's apostrophe doubled as O''Brien", command)
	}
	if strings.Count(command, "'")%2 != 0 {
		t.Errorf("-Command = %q, has an unbalanced number of quotes", command)
	}
}

// Uses forward-slash paths rather than real backslash Windows paths:
// path/filepath's behavior is compiled per-GOOS, and on darwin (where this
// test suite actually runs) filepath.Base does not treat '\' as a separator
// at all — that's a property of filepath.Base itself, orthogonal to what
// this test wants to exercise (integrationFor's case/".exe" handling of
// whatever basename it's given).
func TestIntegrationFor_WindowsExeSuffixIsCaseInsensitive(t *testing.T) {
	if _, _, ok := integrationFor("/Program Files/PowerShell/7/PWSH.EXE", "-NoLogo", "/intdir", "test-nonce"); !ok {
		t.Error("expected ok=true for PWSH.EXE (case-insensitive, .exe stripped)")
	}
	if _, _, ok := integrationFor("/Windows/System32/Bash.exe", "-l", "/intdir", "test-nonce"); !ok {
		t.Error("expected ok=true for Bash.exe")
	}
}

func TestIntegrationFor_UnknownShellsReturnNotOk(t *testing.T) {
	for _, shell := range []string{"cmd", "/bin/sh", "/bin/dash", "/usr/bin/ksh", ""} {
		if _, _, ok := integrationFor(shell, "", "/tmp/cmdex-integration", "test-nonce"); ok {
			t.Errorf("integrationFor(%q) = ok=true, want ok=false (no integration for this shell)", shell)
		}
	}
}

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if key, value, ok := strings.Cut(kv, "="); ok {
			m[key] = value
		}
	}
	return m
}

// --- materializeShellIntegration: writes the embedded scripts to disk ---

func TestMaterializeShellIntegration_WritesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := materializeShellIntegration(dir); err != nil {
		t.Fatalf("materializeShellIntegration failed: %v", err)
	}

	for _, rel := range []string{
		filepath.Join("zsh", ".zshenv"),
		filepath.Join("zsh", ".zprofile"),
		filepath.Join("zsh", ".zshrc"),
		filepath.Join("bash", "cmdex-bashrc.sh"),
		filepath.Join("fish-data", "fish", "vendor_conf.d", "cmdex.fish"),
		filepath.Join("pwsh", "cmdex.ps1"),
	} {
		path := filepath.Join(dir, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", rel)
		}
	}

	// A dev-fixture zsh history file must never be embedded/shipped — it
	// isn't part of the integration and would get materialized into every
	// user's ~/.cmdex/shell-integration on every launch (see .zshrc's
	// HISTFILE-redirect fix for why one could otherwise appear here).
	if _, err := os.Stat(filepath.Join(dir, "zsh", ".zsh_history")); err == nil {
		t.Error("zsh/.zsh_history should not be part of the embedded shell-integration tree")
	}
}

func TestMaterializeShellIntegration_IdempotentAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	if err := materializeShellIntegration(dir); err != nil {
		t.Fatalf("first materializeShellIntegration failed: %v", err)
	}
	if err := materializeShellIntegration(dir); err != nil {
		t.Fatalf("second materializeShellIntegration failed: %v", err)
	}

	path := filepath.Join(dir, "zsh", ".zshenv")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to still exist after a second call: %v", path, err)
	}
}

// --- shellIntegrationEnabled: settings-driven, must default on ---

func TestShellIntegrationEnabled_DefaultsTrueWhenDBNil(t *testing.T) {
	// Drive the package-level db var directly rather than skipping when
	// some earlier test in this binary happened to have already
	// initialized it — that made this test's outcome depend on run order
	// instead of actually exercising the nil-db path it's named for.
	original := db
	db = nil
	defer func() { db = original }()

	if !shellIntegrationEnabled() {
		t.Error("expected shellIntegrationEnabled()=true when db is nil")
	}
}

// --- Real-PTY end-to-end: prove GetLastOutput actually works, not just integrationFor's plumbing ---

func newTestTerminalServiceWithShellIntegration(t *testing.T) *TerminalService {
	t.Helper()
	dir := t.TempDir()
	if err := materializeShellIntegration(dir); err != nil {
		t.Fatalf("materializeShellIntegration failed: %v", err)
	}
	s := &TerminalService{ptyBackend: newPtyBackend(), shellIntegrationDir: dir}
	s.sessions = make(map[string]*sessionState)
	return s
}

// waitForNextOutput polls GetLastOutput until it reports Available with a
// result different from baseline, or timeout elapses (returning whatever was
// last observed either way). Comparing against a baseline — not just
// checking Available — matters for any test that issues more than one
// command: immediately after writing a second command, GetLastOutput still
// reports the FIRST command's (already-Available) result until the shell's
// own round trip through the real PTY (sourcing profile files, running the
// command, redrawing the prompt) actually completes and overwrites it: a
// plain "wait for Available" would race and read stale data.
//
// Pass the zero value TerminalLastOutput{} as baseline for a session's very
// first command (Available=false makes any real result look "different").
func waitForNextOutput(
	t *testing.T,
	s *TerminalService,
	sessionID string,
	timeout time.Duration,
	baseline TerminalLastOutput,
) TerminalLastOutput {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last TerminalLastOutput
	for time.Now().Before(deadline) {
		out, err := s.GetLastOutput(sessionID)
		if err != nil {
			t.Fatalf("GetLastOutput failed: %v", err)
		}
		last = out
		if out.Available && out != baseline {
			return out
		}
		time.Sleep(20 * time.Millisecond)
	}
	return last
}

// waitForLastOutput is waitForNextOutput for a session's first command,
// where any Available result is by definition new.
func waitForLastOutput(t *testing.T, s *TerminalService, sessionID string, timeout time.Duration) TerminalLastOutput {
	t.Helper()
	return waitForNextOutput(t, s, sessionID, timeout, TerminalLastOutput{})
}

func TestShellIntegration_ZshCapturesCommandOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh not present on this machine")
	}
	t.Setenv("SHELL", "/bin/zsh")

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	if err := s.Write(id, "echo integration-test-hello\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	out := waitForLastOutput(t, s, id, 5*time.Second)
	if !out.Available {
		t.Fatal("GetLastOutput never became available — zsh integration did not activate")
	}
	if strings.TrimSpace(out.Text) != "integration-test-hello" {
		t.Errorf("Text = %q, want %q", out.Text, "integration-test-hello")
	}
	if out.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", out.ExitCode)
	}
}

func TestShellIntegration_ZshCapturesNonZeroExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh not present on this machine")
	}
	t.Setenv("SHELL", "/bin/zsh")

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	// Prime with a command whose output we don't care about, so we're not
	// racing the shell's own startup-time D marker.
	if err := s.Write(id, "true\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	baseline := waitForLastOutput(t, s, id, 5*time.Second)
	if !baseline.Available {
		t.Fatal("priming command's GetLastOutput never became available")
	}

	if err := s.Write(id, "sh -c 'exit 3'\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	out := waitForNextOutput(t, s, id, 5*time.Second, baseline)
	if !out.Available {
		t.Fatal("GetLastOutput never became available")
	}
	if out.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", out.ExitCode)
	}
}

// TestShellIntegration_ZshUserZshenvSeesRealZDOTDIR is the regression test
// for a bug found in review: .zshenv and .zprofile (unlike .zshrc) sourced
// the user's own file while $ZDOTDIR still pointed at Cmdex's integration
// directory, so a user config referencing $ZDOTDIR (e.g. `source
// "$ZDOTDIR/extra.zsh"`) would silently resolve inside Cmdex's own
// directory instead of the user's real one. The user's .zshenv here writes
// out whatever $ZDOTDIR it observed to a file, which a later command reads
// back through the normal capture path to prove the value it saw.
func TestShellIntegration_ZshUserZshenvSeesRealZDOTDIR(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh not present on this machine")
	}
	t.Setenv("SHELL", "/bin/zsh")

	userDir := t.TempDir()
	checkFile := filepath.Join(userDir, "zdotdir-check")
	zshenv := "if [ \"$ZDOTDIR\" = \"" + userDir + "\" ]; then\n" +
		"    echo correct > \"" + checkFile + "\"\n" +
		"else\n" +
		"    echo \"wrong:$ZDOTDIR\" > \"" + checkFile + "\"\n" +
		"fi\n"
	if err := os.WriteFile(filepath.Join(userDir, ".zshenv"), []byte(zshenv), 0o644); err != nil {
		t.Fatalf("write fake user .zshenv: %v", err)
	}
	t.Setenv("ZDOTDIR", userDir)

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	if err := s.Write(id, "cat \""+checkFile+"\"\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	out := waitForLastOutput(t, s, id, 5*time.Second)
	if !out.Available {
		t.Fatal("GetLastOutput never became available")
	}
	if strings.TrimSpace(out.Text) != "correct" {
		t.Errorf(
			"user's .zshenv observed %q, want $ZDOTDIR to be the user's real directory (%q) while it sourced",
			strings.TrimSpace(out.Text), userDir,
		)
	}
}

// TestShellIntegration_ZshUserZshenvRelocatingZDOTDIRIsHonored is the
// regression test for a bug found in review: a common zsh dotfiles pattern
// has .zshenv relocate $ZDOTDIR itself to point the rest of the startup
// chain (.zprofile/.zshrc/.zlogin) at a custom directory — .zshenv is the
// one file zsh always loads from the default location regardless of
// $ZDOTDIR, so it's the standard place to do this. Before the fix, Cmdex
// unconditionally restored $ZDOTDIR to the stale original directory right
// after sourcing the user's .zshenv, so its own .zshrc went on to source
// $CMDEX_USER_ZDOTDIR/.zshrc at the ORIGINAL location — never the user's
// real, relocated one — silently skipping their actual configuration.
func TestShellIntegration_ZshUserZshenvRelocatingZDOTDIRIsHonored(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh not present on this machine")
	}
	t.Setenv("SHELL", "/bin/zsh")

	originalDir := t.TempDir()
	relocatedDir := t.TempDir()
	markerFile := filepath.Join(relocatedDir, "relocated-zshrc-loaded")

	zshenv := "export ZDOTDIR=\"" + relocatedDir + "\"\n"
	if err := os.WriteFile(filepath.Join(originalDir, ".zshenv"), []byte(zshenv), 0o644); err != nil {
		t.Fatalf("write fake user .zshenv: %v", err)
	}
	zshrc := "touch \"" + markerFile + "\"\n"
	if err := os.WriteFile(filepath.Join(relocatedDir, ".zshrc"), []byte(zshrc), 0o644); err != nil {
		t.Fatalf("write fake relocated .zshrc: %v", err)
	}
	t.Setenv("ZDOTDIR", originalDir)

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	if err := s.Write(id, "cat \""+markerFile+"\" 2>&1; echo done\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	out := waitForLastOutput(t, s, id, 5*time.Second)
	if !out.Available {
		t.Fatal("GetLastOutput never became available")
	}
	if _, err := os.Stat(markerFile); err != nil {
		t.Errorf(
			"relocated .zshrc at %q never ran (marker file missing): %v — Cmdex likely sourced the stale original directory instead of the user's relocated one",
			relocatedDir,
			err,
		)
	}
}

// TestShellIntegration_ZshUserZshenvUnsettingZDOTDIRFallsBackToHome is the
// regression test for a bug found in review: the ZDOTDIR-relocation
// propagation above stored $ZDOTDIR verbatim into CMDEX_USER_ZDOTDIR, so a
// .zshenv that UNSETS $ZDOTDIR (the standard way to restore zsh's own
// default of looking in $HOME, the mirror image of relocating it to a new
// directory) propagated an EMPTY value. Every "-n $CMDEX_USER_ZDOTDIR"
// guard downstream (.zprofile's and .zshrc's own sourcing of the user's
// files, and .zshrc's final ZDOTDIR restore) then saw that as unset and
// skipped loading the user's config entirely, leaving ZDOTDIR pointed at
// Cmdex's own directory for the rest of the session.
func TestShellIntegration_ZshUserZshenvUnsettingZDOTDIRFallsBackToHome(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh not present on this machine")
	}
	t.Setenv("SHELL", "/bin/zsh")

	// originalDir simulates whatever $ZDOTDIR the user's environment had
	// before Cmdex launched the shell; home simulates $HOME, deliberately a
	// DIFFERENT directory, so the test can tell whether .zshrc was sourced
	// from the real fallback (home) rather than by coincidence.
	originalDir := t.TempDir()
	home := t.TempDir()
	markerFile := filepath.Join(home, "home-zshrc-loaded")

	if err := os.WriteFile(filepath.Join(originalDir, ".zshenv"), []byte("unset ZDOTDIR\n"), 0o644); err != nil {
		t.Fatalf("write fake user .zshenv: %v", err)
	}
	zshrc := "touch \"" + markerFile + "\"\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(zshrc), 0o644); err != nil {
		t.Fatalf("write fake home .zshrc: %v", err)
	}
	t.Setenv("ZDOTDIR", originalDir)
	t.Setenv("HOME", home)

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	if err := s.Write(id, "cat \""+markerFile+"\" 2>&1; echo done\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	out := waitForLastOutput(t, s, id, 5*time.Second)
	if !out.Available {
		t.Fatal("GetLastOutput never became available")
	}
	if _, err := os.Stat(markerFile); err != nil {
		t.Errorf(
			"$HOME/.zshrc at %q never ran (marker file missing): %v — unsetting $ZDOTDIR in the user's .zshenv likely caused Cmdex to skip the rest of their config",
			home,
			err,
		)
	}
}

// TestShellIntegration_BashPreservesPromptCommandArray is the regression
// test for a bug found in review: bash 5.1+ runs PROMPT_COMMAND as an array
// (every element, in order) when it's declared as one — some prompt/timing
// tools set it up that way. Before the fix, cmdex-bashrc.sh always rebuilt
// PROMPT_COMMAND as a single string via scalar expansion, which only reads
// element 0 of an array — every other element (and whatever it did) was
// silently dropped.
func TestShellIntegration_BashPreservesPromptCommandArray(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("/bin/bash not present on this machine")
	}

	// PROMPT_COMMAND-as-array is a bash 5.1+ feature; older bash (e.g.
	// macOS's system /bin/bash, frozen at 3.2 for licensing reasons) never
	// runs more than element 0 regardless of what cmdex-bashrc.sh does, so
	// the bug this test guards against can't be observed there.
	verOut, err := exec.Command("/bin/bash", "-c", "echo ${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}").Output()
	if err != nil {
		t.Fatalf("checking bash version: %v", err)
	}
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(verOut)), "%d.%d", &major, &minor); err != nil {
		t.Fatalf("parsing bash version %q: %v", verOut, err)
	}
	if major < 5 || (major == 5 && minor < 1) {
		t.Skipf("bash %d.%d predates 5.1's PROMPT_COMMAND array support", major, minor)
	}

	t.Setenv("SHELL", "/bin/bash")
	home := t.TempDir()
	t.Setenv("HOME", home)

	marker1 := filepath.Join(home, "hook1-ran")
	marker2 := filepath.Join(home, "hook2-ran")
	profile := "PROMPT_COMMAND=('touch \"" + marker1 + "\"' 'touch \"" + marker2 + "\"')\n"
	if err := os.WriteFile(filepath.Join(home, ".bash_profile"), []byte(profile), 0o644); err != nil {
		t.Fatalf("write fake .bash_profile: %v", err)
	}

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	if err := s.Write(id, "echo hi\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	out := waitForLastOutput(t, s, id, 5*time.Second)
	if !out.Available {
		t.Fatal("GetLastOutput never became available — bash integration did not activate")
	}

	for _, m := range []string{marker1, marker2} {
		if _, err := os.Stat(m); err != nil {
			t.Errorf("expected %s to exist (PROMPT_COMMAND array element should have run): %v", m, err)
		}
	}
}

// TestShellIntegration_BashCapturesOutputAndExitCode exercises the bash
// path specifically, since it's the one that needs its login flag (-l)
// replaced with --rcfile — the exact quirk this integration works around
// (see integrationForBash).
func TestShellIntegration_BashCapturesOutputAndExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("/bin/bash not present on this machine")
	}
	t.Setenv("SHELL", "/bin/bash")

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	if err := s.Write(id, "echo bash-integration-hello\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	out := waitForLastOutput(t, s, id, 5*time.Second)
	if !out.Available {
		t.Fatal("GetLastOutput never became available — bash integration did not activate")
	}
	if strings.TrimSpace(out.Text) != "bash-integration-hello" {
		t.Errorf("Text = %q, want %q", out.Text, "bash-integration-hello")
	}

	if err := s.Write(id, "false\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	out = waitForNextOutput(t, s, id, 5*time.Second, out)
	if out.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1 after `false`", out.ExitCode)
	}
}

// TestShellIntegration_LongOutputSurvivesNarrowTerminal is the direct
// regression test for the reported bug: the old frontend approach scraped
// xterm's on-screen buffer, so a command line that wrapped across several
// narrow rows could be mistaken for part of the output. Capture here reads
// raw PTY bytes before any client-side reflow, so a narrow terminal (or a
// command long enough to wrap many times over) cannot corrupt it.
func TestShellIntegration_LongOutputSurvivesNarrowTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("/bin/zsh not present on this machine")
	}
	t.Setenv("SHELL", "/bin/zsh")

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)
	if err := s.Resize(id, 20, 24); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}

	longLine := strings.Repeat("x", 250)
	if err := s.Write(id, "echo "+longLine+"\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	out := waitForLastOutput(t, s, id, 5*time.Second)
	if !out.Available {
		t.Fatal("GetLastOutput never became available")
	}
	if got := strings.TrimSpace(out.Text); got != longLine {
		t.Errorf(
			"Text = %q, want %q — output corrupted by narrow-terminal wrapping",
			got,
			longLine,
		)
	}
}

// TestShellIntegration_UnintegratedShellReportsUnavailable verifies a shell
// with no integration (here, /bin/sh) never produces a false-positive
// GetLastOutput result — the frontend relies on Available=false to know it
// must fall back to the xterm-scraping heuristic.
func TestShellIntegration_UnintegratedShellReportsUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh not present on this machine")
	}
	t.Setenv("SHELL", "/bin/sh")

	s := newTestTerminalServiceWithShellIntegration(t)
	id := mustCreateAndStart(t, s)

	if err := s.Write(id, "echo hello\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	out, err := s.GetLastOutput(id)
	if err != nil {
		t.Fatalf("GetLastOutput failed: %v", err)
	}
	if out.Available {
		t.Errorf("expected Available=false for an unintegrated shell, got %+v", out)
	}
}
