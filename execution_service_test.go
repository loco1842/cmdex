package main

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func testCleanup(t *testing.T, dbConn *DB, catID, cmdID string) {
	t.Helper()
	dbConn.conn.Exec(`DELETE FROM commands WHERE id = ?`, cmdID)
	dbConn.conn.Exec(`DELETE FROM categories WHERE id = ?`, catID)
}

func testDBCreateCommand(
	t *testing.T,
	catID, cmdID, categoryName, cmdTitle, scriptContent, workingDirJSON string,
) (*DB, func()) {
	t.Helper()

	initDB, err := NewDB()
	if err != nil {
		t.Skipf("cannot open test DB: %v", err)
	}

	testCleanup(t, initDB, catID, cmdID)

	_, err = initDB.conn.Exec(
		`INSERT INTO categories (id, name, icon, color) VALUES (?, ?, '', '')`,
		catID,
		categoryName,
	)
	if err != nil {
		initDB.Close()
		t.Fatalf("insert category: %v", err)
	}

	_, err = initDB.conn.Exec(
		`INSERT INTO commands (id, category_id, title, script_content, working_dir, position) VALUES (?, ?, ?, ?, ?, 0)`,
		cmdID,
		catID,
		cmdTitle,
		GenerateScript(scriptContent),
		workingDirJSON,
	)
	if err != nil {
		initDB.Close()
		t.Fatalf("insert command: %v", err)
	}

	prevDB := db
	db = initDB

	return initDB, func() {
		db = prevDB
		initDB.Close()
	}
}

// testWithTerminalSvc sets up a real TerminalService so RunCommand can dispatch
// via terminalSvc.Write. Mirrors the save/restore pattern from
// TestTerminalService_ServiceStartupAssignsTerminalSvc. The returned cleanup
// func must be called (typically via defer) to restore the previous global
// state and shut down the test service.
func testWithTerminalSvc(t *testing.T) func() {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode (requires real PTY)")
	}
	prevTerminalSvc := terminalSvc
	terminalSvc = nil
	ts := &TerminalService{}
	if err := ts.ServiceStartup(nil, application.ServiceOptions{}); err != nil {
		terminalSvc = prevTerminalSvc
		t.Skipf("TerminalService.ServiceStartup failed: %v", err)
	}
	return func() {
		_ = ts.ServiceShutdown()
		terminalSvc = prevTerminalSvc
	}
}

func TestRunCommand_FinalCmdWithWorkingDir(t *testing.T) {
	defer testWithTerminalSvc(t)()

	dir := "/Users/test"
	if runtime.GOOS == "windows" {
		dir = `C:\Users\test`
	}
	workingDirJSON := `{"` + runtime.GOOS + `":` + strconv.Quote(dir) + `}`
	_, cleanup := testDBCreateCommand(
		t,
		"test-cat-wd-18",
		"test-cmd-wd-18",
		"TestWD",
		"Test Cmd WD",
		"echo hello",
		workingDirJSON,
	)
	defer cleanup()

	svc := &ExecutionService{}
	record := svc.RunCommand("test-cmd-wd-18", nil)

	if record.Error != "" {
		t.Errorf("Error = %q, want empty", record.Error)
	}

	if runtime.GOOS != "windows" {
		want := "cd '/Users/test' && echo hello\n"
		if record.FinalCmd != want {
			t.Errorf("FinalCmd = %q, want %q", record.FinalCmd, want)
		}
		return
	}

	// On Windows the exact shell (pwsh/powershell/cmd) depends on what's
	// installed on the runner, so assert the byte-exact shape via
	// TestBuildCommandLine and check the wiring structurally here: a single
	// CR-terminated line, no LF, containing both the working dir and the
	// script.
	if !strings.HasSuffix(record.FinalCmd, "\r") {
		t.Errorf("FinalCmd = %q, want it to end with \\r", record.FinalCmd)
	}
	if strings.Contains(record.FinalCmd, "\n") {
		t.Errorf("FinalCmd = %q, want no \\n on Windows", record.FinalCmd)
	}
	if !strings.Contains(record.FinalCmd, dir) {
		t.Errorf("FinalCmd = %q, want it to contain %q", record.FinalCmd, dir)
	}
	if !strings.Contains(record.FinalCmd, "echo hello") {
		t.Errorf("FinalCmd = %q, want it to contain %q", record.FinalCmd, "echo hello")
	}
}

