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

// cdPrefix returns the change-directory command plus the separator that
// keeps it on the SAME submitted line as the script that follows, so the
// script does not run when the directory is unusable.
func (d shellDialect) cdPrefix(dir string) string {
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

// cmdQuoteDir wraps dir in double quotes for cmd.exe, which has no escape
// mechanism inside them and needs none: " is not a legal character in a
// Windows path, so the strip below is purely defensive against a value
// that could not have come from a real directory.
func cmdQuoteDir(dir string) string {
	return `"` + strings.ReplaceAll(dir, `"`, "") + `"`
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
func buildCommandLine(shellPath, script, workingDir string) string {
	d := shellDialectFor(shellPath)
	key := d.submitKey()

	line := script
	if workingDir != "" {
		line = d.cdPrefix(workingDir) + script
	}

	if key != "\n" {
		line = strings.ReplaceAll(line, "\r\n", "\n")
		line = strings.ReplaceAll(line, "\n", key)
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
