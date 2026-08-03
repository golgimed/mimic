package storage

import (
	"database/sql"
	"fmt"
)

// Flush deletes all rows from every provider/shared data table, but keeps
// schema and _migrations intact so the app doesn't need to re-migrate on
// next boot.
func Flush(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != '_migrations'
	`)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, name := range tables {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s"`, name)); err != nil {
			tx.Rollback()
			return fmt.Errorf("flush table %s: %w", name, err)
		}
	}
	return tx.Commit()
}