func TestRunCommandInSession_ResolvesDefaultsAndRequiresValues(t *testing.T) {
	defer testWithTerminalSvc(t)()

	initDB := launcherTestDB(t)
	commandService := &CommandService{}
	cmd, err := commandService.CreateCommand(
		"variable command", "", "echo {{greeting}} {{required}}", "", nil,
		[]VariableDefinition{
			{Name: "greeting", Default: `"hello"`},
			{Name: "required"},
		}, OSPathMap{},
	)
	if err != nil {
		t.Fatalf("CreateCommand failed: %v", err)
	}
	t.Cleanup(func() { _ = initDB.DeleteCommand(cmd.ID) })

	session := terminalSvc.GetActiveSession()
	if session == nil {
		t.Fatal("test terminal has no active session")
	}
	svc := &ExecutionService{}
	missing := svc.RunCommandInSession(cmd.ID, nil, session.ID)
	if missing.Error != "missing required variable: required" {
		t.Fatalf("missing variable error = %q, want required-variable validation", missing.Error)
	}
	if missing.ExitCode != -1 {
		t.Fatalf("missing variable exit code = %d, want -1", missing.ExitCode)
	}

	resolved := svc.RunCommandInSession(cmd.ID, map[string]string{"required": "world"}, session.ID)
	if resolved.Error != "" {
		t.Fatalf("supplied variable error = %q, want empty", resolved.Error)
	}
	if !strings.Contains(resolved.FinalCmd, "echo hello world") {
		t.Fatalf("resolved command = %q, want CEL default and supplied value", resolved.FinalCmd)
	}
}

func TestResolveScript_AllowsExplicitDefaultThatEvaluatesEmpty(t *testing.T) {
	cmd := Command{
		ScriptContent: "echo {{optional}}",
		Variables: []VariableDefinition{{
			Name:    "optional",
			Default: `env("CMDEX_TEST_UNSET_DEFAULT_7D2C")`,
		}},
	}

	resolved, err := (&ExecutionService{}).resolveScript(cmd, nil)
	if err != nil {
		t.Fatalf("resolveScript returned error for empty evaluated default: %v", err)
	}
	if resolved != "echo" {
		t.Fatalf("resolved script = %q, want empty default substituted", resolved)
	}
}

func TestResolveScript_RequiresDefinitionWithoutDefault(t *testing.T) {
	cmd := Command{
		ScriptContent: "echo {{required}}",
		Variables:     []VariableDefinition{{Name: "required"}},
	}

	if _, err := (&ExecutionService{}).resolveScript(cmd, nil); err == nil {
		t.Fatal("resolveScript accepted a missing variable without a default")
	}
}

func TestResolveScript_IgnoresUnusedRequiredDefinition(t *testing.T) {
	cmd := Command{
		ScriptContent: "echo hello",
		Variables:     []VariableDefinition{{Name: "unused"}},
	}

	resolved, err := (&ExecutionService{}).resolveScript(cmd, nil)
	if err != nil {
		t.Fatalf("resolveScript rejected an unused required definition: %v", err)
	}
	if resolved != "echo hello" {
		t.Fatalf("resolved script = %q, want unchanged script", resolved)
	}
}

