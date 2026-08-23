package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

type Executor struct {
	shell string
	flag  string
}

func NewExecutor() *Executor {
	var shell, flag string

	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/C"
	} else {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		flag = "-lc"
	}

	return &Executor{shell: shell, flag: flag}
}

// stripShebang removes any shebang line (#!...) from the beginning of script content.
// Used for backward compatibility with old DB records that stored scripts with #!/bin/bash.
func stripShebang(content string) string {
	s := strings.TrimSpace(content)
	if strings.HasPrefix(s, "#!") {
		if _, after, ok := strings.Cut(s, "\n"); ok {
			return after
		}
		return ""
	}
	return s
}

func shellQuoteDir(dir string) string {
	if !strings.Contains(dir, `'`) {
		return `'` + dir + `'`
	}
	escaped := strings.ReplaceAll(dir, `'`, `'"'"'`)
	return `'` + escaped + `'`
}

// shellBaseName returns shellPath's lowercased base name with any ".exe"
// suffix removed. It splits on both separators rather than using
// filepath.Base so a Windows shell path classifies identically no matter
// which OS is doing the classifying — that is what lets shellDialectFor and
// buildCommandLine be table-tested on Linux with real Windows inputs.
func shellBaseName(shellPath string) string {
	base := shellPath
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
}

// shellDialect identifies both the command syntax and the line-submit
// convention of the shell a terminal session is running.
type shellDialect int

const (
	// dialectPOSIX covers bash/zsh/fish/sh and every unrecognized shell.
	dialectPOSIX shellDialect = iota
	// dialectPowerShell covers pwsh (6/7+) and Windows PowerShell 5.1.
	dialectPowerShell
	// dialectCmd covers cmd.exe.
	dialectCmd
)

// shellDialectFor classifies shellPath by base name, the same way
// clearKeyFor does. detectShell only ever launches pwsh/powershell/cmd on
// Windows and a POSIX shell everywhere else, so no runtime.GOOS check is
// needed — and leaving it out is what keeps this function testable
// off-Windows.
func shellDialectFor(shellPath string) shellDialect {
	switch shellBaseName(shellPath) {
	case "cmd":
		return dialectCmd
	case "pwsh", "powershell":
		return dialectPowerShell
	default:
		return dialectPOSIX
	}
}

// submitKey returns the byte that makes this shell accept the line just
// written to its input.
//
// A Unix pty has a line discipline: it hands the reading process everything
// up to and including LF, so "\n" submits. Windows has none — ConPTY
// translates bytes written to the pseudoconsole's input side into console
// key events, and PSReadLine and cmd.exe both accept the current line only
// on Enter, i.e. CR (0x0D). LF arrives as Ctrl+J and is inserted into the
// edit buffer instead of submitting, which is exactly why a saved command
// used to appear at the Windows prompt fully typed but never run (issue
// #63). clearKeyFor already encodes this same platform reality with
// "cls\r".
func (d shellDialect) submitKey() string {
	if d == dialectPOSIX {
		return "\n"
	}
	return "\r"
}

// stripControlChars removes every ASCII C0 control byte (0x00-0x1F) and DEL
// (0x7F) from s. No real filesystem path legitimately contains any of these
// (unlike '%', there is no usability cost to removing them), and every one
// of them is meaningful to the terminal/line-editing layer the resolved
// command line is written into — not just '\r'/'\n' (submit keys), but also
// bytes like ^C/^\/^Z (signal generation), ^U/^W (line/word kill), and
// backspace/DEL (character erase). Those are intercepted by the pty's line
// discipline or the shell's line editor (e.g. readline) BEFORE the shell's
// own parser ever sees the quoted string, so shell-level quoting — which
// only constrains how the parser reads bytes it actually receives — cannot
// neutralize them. A crafted workingDir containing e.g. ^U (kill-line) could
// erase the "cd '<dir>' && " prefix already typed ahead of it and have
// whatever follows run as a brand-new, completely unrelated command line.
func stripControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// cdPrefix returns the change-directory command plus the separator that
// keeps it on the SAME submitted line as the script that follows, so the
// script does not run when the directory is unusable.
//
// Control characters are stripped here, once, ahead of the per-dialect
// quoting below — see stripControlChars.
func (d shellDialect) cdPrefix(dir string) string {
	dir = stripControlChars(dir)
	switch d {
	case dialectCmd:
		// cmd's bare `cd` will not cross drives: `cd D:\x` from C: changes
		// D:'s remembered directory and leaves the shell on C:. /d is
		// mandatory. cmd.exe does support && and short-circuits exactly
		// like the POSIX form.
		return "cd /d " + cmdQuoteDir(dir) + " && "
	case dialectPowerShell:
		// Windows PowerShell 5.1 has no && operator (pipeline chain
		// operators arrived in PowerShell 7), so the short-circuit is
		// expressed as a terminating error instead: -ErrorAction Stop
		// promotes Set-Location's non-terminating "path not found" into a
		// terminating one, which abandons the rest of the statements in
		// this submission. -LiteralPath stops [ and ] in a path from being
		// read as wildcards.
		return "Set-Location -LiteralPath " + psQuoteDir(dir) + " -ErrorAction Stop; "
	case dialectPOSIX:
		return "cd " + shellQuoteDir(dir) + " && "
	}
	return "cd " + shellQuoteDir(dir) + " && "
}

