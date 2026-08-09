package main

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	return &DB{conn: conn}
}

func TestFreshDBMigrations(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	if err := db.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	var version int
	if err := db.conn.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != 10 {
		t.Errorf("schema_version = %d, want 10", version)
	}

	expectedTables := []string{
		"categories", "commands", "tags", "command_tags",
		"variable_definitions", "variable_presets", "preset_values",
		"executions", "app_settings", "schema_version",
	}
	for _, table := range expectedTables {
		var name string
		err := db.conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q not found: %v", table, err)
		}
	}
}

func TestExistingDBIdempotent(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Seed schema_version to 10 without creating tables
	if _, err := db.conn.Exec("CREATE TABLE schema_version (version INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if _, err := db.conn.Exec("INSERT INTO schema_version (version) VALUES (10)"); err != nil {
		t.Fatalf("insert schema_version: %v", err)
	}

	if err := db.runMigrations(); err != nil {
		t.Fatalf("runMigrations on existing db failed: %v", err)
	}

	var version int
	if err := db.conn.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != 10 {
		t.Errorf("schema_version = %d, want 10", version)
	}
}

func TestRollbackTo(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	if err := db.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	if err := db.RollbackTo(5); err != nil {
		t.Fatalf("RollbackTo(5) failed: %v", err)
	}

	var version int
	if err := db.conn.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != 5 {
		t.Errorf("schema_version = %d, want 5", version)
	}

	// Verify core tables still exist after partial rollback
	expectedTables := []string{
		"categories", "commands", "tags", "command_tags",
		"variable_definitions", "variable_presets", "preset_values",
		"executions", "app_settings", "schema_version",
	}
	for _, table := range expectedTables {
		var name string
		err := db.conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q not found after rollback: %v", table, err)
		}
	}
}