func TestRunCommand_FinalCmdNoWorkingDir(t *testing.T) {
	defer testWithTerminalSvc(t)()

	initDB, err := NewDB()
	if err != nil {
		t.Skipf("cannot open test DB: %v", err)
	}
	defer initDB.Close()

	settings, err := initDB.GetSettings()
	if err == nil {
		settings.DefaultWorkingDir = &OSPathMap{}
		_ = initDB.SetSettings(settings)
	}

	_, cleanup := testDBCreateCommand(
		t,
		"test-cat-nowd-18",
		"test-cmd-nowd-18",
		"TestNoWD",
		"Test Cmd NoWD",
		"echo hello",
		`{}`,
	)
	defer cleanup()

	svc := &ExecutionService{}
	record := svc.RunCommand("test-cmd-nowd-18", nil)

	if record.Error != "" {
		t.Errorf("Error = %q, want empty", record.Error)
	}

	want := "echo hello\n"
	if runtime.GOOS == "windows" {
		want = "echo hello\r"
	}
	if record.FinalCmd != want {
		t.Errorf("FinalCmd = %q, want %q", record.FinalCmd, want)
	}
}

func TestRunCommand_FinalCmdMultilineScript(t *testing.T) {
	defer testWithTerminalSvc(t)()

	dir := "/Users/test"
	if runtime.GOOS == "windows" {
		dir = `C:\Users\test`
	}
	workingDirJSON := `{"` + runtime.GOOS + `":` + strconv.Quote(dir) + `}`
	_, cleanup := testDBCreateCommand(
		t,
		"test-cat-ml-18",
		"test-cmd-ml-18",
		"TestML",
		"Test Cmd ML",
		"line1\nline2",
		workingDirJSON,
	)
	defer cleanup()

	svc := &ExecutionService{}
	record := svc.RunCommand("test-cmd-ml-18", nil)

	if record.Error != "" {
		t.Errorf("Error = %q, want empty", record.Error)
	}

	if runtime.GOOS != "windows" {
		// Grouped in { }: cdPrefix's && would otherwise only gate line1,
		// leaving line2 to run unconditionally even after a failed cd — see
		// TestBuildCommandLine's "posix multiline ... is grouped" case.
		want := "cd '/Users/test' && { line1\nline2\n}\n"
		if record.FinalCmd != want {
			t.Errorf("FinalCmd = %q, want %q", record.FinalCmd, want)
		}
		return
	}

	// Shell-agnostic proof that both script lines, plus the closing group
	// token, each got their own submitted line: three CRs total, and no LF
	// anywhere. Byte-exact shape per shell is covered by TestBuildCommandLine.
	if got := strings.Count(record.FinalCmd, "\r"); got != 3 {
		t.Errorf("FinalCmd = %q, want exactly 3 \\r, got %d", record.FinalCmd, got)
	}
	if strings.Contains(record.FinalCmd, "\n") {
		t.Errorf("FinalCmd = %q, want no \\n on Windows", record.FinalCmd)
	}
}

func TestRunCommand_GetCommandError(t *testing.T) {
	defer testWithTerminalSvc(t)()

	initDB, err := NewDB()
	if err != nil {
		t.Skipf("cannot open test DB: %v", err)
	}
	defer initDB.Close()

	prevDB := db
	db = initDB
	defer func() { db = prevDB }()

	svc := &ExecutionService{}
	record := svc.RunCommand("nonexistent-id-for-test", nil)

	if record.Error == "" {
		t.Error("expected Error field to be set when GetCommand fails, got empty string")
	}
	if record.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", record.ExitCode)
	}
}

func TestRunCommand_NoHistoryPersistence(t *testing.T) {
	defer testWithTerminalSvc(t)()

	initDB, cleanup := testDBCreateCommand(
		t,
		"test-cat-nohist-18",
		"test-cmd-nohist-18",
		"TestNoHist",
		"Test Cmd NoHist",
		"echo hello",
		`{}`,
	)
	defer cleanup()

	_ = initDB.ClearExecutions()

	svc := &ExecutionService{}
	svc.RunCommand("test-cmd-nohist-18", nil)

	records, err := initDB.GetExecutions()
	if err != nil {
		t.Fatalf("GetExecutions failed: %v", err)
	}
	if len(records) > 0 {
		t.Errorf("expected 0 execution records after RunCommand, got %d", len(records))
	}
}