// groupDelims returns the open/close tokens that wrap a multi-line script
// into ONE unit for cdPrefix's && / -ErrorAction Stop to gate — see
// buildCommandLine. Each is a well-established multi-line construct native
// to its shell (an interactively-typed unclosed brace/paren is what already
// makes multi-line function/block definitions work at these shells'
// prompts), so accumulating the script's separately-submitted lines behind
// the open token and only closing it after the last one costs nothing
// beyond the two extra tokens.
//
// PowerShell dot-sources ("." rather than "&") specifically so variables
// and functions the script defines land in the CURRENT session scope
// instead of a child scope that disappears once the block returns — "&"
// would silently make e.g. an exported variable invisible to whatever the
// user types next in that terminal.
//
// Known limitation (cmd.exe only): wrapping in "( ... )" makes cmd.exe
// parse the whole parenthesized block before executing any of it, which is
// also when it textually expands any %VAR% the script references — not
// per-line, as it would for the same lines submitted unwrapped. A script
// that reads a value expected to change partway through (%errorlevel%,
// %random%, %date%/%time%, %cd% after an earlier `cd` in the same script)
// silently gets the value from before the block started instead, only when
// a working directory is configured (the case that triggers grouping at
// all). The standard cmd.exe fix is `setlocal enabledelayedexpansion` plus
// rewriting %VAR% to !VAR!, but auto-injecting that would change semantics
// for scripts that already rely on the classic %VAR% behavior elsewhere in
// the same block, so it isn't done here.
func (d shellDialect) groupDelims() (string, string) {
	switch d {
	case dialectCmd:
		return "( ", ")"
	case dialectPowerShell:
		return ". { ", "}"
	case dialectPOSIX:
		return "{ ", "}"
	}
	return "{ ", "}"
}

// cmdQuoteDir wraps dir in double quotes for cmd.exe. " is stripped first —
// it is not a legal character in a Windows path, so this is purely
// defensive against a value that could not have come from a real directory.
//
// % is stripped for a different reason: unlike every other special
// character, cmd.exe expands %VAR% references even inside double quotes —
// quoting does not make them literal. A workingDir of "%windir%" changes
// directory to wherever that variable points instead of failing, silently
// running the rest of the line somewhere other than the configured
// directory. There is no in-band way to escape a literal % for cmd.exe
// short of not being in a batch file (the usual %% doubling trick only
// applies inside one), so unlike " this can't be solved by quoting or
// doubling — removing every % guarantees no %...% pair can ever form from
// this value, at the cost of a working directory that legitimately
// contains a % losing that character (cd then fails closed rather than
// landing somewhere unintended).
func cmdQuoteDir(dir string) string {
	dir = strings.ReplaceAll(dir, `"`, "")
	dir = strings.ReplaceAll(dir, "%", "")
	return `"` + dir + `"`
}

// psQuoteDir wraps dir in a PowerShell single-quoted string, whose only
// escape is a doubled '. Single quotes rather than double matter:
// PowerShell expands $variables and $(subexpressions) inside
// double-quoted strings, so a path containing a literal $ would otherwise
// be rewritten.
func psQuoteDir(dir string) string {
	return "'" + strings.ReplaceAll(dir, "'", "''") + "'"
}