func TestUpdateCommandCategoryChange(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	if err := db.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	catA := Category{ID: "cat-a", Name: "A", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	catB := Category{ID: "cat-b", Name: "B", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.CreateCategory(catA); err != nil {
		t.Fatalf("create category A: %v", err)
	}
	if err := db.CreateCategory(catB); err != nil {
		t.Fatalf("create category B: %v", err)
	}

	existingInB := Command{
		ID:            "cmd-b-existing",
		Title:         sql.NullString{String: "already in B", Valid: true},
		ScriptContent: "echo existing",
		CategoryID:    catB.ID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := db.CreateCommand(existingInB); err != nil {
		t.Fatalf("create existing command in B: %v", err)
	}

	cmd := Command{
		ID:            "cmd-1",
		Title:         sql.NullString{String: "original", Valid: true},
		ScriptContent: "echo hi",
		CategoryID:    catA.ID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := db.CreateCommand(cmd); err != nil {
		t.Fatalf("create command: %v", err)
	}

	// Category change must reindex via UpdateCommandPosition, then update fields.
	cmd.Title = sql.NullString{String: "updated", Valid: true}
	cmd.CategoryID = catB.ID
	cmd.UpdatedAt = time.Now()
	if err := db.UpdateCommand(cmd); err != nil {
		t.Fatalf("UpdateCommand with category change failed: %v", err)
	}

	got, err := db.GetCommand(cmd.ID)
	if err != nil {
		t.Fatalf("GetCommand after update: %v", err)
	}
	if got.CategoryID != catB.ID {
		t.Errorf("CategoryID = %q, want %q", got.CategoryID, catB.ID)
	}
	if got.Title.String != "updated" {
		t.Errorf("Title = %q, want %q", got.Title.String, "updated")
	}
	// Appended after the existing command in catB (positions are 0-indexed).
	if got.Position != 1 {
		t.Errorf("Position = %d, want 1 (after existing command in B)", got.Position)
	}
}

func TestGenerateScript(t *testing.T) {
	// Basic body
	result := GenerateScript("echo hello")
	expected := "echo hello\n"
	if result != expected {
		t.Errorf("GenerateScript: got %q, want %q", result, expected)
	}

	// Empty body
	result = GenerateScript("")
	if result != "" {
		t.Errorf("GenerateScript empty: got %q, want empty", result)
	}

	// Whitespace-only body
	result = GenerateScript("  \n  ")
	if result != "" {
		t.Errorf("GenerateScript whitespace: got %q, want empty", result)
	}

	// Multi-line body
	result = GenerateScript("echo one\necho two")
	expected = "echo one\necho two\n"
	if result != expected {
		t.Errorf("GenerateScript multi-line: got %q, want %q", result, expected)
	}

	// Ensure no shebang prefix
	if len(result) >= 2 && result[:2] == "#!" {
		t.Errorf("GenerateScript contains shebang prefix: %q", result)
	}
}

func TestParseScriptBody(t *testing.T) {
	// Old format: #!/bin/bash shebang
	result := ParseScriptBody("#!/bin/bash\n\necho hello\n")
	expected := "echo hello"
	if result != expected {
		t.Errorf("ParseScriptBody old format: got %q, want %q", result, expected)
	}

	// Old format with #!/usr/bin/env bash
	result = ParseScriptBody("#!/usr/bin/env bash\n\necho hello\n")
	expected = "echo hello"
	if result != expected {
		t.Errorf("ParseScriptBody env bash: got %q, want %q", result, expected)
	}

	// New format: no shebang
	result = ParseScriptBody("echo hello\n")
	expected = "echo hello"
	if result != expected {
		t.Errorf("ParseScriptBody no shebang: got %q, want %q", result, expected)
	}

	// Empty content
	result = ParseScriptBody("")
	if result != "" {
		t.Errorf("ParseScriptBody empty: got %q, want empty", result)
	}

	// Only shebang, no body
	result = ParseScriptBody("#!/bin/bash\n")
	if result != "" {
		t.Errorf("ParseScriptBody shebang only: got %q, want empty", result)
	}

	// Shebang with no trailing newline
	result = ParseScriptBody("#!/bin/bash")
	if result != "" {
		t.Errorf("ParseScriptBody shebang no newline: got %q, want empty", result)
	}
}

func TestExtractTemplateVars(t *testing.T) {
	vars := ExtractTemplateVars("echo {{name}} {{greeting}}")
	if len(vars) != 2 || vars[0] != "name" || vars[1] != "greeting" {
		t.Errorf("ExtractTemplateVars: got %v, want [name greeting]", vars)
	}

	// Duplicate vars should be deduplicated
	vars = ExtractTemplateVars("{{x}} {{x}}")
	if len(vars) != 1 || vars[0] != "x" {
		t.Errorf("ExtractTemplateVars dedup: got %v, want [x]", vars)
	}

	// No vars
	vars = ExtractTemplateVars("echo hello")
	if len(vars) != 0 {
		t.Errorf("ExtractTemplateVars none: got %v, want []", vars)
	}
}

func TestReplaceTemplateVars(t *testing.T) {
	values := map[string]string{"name": "world", "greeting": "hello"}
	result := ReplaceTemplateVars("echo {{greeting}} {{name}}", values)
	expected := "echo hello world"
	if result != expected {
		t.Errorf("ReplaceTemplateVars: got %q, want %q", result, expected)
	}

	// Unreplaced vars left as-is
	result = ReplaceTemplateVars("echo {{unknown}}", values)
	if result != "echo {{unknown}}" {
		t.Errorf("ReplaceTemplateVars unknown: got %q, want %q", result, "echo {{unknown}}")
	}

	// Empty values
	result = ReplaceTemplateVars("echo hi", map[string]string{})
	if result != "echo hi" {
		t.Errorf("ReplaceTemplateVars empty: got %q, want %q", result, "echo hi")
	}
}

func TestMergeDetectedVars(t *testing.T) {
	existing := []VariableDefinition{
		{Name: "name", SortOrder: 0},
		{Name: "manual", SortOrder: 1},
	}
	detected := []string{"name", "auto"}
	result := MergeDetectedVars(detected, existing)
	if len(result) != 3 {
		t.Errorf("MergeDetectedVars len: got %d, want 3", len(result))
	}
	if result[0].Name != "name" || result[1].Name != "auto" || result[2].Name != "manual" {
		t.Errorf("MergeDetectedVars order: got %v", result)
	}
}