func TestRunCommand_NilTerminalSvc(t *testing.T) {
	prevTerminalSvc := terminalSvc
	terminalSvc = nil
	defer func() { terminalSvc = prevTerminalSvc }()

	_, cleanup := testDBCreateCommand(
		t,
		"test-cat-nilsvc-24",
		"test-cmd-nilsvc-24",
		"TestNilSvc",
		"Test Cmd NilSvc",
		"echo hi",
		`{}`,
	)
	defer cleanup()

	svc := &ExecutionService{}
	record := svc.RunCommand("test-cmd-nilsvc-24", nil)
	if record.Error == "" {
		t.Error("expected Error to be set when terminalSvc is nil, got empty")
	}
	if record.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", record.ExitCode)
	}
}

func TestRunCommand_NoActiveSession(t *testing.T) {
	defer testWithTerminalSvc(t)()

	// testWithTerminalSvc creates a default session. To exercise the
	// "no active" path, clear the activeSessionID on the service.
	if terminalSvc == nil {
		t.Skip("terminalSvc not initialized")
	}
	terminalSvc.mu.Lock()
	terminalSvc.activeSessionID = ""
	terminalSvc.mu.Unlock()

	_, cleanup := testDBCreateCommand(
		t,
		"test-cat-noact-24",
		"test-cmd-noact-24",
		"TestNoAct",
		"Test Cmd NoAct",
		"echo hi",
		`{}`,
	)
	defer cleanup()

	svc := &ExecutionService{}
	record := svc.RunCommand("test-cmd-noact-24", nil)
	if record.Error == "" {
		t.Error("expected Error to be set when no active session, got empty")
	}
	if !strings.Contains(record.Error, "no active") {
		t.Errorf("Error = %q, want it to contain 'no active'", record.Error)
	}
	if record.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", record.ExitCode)
	}
}

func TestRunCommand_ExecutesOnActiveSession(t *testing.T) {
	defer testWithTerminalSvc(t)()

	// testWithTerminalSvc creates a default session and makes it active.
	// "echo ok" rather than "true": true isn't a recognized command on
	// Windows shells, so this keeps the happy path happy on every platform.
	_, cleanup := testDBCreateCommand(
		t,
		"test-cat-exec-24",
		"test-cmd-exec-24",
		"TestExec",
		"Test Cmd Exec",
		"echo ok",
		`{}`,
	)
	defer cleanup()

	svc := &ExecutionService{}
	record := svc.RunCommand("test-cmd-exec-24", nil)
	if record.Error != "" {
		t.Errorf("Error = %q, want empty (happy path)", record.Error)
	}
	want := "echo ok\n"
	if runtime.GOOS == "windows" {
		want = "echo ok\r"
	}
	if record.FinalCmd != want {
		t.Errorf("FinalCmd = %q, want %q", record.FinalCmd, want)
	}
}

