package main

import "testing"

// envMap converts a KEY=VALUE slice (the os.Environ() shape) into a map for
// easy assertions, failing the test if any key is duplicated.
func envMap(t *testing.T, env []string) map[string]string {
	t.Helper()
	m := make(map[string]string, len(env))
	for _, kv := range env {
		key, value, ok := splitEnvEntry(kv)
		if !ok {
			t.Fatalf("malformed env entry: %q", kv)
		}
		if _, dup := m[key]; dup {
			t.Fatalf("duplicate env key %q in output: %v", key, env)
		}
		m[key] = value
	}
	return m
}

func splitEnvEntry(kv string) (key, value string, ok bool) {
	for i := range kv {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return "", "", false
}

func TestBuildPtyEnv_ForcesTermWhenAbsent(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"HOME=/Users/mac"}))
	if out["TERM"] != "xterm-256color" {
		t.Errorf("TERM = %q, want xterm-256color", out["TERM"])
	}
}

func TestBuildPtyEnv_OverridesExistingTerm(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"TERM=dumb"}))
	if out["TERM"] != "xterm-256color" {
		t.Errorf("TERM = %q, want xterm-256color (dumb should be overridden)", out["TERM"])
	}
}

func TestBuildPtyEnv_ForcesColorterm(t *testing.T) {
	out := envMap(t, buildPtyEnv(nil))
	if out["COLORTERM"] != "truecolor" {
		t.Errorf("COLORTERM = %q, want truecolor", out["COLORTERM"])
	}
}

func TestBuildPtyEnv_DefaultsLocaleWhenAbsent(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"HOME=/Users/mac"}))
	if out["LANG"] != "en_US.UTF-8" {
		t.Errorf("LANG = %q, want en_US.UTF-8", out["LANG"])
	}
	if out["LC_CTYPE"] != "en_US.UTF-8" {
		t.Errorf("LC_CTYPE = %q, want en_US.UTF-8", out["LC_CTYPE"])
	}
}

func TestBuildPtyEnv_DoesNotOverrideExistingLang(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"LANG=fr_FR.UTF-8"}))
	if out["LANG"] != "fr_FR.UTF-8" {
		t.Errorf("LANG = %q, want fr_FR.UTF-8 (should not be overridden)", out["LANG"])
	}
}

func TestBuildPtyEnv_DoesNotOverrideExistingLcCtype(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"LC_CTYPE=ja_JP.UTF-8"}))
	if out["LC_CTYPE"] != "ja_JP.UTF-8" {
		t.Errorf("LC_CTYPE = %q, want ja_JP.UTF-8 (should not be overridden even when LANG/LC_ALL are absent)", out["LC_CTYPE"])
	}
}

func TestBuildPtyEnv_DoesNotInjectLangWhenLcAllSet(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"LC_ALL=de_DE.UTF-8"}))
	if _, ok := out["LANG"]; ok {
		t.Errorf("LANG should not be injected when LC_ALL is already set, got %q", out["LANG"])
	}
	if out["LC_ALL"] != "de_DE.UTF-8" {
		t.Errorf("LC_ALL = %q, want de_DE.UTF-8 (should be preserved)", out["LC_ALL"])
	}
}

func TestBuildPtyEnv_StripsStaleDimensions(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"COLUMNS=300", "LINES=99"}))
	if _, ok := out["COLUMNS"]; ok {
		t.Error("COLUMNS should be stripped, but is present")
	}
	if _, ok := out["LINES"]; ok {
		t.Error("LINES should be stripped, but is present")
	}
}

func TestBuildPtyEnv_PreservesOtherVars(t *testing.T) {
	out := envMap(t, buildPtyEnv([]string{"HOME=/Users/mac", "PATH=/usr/bin:/bin", "SHELL=/bin/zsh"}))
	if out["HOME"] != "/Users/mac" {
		t.Errorf("HOME = %q, want /Users/mac", out["HOME"])
	}
	if out["PATH"] != "/usr/bin:/bin" {
		t.Errorf("PATH = %q, want /usr/bin:/bin", out["PATH"])
	}
	if out["SHELL"] != "/bin/zsh" {
		t.Errorf("SHELL = %q, want /bin/zsh", out["SHELL"])
	}
}

func TestBuildPtyEnv_NoDuplicateKeys(t *testing.T) {
	// A base slice with a pre-existing TERM must not result in two TERM=
	// entries after the override — envMap already fails on duplicates, so
	// simply exercising it here is the assertion.
	envMap(t, buildPtyEnv([]string{"TERM=screen", "HOME=/Users/mac", "TERM_PROGRAM=Apple_Terminal"}))
}
