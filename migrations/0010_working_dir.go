package migrations

import (
	"database/sql"
	"fmt"
)

var migration0010 = Migration{
	Version:     10,
	Description: "commands: add working_dir column for OS-keyed paths",
	Up: func(tx *sql.Tx) error {
		_, err := tx.Exec(`ALTER TABLE commands ADD COLUMN working_dir TEXT NOT NULL DEFAULT '{}'`)
		if err != nil {
			return fmt.Errorf("migration 0010 up: %w", err)
		}
		return nil
	},
	Down: func(tx *sql.Tx) error {
		_, err := tx.Exec(`ALTER TABLE commands DROP COLUMN working_dir`)
		if err != nil {
			return fmt.Errorf("migration 0010 down: %w", err)
		}
		return nil
	},
}