func TestShellQuoteDir(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"simple path", "/Users/test", "'/Users/test'"},
		{"path with spaces", "/Users/My Folder", "'/Users/My Folder'"},
		{"path with quote", "/Users/O'Brien", "'/Users/O'\"'\"'Brien'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuoteDir(tt.dir)
			if got != tt.want {
				t.Errorf("shellQuoteDir(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

// TestBuildCommandLine is the cross-platform contract for issue #63: the
// line RunCommand writes must be terminated by the key the target shell
// actually accepts, and the cd prefix must be syntax that shell can parse.
// Classification is by shell base name alone (shellDialectFor), so every
// Windows case below is exercised on Linux CI too — there is no other seam
// that can prove Windows behavior without a Windows runner.
func TestBuildCommandLine(t *testing.T) {
	const psWin = `C:\Program Files\PowerShell\7\pwsh.exe`
	const ps51 = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	const cmdExe = `C:\Windows\System32\cmd.exe`

	tests := []struct {
		name       string
		shellPath  string
		script     string
		workingDir string
		want       string
	}{
		// POSIX — must stay byte-identical to pre-fix behavior.
		{"posix no wd", "/bin/zsh", "echo hello", "", "echo hello\n"},
		{"posix with wd", "/bin/zsh", "echo hello", "/Users/test", "cd '/Users/test' && echo hello\n"},
		{
			"posix wd with apostrophe", "/bin/bash", "echo hi", "/Users/O'Brien",
			`cd '/Users/O'"'"'Brien' && echo hi` + "\n",
		},
		{
			// Grouped in { }: without it, only line1 would be gated by the
			// cd's success (see "posix multiline with bad-looking wd still
			// grouped" and TestBuildCommandLine's cmd/pwsh equivalents) —
			// line2 would run unconditionally as its own, separately
			// submitted line even if the cd had failed.
			"posix multiline keeps LF and is grouped so cd gates both lines", "/bin/zsh", "line1\nline2", "/Users/test",
			"cd '/Users/test' && { line1\nline2\n}\n",
		},
		{
			"posix multiline with no wd is not grouped", "/bin/zsh", "line1\nline2", "",
			"line1\nline2\n",
		},
		{"posix leaves CRLF alone", "/bin/zsh", "line1\r\nline2", "", "line1\r\nline2\n"},
		{"unknown shell defaults to posix", "/usr/bin/dash", "echo hi", "", "echo hi\n"},
		{"empty shell path defaults to posix", "", "echo hi", "", "echo hi\n"},

		// PowerShell — CR submits; Set-Location, never &&.
		{"pwsh no wd", psWin, "echo hello", "", "echo hello\r"},
		{
			"pwsh with wd", psWin, "echo hello", `C:\Users\test`,
			`Set-Location -LiteralPath 'C:\Users\test' -ErrorAction Stop; echo hello` + "\r",
		},
		{
			"windows powershell 5.1 uses no && operator", ps51, "echo hello", `C:\Users\test`,
			`Set-Location -LiteralPath 'C:\Users\test' -ErrorAction Stop; echo hello` + "\r",
		},
		{
			"pwsh wd with apostrophe is doubled", psWin, "echo hi", `C:\Users\O'Brien`,
			`Set-Location -LiteralPath 'C:\Users\O''Brien' -ErrorAction Stop; echo hi` + "\r",
		},
		{"pwsh multiline submits each line", psWin, "line1\nline2", "", "line1\rline2\r"},
		{"pwsh CRLF does not become a double Enter", psWin, "line1\r\nline2", "", "line1\rline2\r"},
		{
			// Dot-sourced ("." not "&") so the block runs in the current
			// session scope, not a child scope that disappears once it
			// returns — otherwise a script's own variable/function
			// definitions would vanish right after it ran.
			"pwsh multiline with wd is grouped in a dot-sourced block", psWin, "line1\nline2", `C:\Users\test`,
			`Set-Location -LiteralPath 'C:\Users\test' -ErrorAction Stop; . { line1` + "\rline2\r}\r",
		},

		// cmd.exe — CR submits; cd /d + double quotes; && is supported.
		{"cmd no wd", cmdExe, "echo hello", "", "echo hello\r"},
		{
			"cmd with wd needs /d to cross drives", cmdExe, "echo hello", `D:\work`,
			`cd /d "D:\work" && echo hello` + "\r",
		},
		{
			"cmd wd with spaces", cmdExe, "echo hi", `C:\Program Files\app`,
			`cd /d "C:\Program Files\app" && echo hi` + "\r",
		},
		{
			// Grouped in ( ): cmd.exe's own "More?" continuation for an
			// unclosed paren accumulates line1/line2/) into one block that
			// && gates as a unit, instead of only gating line1.
			"cmd multiline is grouped so cd gates both lines", cmdExe, "line1\nline2", `D:\work`,
			`cd /d "D:\work" && ( line1` + "\rline2\r)\r",
		},
		{
			"cmd multiline with no wd is not grouped", cmdExe, "line1\nline2", "",
			"line1\rline2\r",
		},
		{
			// cmd.exe expands %VAR% even inside double quotes, so a
			// workingDir of "%windir%" would silently cd somewhere other
			// than the literal string shown in the UI. % has no working
			// escape at the interactive prompt (the %% doubling trick only
			// applies inside batch files), so it is stripped instead — same
			// treatment as ", just for a different reason.
			"cmd wd with percent has it stripped to prevent var expansion", cmdExe, "echo hi", `C:\Users\test\%windir%`,
			`cd /d "C:\Users\test\windir" && echo hi` + "\r",
		},
		{
			// A CR/LF embedded in workingDir (e.g. from a hand-edited import
			// file) is byte-identical to some dialect's own submit key —
			// left in, it would inject an early Enter into the middle of
			// the cd command itself, before per-dialect quoting even runs.
			// No real path legitimately contains either, so stripping is
			// free of the tradeoff % stripping has.
			"cmd wd with embedded CRLF has it stripped", cmdExe, "echo hi", "C:\\evil\r\ncalc.exe",
			`cd /d "C:\evilcalc.exe" && echo hi` + "\r",
		},
		{
			"pwsh wd with embedded CR has it stripped", psWin, "echo hi", "C:\\evil\rcalc.exe",
			`Set-Location -LiteralPath 'C:\evilcalc.exe' -ErrorAction Stop; echo hi` + "\r",
		},
		{
			"posix wd with embedded LF has it stripped", "/bin/zsh", "echo hi", "/tmp/evil\ncalc",
			"cd '/tmp/evilcalc' && echo hi\n",
		},
		{
			// Not just '\r'/'\n': any C0 control byte is meaningful to the
			// pty's line discipline or the shell's line editor (e.g. ^U
			// kill-line, ^C SIGINT) BEFORE the shell's own quoting-aware
			// parser ever sees the string, so those bytes could bypass
			// shellQuoteDir/cmdQuoteDir/psQuoteDir entirely regardless of
			// what they wrap the value in.
			"posix wd with embedded control char (^U) has it stripped", "/bin/zsh", "echo hi", "/tmp/evil\x15calc",
			"cd '/tmp/evilcalc' && echo hi\n",
		},
		{
			"cmd wd with embedded control char (^C) has it stripped", cmdExe, "echo hi", "C:\\evil\x03calc.exe",
			`cd /d "C:\evilcalc.exe" && echo hi` + "\r",
		},
		{
			// A lone '\r' with no '\n' still submits its own line on every
			// dialect (literally on cmd.exe/PowerShell; via readline's
			// accept-line binding on POSIX) — checking only for "\n" would
			// let this bypass grouping and run "command" as its own
			// separately-submitted, cd-ungated line.
			"posix script with lone CR (no LF) is still grouped when wd is set", "/bin/zsh", "some\rcommand", "/tmp/x",
			"cd '/tmp/x' && { some\rcommand\n}\n",
		},
		{
			"cmd script with lone CR (no LF) is still grouped when wd is set", cmdExe, "some\rcommand", `D:\work`,
			"cd /d \"D:\\work\" && ( some\rcommand\r)\r",
		},
		{"bare cmd with no extension", "cmd", "echo hi", "", "echo hi\r"},
		{"case-insensitive CMD.EXE", `C:\Windows\System32\CMD.EXE`, "echo hi", "", "echo hi\r"},
		{"unix-style pwsh path", "/usr/local/bin/pwsh", "echo hi", "", "echo hi\r"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildCommandLine(tt.shellPath, tt.script, tt.workingDir); got != tt.want {
				t.Errorf("buildCommandLine(%q, %q, %q) = %q, want %q",
					tt.shellPath, tt.script, tt.workingDir, got, tt.want)
			}
		})
	}
}

func TestTerminalService_ServiceStartupAssignsTerminalSvc(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	prevTerminalSvc := terminalSvc
	terminalSvc = nil
	defer func() { terminalSvc = prevTerminalSvc }()

	s := &TerminalService{}
	_ = s.ServiceStartup(nil, application.ServiceOptions{})
	defer s.ServiceShutdown()

	if terminalSvc == nil {
		t.Error("terminalSvc should be non-nil after ServiceStartup, got nil")
	}

	s.mu.RLock()
	count := len(s.sessions)
	s.mu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 default session after ServiceStartup, got %d", count)
	}

	if s.sessionCounter != 1 {
		t.Errorf("expected sessionCounter=1 after ServiceStartup, got %d", s.sessionCounter)
	}
}