// buildCommandLine renders the exact string RunCommand writes to a
// session's PTY: the resolved script, optionally prefixed with a
// change-directory command, with every line terminated by the key this
// shell actually accepts (see submitKey). workingDir == "" means no cd
// prefix — the shell stays wherever it is.
//
// script may span several lines, and each of those is a separate line the
// shell has to accept, so its internal newlines are rewritten to the
// submit key too. CRLF is collapsed first: a script stored with Windows
// line endings would otherwise become a double Enter, the second of which
// submits an empty line at the prompt. The POSIX path is left untouched
// byte-for-byte, CRLF included, so nothing changes on the platforms where
// dispatch already works.
//
// When workingDir is set AND script spans multiple lines, the whole script
// is wrapped in the shell's own grouping construct (see groupDelims) rather
// than appended plainly after cdPrefix. Without that, cdPrefix's && /
// -ErrorAction Stop only gates whatever immediately follows it on the SAME
// submitted line — the script's first line — leaving every later line
// (submitted separately, per the paragraph above) to run unconditionally,
// in whatever directory the shell was already in, even after a failed cd.
// Grouping makes the whole script one unit that single gate covers. A
// single-line script needs none of this: cdPrefix already gates it
// directly, so that case (already exercised against real Windows hardware
// by TestPwshIntegration_WorkingDirPrefixShortCircuitsOnBadDir) is left
// byte-for-byte as before.
func buildCommandLine(shellPath, script, workingDir string) string {
	d := shellDialectFor(shellPath)
	key := d.submitKey()

	line := script
	suffix := ""
	if workingDir != "" {
		prefix := d.cdPrefix(workingDir)
		// A lone '\r' with no '\n' still submits its own line — on
		// cmd.exe/PowerShell because '\r' literally IS the submit key, and
		// on POSIX shells because readline's default keymap binds '\r' to
		// accept-line exactly like '\n'. A script containing one (e.g. from
		// an unsanitized template-variable value) must trigger the same
		// grouping multi-line scripts get, or that later fragment would run
		// as its own separately-submitted line, ungated by the cd check
		// above — checking only for "\n" let such a script slip through.
		if strings.ContainsAny(script, "\r\n") {
			open, groupClose := d.groupDelims()
			line = prefix + open + script
			suffix = groupClose
		} else {
			line = prefix + script
		}
	}

	if key != "\n" {
		line = strings.ReplaceAll(line, "\r\n", "\n")
		line = strings.ReplaceAll(line, "\n", key)
	}
	if suffix != "" {
		return line + key + suffix + key
	}
	return line + key
}

// EvalDefaults evaluates CEL expressions in variable definitions and returns resolved defaults.
func (e *Executor) EvalDefaults(defs []VariableDefinition) map[string]string {
	results := make(map[string]string, len(defs))
	if len(defs) == 0 {
		return results
	}

	env, err := cel.NewEnv(
		cel.Function("now",
			cel.Overload("now_void", nil, cel.StringType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					return types.String(time.Now().Format(time.RFC3339))
				}),
			),
		),
		cel.Function("env",
			cel.Overload("env_string", []*cel.Type{cel.StringType}, cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s, ok := val.(types.String)
					if !ok {
						return types.NewErr("expected string")
					}
					key := string(s)
					return types.String(os.Getenv(key))
				}),
			),
		),
		cel.Function("date",
			cel.Overload("date_string", []*cel.Type{cel.StringType}, cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s, ok := val.(types.String)
					if !ok {
						return types.NewErr("expected string")
					}
					layout := string(s)
					return types.String(time.Now().Format(layout))
				}),
			),
		),
	)
	if err != nil {
		for _, d := range defs {
			results[d.Name] = d.Default
		}
		return results
	}

	for _, d := range defs {
		if d.Default == "" {
			results[d.Name] = ""
			continue
		}

		ast, issues := env.Compile(d.Default)
		if issues != nil && issues.Err() != nil {
			results[d.Name] = d.Default
			continue
		}

		prg, err := env.Program(ast)
		if err != nil {
			results[d.Name] = d.Default
			continue
		}

		out, _, err := prg.Eval(cel.NoVars())
		if err != nil {
			results[d.Name] = d.Default
			continue
		}

		results[d.Name] = fmt.Sprintf("%v", out.Value())
	}

	return results
}
