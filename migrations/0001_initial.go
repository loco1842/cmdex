package migrations

import (
	"database/sql"
	"fmt"
)

var migration0001 = Migration{
	Version:     1,
	Description: "initial schema: all tables, FTS5, and triggers",
	Up: func(tx *sql.Tx) error {
		stmts := []string{
			`CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL
)`,
			`CREATE TABLE IF NOT EXISTS categories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    icon TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
			`CREATE TABLE IF NOT EXISTS commands (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    script_content TEXT NOT NULL,
    category_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
)`,
			`CREATE TABLE IF NOT EXISTS command_tags (
    command_id TEXT NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (command_id, tag_id),
    FOREIGN KEY (command_id) REFERENCES commands(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS variable_definitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    example TEXT NOT NULL DEFAULT '',
    default_expr TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (command_id) REFERENCES commands(id) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS variable_presets (
    id TEXT PRIMARY KEY,
    command_id TEXT NOT NULL,
    name TEXT NOT NULL,
    FOREIGN KEY (command_id) REFERENCES commands(id) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS preset_values (
    preset_id TEXT NOT NULL,
    variable_name TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (preset_id, variable_name),
    FOREIGN KEY (preset_id) REFERENCES variable_presets(id) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS executions (
    id TEXT PRIMARY KEY,
    command_id TEXT NOT NULL,
    script_content TEXT NOT NULL,
    final_cmd TEXT NOT NULL,
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0,
    executed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
			`CREATE TABLE IF NOT EXISTS app_settings (
    locale TEXT NOT NULL DEFAULT 'en',
    terminal TEXT NOT NULL DEFAULT ''
)`,
			`CREATE VIRTUAL TABLE IF NOT EXISTS commands_fts USING fts5(
    title, description, script_content, content='commands', content_rowid='rowid'
)`,
			`CREATE TRIGGER IF NOT EXISTS commands_ai AFTER INSERT ON commands BEGIN
    INSERT INTO commands_fts(rowid, title, description, script_content)
    VALUES (new.rowid, COALESCE(new.title, ''), COALESCE(new.description, ''), new.script_content);
END`,
			`CREATE TRIGGER IF NOT EXISTS commands_ad AFTER DELETE ON commands BEGIN
    INSERT INTO commands_fts(commands_fts, rowid, title, description, script_content)
    VALUES ('delete', old.rowid, COALESCE(old.title, ''), COALESCE(old.description, ''), old.script_content);
END`,
			`CREATE TRIGGER IF NOT EXISTS commands_au AFTER UPDATE ON commands BEGIN
    INSERT INTO commands_fts(commands_fts, rowid, title, description, script_content)
    VALUES ('delete', old.rowid, COALESCE(old.title, ''), COALESCE(old.description, ''), old.script_content);
    INSERT INTO commands_fts(rowid, title, description, script_content)
    VALUES (new.rowid, COALESCE(new.title, ''), COALESCE(new.description, ''), new.script_content);
END`,
		}
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return fmt.Errorf("migration 0001 up: %w", err)
			}
		}
		return nil
	},
	// Down drops all tables in reverse FK dependency order.
	// This is a dev/test utility — data will be lost.
	Down: func(tx *sql.Tx) error {
		stmts := []string{
			`DROP TRIGGER IF EXISTS commands_au`,
			`DROP TRIGGER IF EXISTS commands_ad`,
			`DROP TRIGGER IF EXISTS commands_ai`,
			`DROP TABLE IF EXISTS commands_fts`,
			`DROP TABLE IF EXISTS preset_values`,
			`DROP TABLE IF EXISTS variable_presets`,
			`DROP TABLE IF EXISTS variable_definitions`,
			`DROP TABLE IF EXISTS command_tags`,
			`DROP TABLE IF EXISTS executions`,
			`DROP TABLE IF EXISTS app_settings`,
			`DROP TABLE IF EXISTS commands`,
			`DROP TABLE IF EXISTS tags`,
			`DROP TABLE IF EXISTS categories`,
			`DROP TABLE IF EXISTS schema_version`,
		}
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return fmt.Errorf("migration 0001 down: %w", err)
			}
		}
		return nil
	},
}
