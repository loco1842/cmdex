package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"cmdex/migrations"
)

type DB struct {
	conn    *sql.DB
	dataDir string
}

// appendToEndPosition is an out-of-range index passed to UpdateCommandPosition
// to mean "insert after the last command" — it clamps to len(ordered).
const appendToEndPosition = 999999

func NewDB() (*DB, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	dataDir := filepath.Join(homeDir, ".cmdex")
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.Chmod(dataDir, 0750); err != nil { //nolint:gosec // G302: 0750 is correct for a dir, not a file
		return nil, fmt.Errorf("chmod data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "cmdex.db")
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(wal)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db := &DB{conn: conn, dataDir: dataDir}
	if err := db.runMigrations(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) runMigrations() error {
	var currentVersion int
	err := db.conn.QueryRowContext(context.Background(), "SELECT version FROM schema_version LIMIT 1").
		Scan(&currentVersion)
	if err != nil {
		currentVersion = 0
		if err == sql.ErrNoRows {
			if _, err := db.conn.ExecContext(
				context.Background(),
				"INSERT INTO schema_version (version) VALUES (0)",
			); err != nil {
				return fmt.Errorf("insert initial schema_version: %w", err)
			}
		} else {
			if _, err := db.conn.ExecContext(context.Background(),
				"CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)",
			); err != nil {
				return fmt.Errorf("create schema_version table: %w", err)
			}
			if _, err := db.conn.ExecContext(
				context.Background(),
				"INSERT INTO schema_version (version) VALUES (0)",
			); err != nil {
				return fmt.Errorf("insert initial schema_version: %w", err)
			}
		}
	}

	for _, m := range migrations.Migrations {
		if m.Version <= currentVersion {
			continue
		}

		if m.DisableFKDuringMigration {
			if _, err := db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = OFF"); err != nil {
				return fmt.Errorf("disable FK for migration %d: %w", m.Version, err)
			}
		}

		tx, err := db.conn.BeginTx(context.Background(), nil)
		if err != nil {
			if m.DisableFKDuringMigration {
				_, _ = db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
			}
			return fmt.Errorf("begin migration %d tx: %w", m.Version, err)
		}

		if err := m.Up(tx); err != nil {
			_ = tx.Rollback()
			if m.DisableFKDuringMigration {
				_, _ = db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
			}
			return fmt.Errorf("migration %d (%s) up: %w", m.Version, m.Description, err)
		}

		if _, err := tx.ExecContext(
			context.Background(),
			"UPDATE schema_version SET version = ?",
			m.Version,
		); err != nil {
			_ = tx.Rollback()
			if m.DisableFKDuringMigration {
				_, _ = db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
			}
			return fmt.Errorf("update schema_version for migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			if m.DisableFKDuringMigration {
				_, _ = db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
			}
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}

		if m.DisableFKDuringMigration {
			if _, err := db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); err != nil {
				return fmt.Errorf("re-enable FK after migration %d: %w", m.Version, err)
			}
		}
	}

	return nil
}

// RollbackTo steps the schema back to the target version.
// It is a dev/testing utility and is not exposed to the frontend.
func (db *DB) RollbackTo(targetVersion int) error {
	var currentVersion int
	err := db.conn.QueryRowContext(context.Background(), "SELECT version FROM schema_version LIMIT 1").
		Scan(&currentVersion)
	if err != nil {
		return errors.New("schema_version not initialized")
	}

	if targetVersion >= currentVersion {
		return nil
	}

	// Iterate in reverse order, only undoing migrations that are actually
	// applied (targetVersion < m.Version <= currentVersion) — a migration
	// with Version > currentVersion was never run and must not be rolled back.
	for _, v := range slices.Backward(migrations.Migrations) {
		m := v
		if m.Version <= targetVersion || m.Version > currentVersion {
			continue
		}

		if m.DisableFKDuringMigration {
			if _, err := db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = OFF"); err != nil {
				return fmt.Errorf("disable FK for rollback %d: %w", m.Version, err)
			}
		}

		tx, err := db.conn.BeginTx(context.Background(), nil)
		if err != nil {
			if m.DisableFKDuringMigration {
				_, _ = db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
			}
			return fmt.Errorf("begin rollback %d tx: %w", m.Version, err)
		}

		if err := m.Down(tx); err != nil {
			_ = tx.Rollback()
			if m.DisableFKDuringMigration {
				_, _ = db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
			}
			return fmt.Errorf("rollback migration %d (%s) down: %w", m.Version, m.Description, err)
		}

		if err := tx.Commit(); err != nil {
			if m.DisableFKDuringMigration {
				_, _ = db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
			}
			return fmt.Errorf("commit rollback %d: %w", m.Version, err)
		}

		if m.DisableFKDuringMigration {
			if _, err := db.conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON"); err != nil {
				return fmt.Errorf("re-enable FK after rollback %d: %w", m.Version, err)
			}
		}
	}

	if _, err := db.conn.ExecContext(
		context.Background(),
		"UPDATE schema_version SET version = ?",
		targetVersion,
	); err != nil {
		return fmt.Errorf("update schema_version after rollback: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

func (db *DB) GetCategories() ([]Category, error) {
	rows, err := db.conn.QueryContext(context.Background(),
		"SELECT id, name, icon, color, created_at, updated_at FROM categories ORDER BY created_at",
	)
	if err != nil {
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer rows.Close()

	cats := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.Color, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func (db *DB) CreateCategory(cat Category) error {
	_, err := db.conn.ExecContext(context.Background(),
		"INSERT INTO categories (id, name, icon, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		cat.ID, cat.Name, cat.Icon, cat.Color, cat.CreatedAt, cat.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert category: %w", err)
	}
	return nil
}

func (db *DB) UpdateCategory(cat Category) error {
	res, err := db.conn.ExecContext(context.Background(),
		"UPDATE categories SET name = ?, icon = ?, color = ?, updated_at = ? WHERE id = ?",
		cat.Name, cat.Icon, cat.Color, cat.UpdatedAt, cat.ID,
	)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("category %s not found", cat.ID)
	}
	return nil
}

func (db *DB) DeleteCategory(id string) error {
	res, err := db.conn.ExecContext(context.Background(), "DELETE FROM categories WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("category %s not found", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (db *DB) GetCommands() ([]Command, error) {
	rows, err := db.conn.QueryContext(
		context.Background(),
		"SELECT id, title, description, script_content, category_id, position, created_at, updated_at, working_dir FROM commands ORDER BY position ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("query commands: %w", err)
	}
	defer rows.Close()

	cmds := []Command{}
	for rows.Next() {
		var c Command
		var catID sql.NullString
		var workingDirRaw string
		if err := rows.Scan(
			&c.ID,
			&c.Title,
			&c.Description,
			&c.ScriptContent,
			&catID,
			&c.Position,
			&c.CreatedAt,
			&c.UpdatedAt,
			&workingDirRaw,
		); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		c.CategoryID = catID.String
		if err := json.Unmarshal([]byte(workingDirRaw), &c.WorkingDir); err != nil {
			return nil, fmt.Errorf("unmarshal working_dir: %w", err)
		}
		cmds = append(cmds, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range cmds {
		if err := db.loadCommandRelations(&cmds[i]); err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

func (db *DB) GetCommandsByCategory(categoryID string) ([]Command, error) {
	rows, err := db.conn.QueryContext(
		context.Background(),
		"SELECT id, title, description, script_content, category_id, position, created_at, updated_at, working_dir FROM commands WHERE category_id = ? ORDER BY position ASC",
		categoryID,
	)
	if err != nil {
		return nil, fmt.Errorf("query commands by category: %w", err)
	}
	defer rows.Close()

	cmds := []Command{}
	for rows.Next() {
		var c Command
		var catID sql.NullString
		var workingDirRaw string
		if err := rows.Scan(
			&c.ID,
			&c.Title,
			&c.Description,
			&c.ScriptContent,
			&catID,
			&c.Position,
			&c.CreatedAt,
			&c.UpdatedAt,
			&workingDirRaw,
		); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		c.CategoryID = catID.String
		if err := json.Unmarshal([]byte(workingDirRaw), &c.WorkingDir); err != nil {
			return nil, fmt.Errorf("unmarshal working_dir: %w", err)
		}
		cmds = append(cmds, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range cmds {
		if err := db.loadCommandRelations(&cmds[i]); err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

func (db *DB) GetCommandsByIDs(ids []string) ([]Command, error) {
	if len(ids) == 0 {
		return []Command{}, nil
	}

	cmds := make([]Command, 0, len(ids))
	for _, id := range ids {
		cmd, err := db.GetCommand(id)
		if err != nil {
			// Skip missing commands, continue with others
			continue
		}
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}

// ImportPresetInput is the input format for importing a preset.
type ImportPresetInput struct {
	Name   string
	Values map[string]string
}

// ImportCommandInput is the input format for importing a command.
type ImportCommandInput struct {
	Title         string
	Description   string
	ScriptContent string
	Tags          []string
	Variables     []VariableDefinition
	Presets       []ImportPresetInput
	WorkingDir    OSPathMap
	CategoryName  string
}

// ImportCommands imports commands from import data structure
func (db *DB) ImportCommands(commands []ImportCommandInput) error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Build category name -> ID map
	categoryMap := make(map[string]string)
	// Closed explicitly (not deferred) below: further writes happen on the
	// same tx afterward and must not race an open Rows on it.
	rows, err := tx.QueryContext(context.Background(), "SELECT id, name FROM categories")
	if err != nil {
		return fmt.Errorf("query categories: %w", err)
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close() //nolint:sqlclosecheck // see comment above the query
			return fmt.Errorf("scan category: %w", err)
		}
		categoryMap[name] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close() //nolint:sqlclosecheck // see comment above the query
		return fmt.Errorf("iterate categories: %w", err)
	}
	_ = rows.Close() //nolint:sqlclosecheck // see comment above the query

	for _, importedCmd := range commands {
		catID := ""
		if importedCmd.CategoryName != "" {
			if existing, exists := categoryMap[importedCmd.CategoryName]; exists {
				catID = existing
			} else {
				newCatID := uuid.New().String()
				now := time.Now()
				_, err := tx.ExecContext(context.Background(),
					"INSERT INTO categories (id, name, icon, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
					newCatID, importedCmd.CategoryName, "", "", now, now,
				)
				if err != nil {
					return fmt.Errorf("create category: %w", err)
				}
				catID = newCatID
				categoryMap[importedCmd.CategoryName] = newCatID
			}
		}

		cmdID := uuid.New().String()
		now := time.Now()

		var maxPos int
		if catID != "" {
			err = tx.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(position), -1) FROM commands WHERE category_id = ?", catID).
				Scan(&maxPos)
		} else {
			err = tx.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(position), -1) FROM commands WHERE category_id IS NULL OR category_id = ''").
				Scan(&maxPos)
		}
		if err != nil {
			return fmt.Errorf("query max position: %w", err)
		}

		workingDirJSON, err := json.Marshal(importedCmd.WorkingDir)
		if err != nil {
			return fmt.Errorf("marshal working_dir: %w", err)
		}

		_, err = tx.ExecContext(
			context.Background(),
			`INSERT INTO commands (id, title, description, script_content, category_id, position, created_at, updated_at, working_dir)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			cmdID,
			nullableString(importedCmd.Title),
			nullableString(importedCmd.Description),
			importedCmd.ScriptContent,
			nullableString(catID),
			maxPos+1,
			now,
			now,
			string(workingDirJSON),
		)
		if err != nil {
			return fmt.Errorf("insert command: %w", err)
		}

		// Save tags
		for _, tag := range importedCmd.Tags {
			if tag == "" {
				continue
			}
			_, err := tx.ExecContext(context.Background(), "INSERT OR IGNORE INTO tags (name) VALUES (?)", tag)
			if err != nil {
				return fmt.Errorf("upsert tag: %w", err)
			}
			_, err = tx.ExecContext(context.Background(),
				"INSERT OR IGNORE INTO command_tags (command_id, tag_id) SELECT ?, id FROM tags WHERE name = ?",
				cmdID, tag,
			)
			if err != nil {
				return fmt.Errorf("link tag: %w", err)
			}
		}

		// Save variables
		for i, v := range importedCmd.Variables {
			_, err := tx.ExecContext(
				context.Background(),
				"INSERT INTO variable_definitions (command_id, name, description, example, default_expr, sort_order) VALUES (?, ?, ?, ?, ?, ?)",
				cmdID,
				v.Name,
				v.Description,
				v.Example,
				v.Default,
				i,
			)
			if err != nil {
				return fmt.Errorf("insert variable: %w", err)
			}
		}

		// Save presets
		for _, p := range importedCmd.Presets {
			presetID := uuid.New().String()
			_, err := tx.ExecContext(context.Background(),
				"INSERT INTO variable_presets (id, command_id, name, position) VALUES (?, ?, ?, ?)",
				presetID, cmdID, p.Name, 0,
			)
			if err != nil {
				return fmt.Errorf("insert preset: %w", err)
			}
			for k, v := range p.Values {
				_, err := tx.ExecContext(context.Background(),
					"INSERT INTO preset_values (preset_id, variable_name, value) VALUES (?, ?, ?)",
					presetID, k, v,
				)
				if err != nil {
					return fmt.Errorf("insert preset value: %w", err)
				}
			}
		}
	}

	return tx.Commit()
}

func (db *DB) GetCommand(id string) (Command, error) {
	var c Command
	var catID sql.NullString
	var workingDirRaw string
	err := db.conn.QueryRowContext(
		context.Background(),
		"SELECT id, title, description, script_content, category_id, position, created_at, updated_at, working_dir FROM commands WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Title, &c.Description, &c.ScriptContent, &catID, &c.Position, &c.CreatedAt, &c.UpdatedAt, &workingDirRaw)
	if err != nil {
		return c, fmt.Errorf("get command %s: %w", id, err)
	}
	c.CategoryID = catID.String
	if err := json.Unmarshal([]byte(workingDirRaw), &c.WorkingDir); err != nil {
		return c, fmt.Errorf("unmarshal working_dir: %w", err)
	}
	if err := db.loadCommandRelations(&c); err != nil {
		return c, err
	}
	return c, nil
}

func (db *DB) loadCommandRelations(cmd *Command) error {
	// Tags
	tagRows, err := db.conn.QueryContext(context.Background(),
		"SELECT t.name FROM tags t JOIN command_tags ct ON ct.tag_id = t.id WHERE ct.command_id = ? ORDER BY t.name",
		cmd.ID,
	)
	if err != nil {
		return fmt.Errorf("query tags: %w", err)
	}
	defer tagRows.Close()

	cmd.Tags = []string{}
	for tagRows.Next() {
		var name string
		if err := tagRows.Scan(&name); err != nil {
			return fmt.Errorf("scan tag: %w", err)
		}
		cmd.Tags = append(cmd.Tags, name)
	}
	if err := tagRows.Err(); err != nil {
		return err
	}

	// Variables
	varRows, err := db.conn.QueryContext(
		context.Background(),
		"SELECT name, description, example, default_expr, sort_order FROM variable_definitions WHERE command_id = ? ORDER BY sort_order",
		cmd.ID,
	)
	if err != nil {
		return fmt.Errorf("query variables: %w", err)
	}
	defer varRows.Close()

	cmd.Variables = []VariableDefinition{}
	for varRows.Next() {
		var v VariableDefinition
		if err := varRows.Scan(&v.Name, &v.Description, &v.Example, &v.Default, &v.SortOrder); err != nil {
			return fmt.Errorf("scan variable: %w", err)
		}
		cmd.Variables = append(cmd.Variables, v)
	}
	if err := varRows.Err(); err != nil {
		return err
	}

	// Presets — ORDER BY position must match GetPresets / ReorderPresets
	presetRows, err := db.conn.QueryContext(context.Background(),
		"SELECT id, name, position FROM variable_presets WHERE command_id = ? ORDER BY position, name",
		cmd.ID,
	)
	if err != nil {
		return fmt.Errorf("query presets: %w", err)
	}
	defer presetRows.Close()

	cmd.Presets = []VariablePreset{}
	for presetRows.Next() {
		var p VariablePreset
		if err := presetRows.Scan(&p.ID, &p.Name, &p.Position); err != nil {
			return fmt.Errorf("scan preset: %w", err)
		}
		p.Values = map[string]string{}

		// Closed explicitly (not deferred): this runs inside the presetRows
		// loop, so deferring would keep one Rows open per preset until return.
		valRows, err := db.conn.QueryContext(context.Background(),
			"SELECT variable_name, value FROM preset_values WHERE preset_id = ?", p.ID,
		)
		if err != nil {
			return fmt.Errorf("query preset values: %w", err)
		}
		for valRows.Next() {
			var k, v string
			if err := valRows.Scan(&k, &v); err != nil {
				_ = valRows.Close() //nolint:sqlclosecheck // see comment above the query
				return fmt.Errorf("scan preset value: %w", err)
			}
			p.Values[k] = v
		}
		_ = valRows.Close() //nolint:sqlclosecheck // see comment above the query
		if err := valRows.Err(); err != nil {
			return err
		}

		cmd.Presets = append(cmd.Presets, p)
	}
	return presetRows.Err()
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableNullString(ns sql.NullString) any {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	return ns.String
}

func (db *DB) CreateCommand(cmd Command) error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	workingDirJSON, err := json.Marshal(cmd.WorkingDir)
	if err != nil {
		return fmt.Errorf("marshal working_dir: %w", err)
	}

	_, err = tx.ExecContext(
		context.Background(),
		`INSERT INTO commands (id, title, description, script_content, category_id, position, created_at, updated_at, working_dir)
		 VALUES (?, ?, ?, ?, ?, COALESCE((SELECT MAX(position)+1 FROM commands WHERE category_id IS ?), 0), ?, ?, ?)`,
		cmd.ID,
		nullableNullString(cmd.Title),
		nullableNullString(cmd.Description),
		cmd.ScriptContent,
		nullableString(cmd.CategoryID),
		nullableString(cmd.CategoryID),
		cmd.CreatedAt,
		cmd.UpdatedAt,
		string(workingDirJSON),
	)
	if err != nil {
		return fmt.Errorf("insert command: %w", err)
	}

	if err := db.saveTags(tx, cmd.ID, cmd.Tags); err != nil {
		return err
	}
	if err := db.saveVariables(tx, cmd.ID, cmd.Variables); err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) UpdateCommand(cmd Command) error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var oldCategoryID sql.NullString
	err = tx.QueryRowContext(context.Background(), "SELECT category_id FROM commands WHERE id = ?", cmd.ID).
		Scan(&oldCategoryID)
	if err != nil {
		return fmt.Errorf("get existing command: %w", err)
	}

	// Category moves need reindexing; share this tx with the field/tag/var writes below.
	if oldCategoryID.String != cmd.CategoryID {
		if err := db.updateCommandPositionTx(tx, cmd.ID, cmd.CategoryID, appendToEndPosition); err != nil {
			return err
		}
	}

	workingDirJSON, err := json.Marshal(cmd.WorkingDir)
	if err != nil {
		return fmt.Errorf("marshal working_dir: %w", err)
	}

	_, err = tx.ExecContext(
		context.Background(),
		"UPDATE commands SET title = ?, description = ?, script_content = ?, category_id = ?, updated_at = ?, working_dir = ? WHERE id = ?",
		nullableNullString(cmd.Title),
		nullableNullString(cmd.Description),
		cmd.ScriptContent,
		nullableString(cmd.CategoryID),
		cmd.UpdatedAt,
		string(workingDirJSON),
		cmd.ID,
	)
	if err != nil {
		return fmt.Errorf("update command: %w", err)
	}

	// Replace tags
	if _, err := tx.ExecContext(
		context.Background(),
		"DELETE FROM command_tags WHERE command_id = ?",
		cmd.ID,
	); err != nil {
		return fmt.Errorf("delete old tags: %w", err)
	}
	if err := db.saveTags(tx, cmd.ID, cmd.Tags); err != nil {
		return err
	}

	// Replace variables
	if _, err := tx.ExecContext(
		context.Background(),
		"DELETE FROM variable_definitions WHERE command_id = ?",
		cmd.ID,
	); err != nil {
		return fmt.Errorf("delete old variables: %w", err)
	}
	if err := db.saveVariables(tx, cmd.ID, cmd.Variables); err != nil {
		return err
	}

	// Clean up stale preset values for variables that no longer exist
	_, err = tx.ExecContext(context.Background(), `
		DELETE FROM preset_values
		WHERE preset_id IN (
			SELECT id FROM variable_presets WHERE command_id = ?
		)
		AND NOT EXISTS (
			SELECT 1 FROM variable_definitions
			WHERE command_id = ? AND name = preset_values.variable_name
		)
	`, cmd.ID, cmd.ID)
	if err != nil {
		return fmt.Errorf("cleanup stale preset values: %w", err)
	}

	return tx.Commit()
}

func (db *DB) RenameCommand(id string, title string) error {
	res, err := db.conn.ExecContext(context.Background(),
		"UPDATE commands SET title = ?, updated_at = ? WHERE id = ?",
		nullableString(title), time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("rename command: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("command %s not found", id)
	}
	return nil
}

func (db *DB) DeleteCommand(id string) error {
	res, err := db.conn.ExecContext(context.Background(), "DELETE FROM commands WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete command: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("command %s not found", id)
	}
	return nil
}

// UpdateCommandPosition moves a command to a new category and normalizes
// positions within the target category so they are 0-indexed with no gaps.
// newCategoryID may be empty string (uncategorized).
// newIndex is the 0-based insertion index within the target category.
func (db *DB) UpdateCommandPosition(id string, newCategoryID string, newIndex int) error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := db.updateCommandPositionTx(tx, id, newCategoryID, newIndex); err != nil {
		return err
	}
	return tx.Commit()
}

// updateCommandPositionTx is the transactional core of UpdateCommandPosition.
// Callers that already hold a transaction (e.g. UpdateCommand) use this so the
// category move shares the same commit/rollback path as later writes.
func (db *DB) updateCommandPositionTx(tx *sql.Tx, id string, newCategoryID string, newIndex int) error {
	// Fetch all commands in the target category, excluding the moving one
	var rows *sql.Rows
	var err error
	if newCategoryID == "" {
		rows, err = tx.QueryContext(context.Background(),
			"SELECT id FROM commands WHERE (category_id IS NULL OR category_id = '') AND id != ? ORDER BY position ASC",
			id,
		)
	} else {
		rows, err = tx.QueryContext(context.Background(),
			"SELECT id FROM commands WHERE category_id = ? AND id != ? ORDER BY position ASC",
			newCategoryID, id,
		)
	}
	if err != nil {
		return fmt.Errorf("query commands for reorder: %w", err)
	}

	var ordered []string
	for rows.Next() {
		var rowID string
		if err := rows.Scan(&rowID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan id: %w", err)
		}
		ordered = append(ordered, rowID)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Insert the moving command at newIndex (clamped to valid range)
	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex > len(ordered) {
		newIndex = len(ordered)
	}
	tail := make([]string, len(ordered)-newIndex)
	copy(tail, ordered[newIndex:])
	ordered = append(ordered[:newIndex], append([]string{id}, tail...)...)

	// Write positions 0, 1, 2, … and update category
	for i, cmdID := range ordered {
		var execErr error
		if newCategoryID == "" {
			_, execErr = tx.ExecContext(context.Background(),
				"UPDATE commands SET position = ?, category_id = NULL WHERE id = ?",
				i, cmdID,
			)
		} else {
			_, execErr = tx.ExecContext(context.Background(),
				"UPDATE commands SET position = ?, category_id = ? WHERE id = ?",
				i, newCategoryID, cmdID,
			)
		}
		if execErr != nil {
			return fmt.Errorf("update position for %s: %w", cmdID, execErr)
		}
	}

	return nil
}

func (db *DB) saveTags(tx *sql.Tx, commandID string, tags []string) error {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		// Upsert tag
		_, err := tx.ExecContext(context.Background(), "INSERT OR IGNORE INTO tags (name) VALUES (?)", tag)
		if err != nil {
			return fmt.Errorf("upsert tag: %w", err)
		}
		// Link
		_, err = tx.ExecContext(context.Background(),
			"INSERT OR IGNORE INTO command_tags (command_id, tag_id) SELECT ?, id FROM tags WHERE name = ?",
			commandID, tag,
		)
		if err != nil {
			return fmt.Errorf("link tag: %w", err)
		}
	}
	return nil
}

func (db *DB) saveVariables(tx *sql.Tx, commandID string, vars []VariableDefinition) error {
	for i, v := range vars {
		_, err := tx.ExecContext(
			context.Background(),
			"INSERT INTO variable_definitions (command_id, name, description, example, default_expr, sort_order) VALUES (?, ?, ?, ?, ?, ?)",
			commandID,
			v.Name,
			v.Description,
			v.Example,
			v.Default,
			i,
		)
		if err != nil {
			return fmt.Errorf("insert variable %s: %w", v.Name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Presets
// ---------------------------------------------------------------------------

func (db *DB) GetPresets(commandID string) ([]VariablePreset, error) {
	rows, err := db.conn.QueryContext(context.Background(),
		"SELECT id, name, position FROM variable_presets WHERE command_id = ? ORDER BY position, name",
		commandID,
	)
	if err != nil {
		return nil, fmt.Errorf("query presets: %w", err)
	}
	defer rows.Close()

	presets := []VariablePreset{}
	for rows.Next() {
		var p VariablePreset
		if err := rows.Scan(&p.ID, &p.Name, &p.Position); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}
		p.Values = map[string]string{}

		// Closed explicitly (not deferred): this runs inside the rows loop
		// above, so deferring would keep one Rows open per preset until return.
		valRows, err := db.conn.QueryContext(
			context.Background(),
			"SELECT variable_name, value FROM preset_values WHERE preset_id = ?",
			p.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("query preset values: %w", err)
		}
		for valRows.Next() {
			var k, v string
			if err := valRows.Scan(&k, &v); err != nil {
				_ = valRows.Close() //nolint:sqlclosecheck // see comment above the query
				return nil, fmt.Errorf("scan preset value: %w", err)
			}
			p.Values[k] = v
		}
		_ = valRows.Close() //nolint:sqlclosecheck // see comment above the query
		if err := valRows.Err(); err != nil {
			return nil, err
		}

		presets = append(presets, p)
	}
	return presets, rows.Err()
}

func (db *DB) SavePreset(commandID string, preset VariablePreset) error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var maxPos int
	_ = tx.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(position), -1) FROM variable_presets WHERE command_id = ?", commandID).
		Scan(&maxPos)

	_, err = tx.ExecContext(
		context.Background(),
		"INSERT INTO variable_presets (id, command_id, name, position) VALUES (?, ?, ?, ?)",
		preset.ID,
		commandID,
		preset.Name,
		maxPos+1,
	)
	if err != nil {
		return fmt.Errorf("insert preset: %w", err)
	}

	for k, v := range preset.Values {
		_, err = tx.ExecContext(
			context.Background(),
			"INSERT INTO preset_values (preset_id, variable_name, value) VALUES (?, ?, ?)",
			preset.ID,
			k,
			v,
		)
		if err != nil {
			return fmt.Errorf("insert preset value: %w", err)
		}
	}

	return tx.Commit()
}

func (db *DB) UpdatePreset(preset VariablePreset) error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(
		context.Background(),
		"UPDATE variable_presets SET name = ? WHERE id = ?",
		preset.Name,
		preset.ID,
	)
	if err != nil {
		return fmt.Errorf("update preset: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("preset %s not found", preset.ID)
	}

	// Replace values
	if _, err := tx.ExecContext(
		context.Background(),
		"DELETE FROM preset_values WHERE preset_id = ?",
		preset.ID,
	); err != nil {
		return fmt.Errorf("delete old preset values: %w", err)
	}
	for k, v := range preset.Values {
		_, err = tx.ExecContext(
			context.Background(),
			"INSERT INTO preset_values (preset_id, variable_name, value) VALUES (?, ?, ?)",
			preset.ID,
			k,
			v,
		)
		if err != nil {
			return fmt.Errorf("insert preset value: %w", err)
		}
	}

	return tx.Commit()
}

func (db *DB) DeletePreset(presetID string) error {
	res, err := db.conn.ExecContext(context.Background(), "DELETE FROM variable_presets WHERE id = ?", presetID)
	if err != nil {
		return fmt.Errorf("delete preset: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("preset %s not found", presetID)
	}
	return nil
}

func (db *DB) ReorderPresets(commandID string, presetIDs []string) error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, id := range presetIDs {
		if _, err := tx.ExecContext(context.Background(),
			"UPDATE variable_presets SET position = ? WHERE id = ? AND command_id = ?",
			i,
			id,
			commandID,
		); err != nil {
			return fmt.Errorf("reorder preset: %w", err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Executions
// ---------------------------------------------------------------------------

func (db *DB) GetExecutions() ([]ExecutionRecord, error) {
	rows, err := db.conn.QueryContext(
		context.Background(),
		"SELECT id, command_id, script_content, final_cmd, output, error, exit_code, working_dir, executed_at FROM executions ORDER BY executed_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("query executions: %w", err)
	}
	defer rows.Close()

	records := []ExecutionRecord{}
	for rows.Next() {
		var r ExecutionRecord
		if err := rows.Scan(
			&r.ID,
			&r.CommandID,
			&r.ScriptContent,
			&r.FinalCmd,
			&r.Output,
			&r.Error,
			&r.ExitCode,
			&r.WorkingDir,
			&r.ExecutedAt,
		); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (db *DB) AddExecution(record ExecutionRecord) error {
	_, err := db.conn.ExecContext(
		context.Background(),
		"INSERT INTO executions (id, command_id, script_content, final_cmd, output, error, exit_code, working_dir, executed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		record.ID,
		record.CommandID,
		record.ScriptContent,
		record.FinalCmd,
		record.Output,
		record.Error,
		record.ExitCode,
		record.WorkingDir,
		record.ExecutedAt,
	)
	if err != nil {
		return fmt.Errorf("insert execution: %w", err)
	}
	return nil
}

func (db *DB) ClearExecutions() error {
	_, err := db.conn.ExecContext(context.Background(), "DELETE FROM executions")
	if err != nil {
		return fmt.Errorf("clear executions: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func (db *DB) SearchCommands(query string) ([]Command, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Command{}, nil
	}

	// Try FTS5 first
	rows, err := db.conn.QueryContext(
		context.Background(),
		`SELECT c.id, c.title, c.description, c.script_content, c.category_id, c.position, c.created_at, c.updated_at, c.working_dir
		 FROM commands_fts fts
		 JOIN commands c ON c.rowid = fts.rowid
		 WHERE commands_fts MATCH ?
		 ORDER BY rank`,
		query+"*",
	)
	if err != nil {
		// Fallback to LIKE search on FTS error
		return db.searchCommandsLike(query)
	}
	defer rows.Close()

	cmds := []Command{}
	for rows.Next() {
		var c Command
		var catID sql.NullString
		var workingDirRaw string
		if err := rows.Scan(
			&c.ID,
			&c.Title,
			&c.Description,
			&c.ScriptContent,
			&catID,
			&c.Position,
			&c.CreatedAt,
			&c.UpdatedAt,
			&workingDirRaw,
		); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		c.CategoryID = catID.String
		if err := json.Unmarshal([]byte(workingDirRaw), &c.WorkingDir); err != nil {
			return nil, fmt.Errorf("unmarshal working_dir: %w", err)
		}
		cmds = append(cmds, c)
	}
	if err := rows.Err(); err != nil {
		return db.searchCommandsLike(query)
	}

	for i := range cmds {
		if err := db.loadCommandRelations(&cmds[i]); err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

func (db *DB) searchCommandsLike(query string) ([]Command, error) {
	like := "%" + query + "%"
	rows, err := db.conn.QueryContext(context.Background(),
		`SELECT id, title, description, script_content, category_id, position, created_at, updated_at, working_dir
		 FROM commands
		 WHERE title LIKE ? OR description LIKE ? OR script_content LIKE ?
		 ORDER BY position ASC`,
		like, like, like,
	)
	if err != nil {
		return nil, fmt.Errorf("like search: %w", err)
	}
	defer rows.Close()

	cmds := []Command{}
	for rows.Next() {
		var c Command
		var catID sql.NullString
		var workingDirRaw string
		if err := rows.Scan(
			&c.ID,
			&c.Title,
			&c.Description,
			&c.ScriptContent,
			&catID,
			&c.Position,
			&c.CreatedAt,
			&c.UpdatedAt,
			&workingDirRaw,
		); err != nil {
			return nil, fmt.Errorf("scan like result: %w", err)
		}
		c.CategoryID = catID.String
		if err := json.Unmarshal([]byte(workingDirRaw), &c.WorkingDir); err != nil {
			return nil, fmt.Errorf("unmarshal working_dir: %w", err)
		}
		cmds = append(cmds, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range cmds {
		if err := db.loadCommandRelations(&cmds[i]); err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func (db *DB) GetSettings() (AppSettings, error) {
	defaults := AppSettings{
		Locale: "en", Terminal: "",
		Theme: "vscode-dark", LastDarkTheme: "vscode-dark", LastLightTheme: "vscode-light",
		CustomThemes: "[]", UIFont: "Inter", MonoFont: "JetBrains Mono", Density: "comfortable",
		DefaultWorkingDir: &OSPathMap{},
	}
	x, y, w, h := -1, -1, 640, 520
	defaults.WindowX = &x
	defaults.WindowY = &y
	defaults.WindowWidth = &w
	defaults.WindowHeight = &h

	var raw string
	err := db.conn.QueryRowContext(context.Background(), `SELECT data FROM app_settings LIMIT 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		data, _ := json.Marshal(defaults)
		_, err = db.conn.ExecContext(context.Background(), `INSERT INTO app_settings (data) VALUES (?)`, string(data))
		if err != nil {
			return defaults, fmt.Errorf("insert default settings: %w", err)
		}
		return defaults, nil
	}
	if err != nil {
		return defaults, fmt.Errorf("get settings: %w", err)
	}

	merged := defaults
	if err := json.Unmarshal([]byte(raw), &merged); err != nil {
		return defaults, fmt.Errorf("unmarshal settings: %w", err)
	}
	return merged, nil
}

func (db *DB) SetSettings(s AppSettings) error {
	existing, err := db.GetSettings()
	if err != nil {
		return fmt.Errorf("get existing settings: %w", err)
	}

	if s.Locale != "" {
		existing.Locale = s.Locale
	}
	if s.Terminal != "" {
		existing.Terminal = s.Terminal
	}
	if s.Theme != "" {
		existing.Theme = s.Theme
	}
	if s.LastDarkTheme != "" {
		existing.LastDarkTheme = s.LastDarkTheme
	}
	if s.LastLightTheme != "" {
		existing.LastLightTheme = s.LastLightTheme
	}
	if s.CustomThemes != "" {
		existing.CustomThemes = s.CustomThemes
	}
	if s.UIFont != "" {
		existing.UIFont = s.UIFont
	}
	if s.MonoFont != "" {
		existing.MonoFont = s.MonoFont
	}
	if s.Density != "" {
		existing.Density = s.Density
	}
	// nil means "don't touch this field"; a non-nil (possibly empty) map means "apply/update/clear".
	if s.DefaultWorkingDir != nil {
		wd := *s.DefaultWorkingDir
		existing.DefaultWorkingDir = &wd
	}
	if s.WindowX != nil {
		existing.WindowX = s.WindowX
	}
	if s.WindowY != nil {
		existing.WindowY = s.WindowY
	}
	if s.WindowWidth != nil {
		existing.WindowWidth = s.WindowWidth
	}
	if s.WindowHeight != nil {
		existing.WindowHeight = s.WindowHeight
	}

	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	_, err = db.conn.ExecContext(context.Background(), `UPDATE app_settings SET data = ?`, string(data))
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// ResetAll deletes all user data and recreates the default settings row.
func (db *DB) ResetAll() error {
	tx, err := db.conn.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tables := []string{
		"preset_values",
		"variable_presets",
		"variable_definitions",
		"command_tags",
		"executions",
		"commands",
		"tags",
		"categories",
		"app_settings",
	}
	for _, t := range tables {
		query := "DELETE FROM " + t //nolint:gosec // G202: table name is a fixed literal, not user input
		if _, err := tx.ExecContext(context.Background(), query); err != nil {
			return fmt.Errorf("clear %s: %w", t, err)
		}
	}

	defaultSettingsX, defaultSettingsY := -1, -1
	defaultSettingsW, defaultSettingsH := 640, 520
	defaultSettings, _ := json.Marshal(AppSettings{
		Locale:         "en",
		Terminal:       "",
		Theme:          "vscode-dark",
		LastDarkTheme:  "vscode-dark",
		LastLightTheme: "vscode-light",
		CustomThemes:   "[]",
		UIFont:         "Inter",
		MonoFont:       "JetBrains Mono",
		Density:        "comfortable",
		WindowX:        &defaultSettingsX,
		WindowY:        &defaultSettingsY,
		WindowWidth:    &defaultSettingsW,
		WindowHeight:   &defaultSettingsH,
	})
	if _, err := tx.ExecContext(
		context.Background(),
		`INSERT INTO app_settings (data) VALUES (?)`,
		string(defaultSettings),
	); err != nil {
		return fmt.Errorf("insert default settings: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset: %w", err)
	}

	_, _ = db.conn.ExecContext(context.Background(), "VACUUM")
	return nil
}

// ensure time import is used
var _ = time.Now
