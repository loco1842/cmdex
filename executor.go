package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// OpenInTerminal opens a terminal and runs the resolved script.
// Each LaunchFn receives the raw script body and handles its own quoting.
func (e *Executor) OpenInTerminal(terminalID string, scriptContent string, workingDir string) error {
	body := stripShebang(scriptContent)
	body = strings.TrimSpace(body)
	defs := e.terminalDefs()

	if terminalID != "" {
		for _, d := range defs {
			if d.ID == terminalID && e.terminalExists(d) && d.LaunchFn != nil {
				return d.LaunchFn(e, body, workingDir)
			}
		}
	}

	for _, d := range defs {
		if e.terminalExists(d) && d.LaunchFn != nil {
			return d.LaunchFn(e, body, workingDir)
		}
	}

	return errors.New("no terminal emulator found")
}

func shellQuoteDir(dir string) string {
	if !strings.Contains(dir, `'`) {
		return `'` + dir + `'`
	}
	escaped := strings.ReplaceAll(dir, `'`, `'"'"'`)
	return `'` + escaped + `'`
}

// terminalDef defines how to detect and launch a terminal emulator
type terminalDef struct {
	ID       string
	Name     string
	Paths    []string // candidate binary paths or app bundle paths
	IsApp    bool     // macOS .app bundle (use osascript to launch)
	LaunchFn func(e *Executor, cmdText string, workingDir string) error
}

// GetAvailableTerminals returns all terminal emulators detected on the current system.
func (e *Executor) GetAvailableTerminals() []TerminalInfo {
	defs := e.terminalDefs()
	var result []TerminalInfo
	for _, d := range defs {
		if e.terminalExists(d) {
			result = append(result, TerminalInfo{ID: d.ID, Name: d.Name})
		}
	}
	if result == nil {
		result = []TerminalInfo{}
	}
	return result
}

func resolveDarwinBin(plain, bundleBin string) string {
	if _, err := exec.LookPath(plain); err != nil {
		return bundleBin
	}
	return plain
}

func (e *Executor) terminalExists(d terminalDef) bool {
	for _, p := range d.Paths {
		if d.IsApp {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		} else {
			if _, err := exec.LookPath(p); err == nil {
				return true
			}
		}
	}
	return false
}

func (e *Executor) terminalDefs() []terminalDef {
	switch runtime.GOOS {
	case "darwin":
		return e.darwinTerminals()
	case "linux":
		return e.linuxTerminals()
	case "windows":
		return e.windowsTerminals()
	}
	return nil
}

