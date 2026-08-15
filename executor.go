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