func (e *Executor) darwinTerminals() []terminalDef {
	osa := func(appName, script string) func(*Executor, string, string) error {
		return func(_ *Executor, body string, workingDir string) error {
			body = strings.TrimRight(body, "\n")
			if workingDir != "" {
				body = fmt.Sprintf("cd %s && %s", shellQuoteDir(workingDir), body)
			}
			asEscaped := strings.ReplaceAll(body, `\`, `\\`)
			asEscaped = strings.ReplaceAll(asEscaped, `"`, `\"`)
			s := fmt.Sprintf(script, asEscaped)
			return exec.CommandContext(context.Background(), "osascript", "-e", s).Start()
		}
	}

	alacrittyBin, alacrittyBundle := "alacritty", "/Applications/Alacritty.app/Contents/MacOS/alacritty"
	kittyBin, kittyBundle := "kitty", "/Applications/kitty.app/Contents/MacOS/kitty"
	ghosttyBin, ghosttyBundle := "ghostty", "/Applications/Ghostty.app/Contents/MacOS/ghostty"

	return []terminalDef{
		{
			ID:    "terminal",
			Name:  "Terminal",
			Paths: []string{"/System/Applications/Utilities/Terminal.app"},
			IsApp: true,
			LaunchFn: osa("Terminal", `tell application "Terminal"
	do script "%s"
	activate
end tell`),
		},
		{
			ID: "iterm2", Name: "iTerm2", Paths: []string{"/Applications/iTerm.app"}, IsApp: true,
			LaunchFn: osa("iTerm2", `tell application "iTerm2"
	create window with default profile
	tell current session of current window
		write text "%s"
	end tell
	activate
end tell`),
		},
		{
			ID: "warp", Name: "Warp", Paths: []string{"/Applications/Warp.app"}, IsApp: true,
			LaunchFn: func(_ *Executor, body string, workingDir string) error {
				body = strings.TrimRight(body, "\n")
				if workingDir != "" {
					body = fmt.Sprintf("cd %s && %s", shellQuoteDir(workingDir), body)
				}
				asEscaped := strings.ReplaceAll(body, `\`, `\\`)
				asEscaped = strings.ReplaceAll(asEscaped, `"`, `\"`)
				s := fmt.Sprintf(`tell application "Warp" to activate
	delay 0.5
	tell application "System Events" to keystroke "%s"
	tell application "System Events" to key code 36`, asEscaped)
				return exec.CommandContext(context.Background(), "osascript", "-e", s).Start()
			},
		},
		{
			ID: "alacritty", Name: "Alacritty", Paths: []string{alacrittyBin, alacrittyBundle}, IsApp: false,
			LaunchFn: func(ex *Executor, body string, workingDir string) error {
				args := []string{"-e", ex.shell, ex.flag, body + "; exec " + ex.shell}
				if workingDir != "" {
					args = append([]string{"--working-directory", workingDir}, args...)
				}
				bin := resolveDarwinBin(alacrittyBin, alacrittyBundle)
				return exec.CommandContext(context.Background(), bin, args...).Start()
			},
		},
		{
			ID: "kitty", Name: "Kitty", Paths: []string{kittyBin, kittyBundle}, IsApp: false,
			LaunchFn: func(ex *Executor, body string, workingDir string) error {
				args := []string{ex.shell, ex.flag, body + "; exec " + ex.shell}
				if workingDir != "" {
					args = append([]string{"--directory", workingDir}, args...)
				}
				bin := resolveDarwinBin(kittyBin, kittyBundle)
				return exec.CommandContext(context.Background(), bin, args...).Start()
			},
		},
		{
			ID: "ghostty", Name: "Ghostty", Paths: []string{ghosttyBin, ghosttyBundle}, IsApp: false,
			LaunchFn: func(ex *Executor, body string, workingDir string) error {
				args := []string{"-e", ex.shell, ex.flag, body + "; exec " + ex.shell}
				if workingDir != "" {
					args = append([]string{"--working-directory=" + workingDir}, args...)
				}
				bin := resolveDarwinBin(ghosttyBin, ghosttyBundle)
				return exec.CommandContext(context.Background(), bin, args...).Start()
			},
		},
		{
			ID: "hyper", Name: "Hyper", Paths: []string{"/Applications/Hyper.app"}, IsApp: true,
			LaunchFn: func(_ *Executor, body string, workingDir string) error {
				return exec.CommandContext(context.Background(), "open", "-a", "Hyper").Start()
			},
		},
	}
}

func (e *Executor) linuxTerminals() []terminalDef {
	shellExec := func(bin string, buildArgs func(shell, body string) []string, dirFlag func(dir string) []string) func(*Executor, string, string) error {
		return func(ex *Executor, body string, workingDir string) error {
			args := buildArgs(ex.shell, body)
			if workingDir != "" && dirFlag != nil {
				args = append(dirFlag(workingDir), args...)
			}
			return exec.CommandContext(context.Background(), bin, args...).Start()
		}
	}

	return []terminalDef{
		{ID: "gnome-terminal", Name: "GNOME Terminal", Paths: []string{"gnome-terminal"},
			LaunchFn: shellExec("gnome-terminal", func(sh, body string) []string {
				return []string{"--", sh, "-c", body + "; exec " + sh}
			}, func(dir string) []string {
				return []string{"--working-directory=" + dir}
			})},
		{ID: "gnome-console", Name: "GNOME Console", Paths: []string{"kgx"},
			LaunchFn: shellExec("kgx", func(sh, body string) []string {
				escaped := strings.ReplaceAll(body, "'", `'\''`)
				return []string{"-e", sh + " -c '" + escaped + "; exec " + sh + "'"}
			}, func(dir string) []string {
				return []string{"--working-directory=" + dir}
			})},
		{ID: "konsole", Name: "Konsole", Paths: []string{"konsole"},
			LaunchFn: shellExec("konsole", func(sh, body string) []string {
				return []string{"-e", sh, "-c", body + "; exec " + sh}
			}, func(dir string) []string {
				return []string{"--workdir", dir}
			})},
		{ID: "xfce4-terminal", Name: "XFCE Terminal", Paths: []string{"xfce4-terminal"},
			LaunchFn: shellExec("xfce4-terminal", func(sh, body string) []string {
				escaped := strings.ReplaceAll(body, "'", `'\''`)
				return []string{"-e", sh + " -c '" + escaped + "; exec " + sh + "'"}
			}, func(dir string) []string {
				return []string{"--working-directory=" + dir}
			})},
		{ID: "alacritty", Name: "Alacritty", Paths: []string{"alacritty"},
			LaunchFn: shellExec("alacritty", func(sh, body string) []string {
				return []string{"-e", sh, "-c", body + "; exec " + sh}
			}, func(dir string) []string {
				return []string{"--working-directory", dir}
			})},
		{ID: "kitty", Name: "Kitty", Paths: []string{"kitty"},
			LaunchFn: shellExec("kitty", func(sh, body string) []string {
				return []string{sh, "-c", body + "; exec " + sh}
			}, func(dir string) []string {
				return []string{"--directory", dir}
			})},
		{ID: "ghostty", Name: "Ghostty", Paths: []string{"ghostty"},
			LaunchFn: shellExec("ghostty", func(sh, body string) []string {
				return []string{"-e", sh, "-c", body + "; exec " + sh}
			}, func(dir string) []string {
				return []string{"--working-directory=" + dir}
			})},
		{ID: "xterm", Name: "XTerm", Paths: []string{"xterm"},
			LaunchFn: func(ex *Executor, body string, workingDir string) error {
				body = strings.TrimRight(body, "\n")
				if workingDir != "" {
					body = fmt.Sprintf("cd %s && %s", shellQuoteDir(workingDir), body)
				}
				//nolint:gosec // G204: shell/body are local executor fields and user-authored script content by design
				return exec.CommandContext(context.Background(), "xterm", "-e", ex.shell, "-c", body+"; exec "+ex.shell).
					Start()
			}},
	}
}

func (e *Executor) windowsTerminals() []terminalDef {
	escapeCmdExe := func(body string) string {
		s := strings.ReplaceAll(body, `"`, `""`)
		s = strings.ReplaceAll(s, `%`, `%%`)
		return `"` + s + `"`
	}

	return []terminalDef{
		{ID: "windows-terminal", Name: "Windows Terminal", Paths: []string{"wt"},
			LaunchFn: func(_ *Executor, body string, workingDir string) error {
				args := []string{"cmd", "/k", escapeCmdExe(body)}
				if workingDir != "" {
					args = append([]string{"-d", workingDir}, args...)
				}
				return exec.CommandContext(context.Background(), "wt", args...).Start()
			}},
		{ID: "cmd", Name: "Command Prompt", Paths: []string{"cmd"},
			LaunchFn: func(_ *Executor, body string, workingDir string) error {
				cmdBody := escapeCmdExe(body)
				if workingDir != "" {
					cmdBody = fmt.Sprintf("cd /d %s && %s", escapeCmdExe(workingDir), cmdBody)
				}
				return exec.CommandContext(context.Background(), "cmd", "/c", "start", "cmd", "/k", cmdBody).Start()
			}},
		{ID: "pwsh", Name: "PowerShell", Paths: []string{"pwsh", "powershell"},
			LaunchFn: func(_ *Executor, body string, workingDir string) error {
				bin := "powershell"
				if _, err := exec.LookPath("pwsh"); err == nil {
					bin = "pwsh"
				}
				if workingDir != "" {
					body = fmt.Sprintf(
						"Set-Location -LiteralPath '%s' -ErrorAction Stop; %s",
						strings.ReplaceAll(workingDir, "'", "''"),
						body,
					)
				}
				return exec.CommandContext(context.Background(), bin, "-NoExit", "-Command", body).Start()
			}},
	}
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
