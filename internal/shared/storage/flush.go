package storage

import (
	"database/sql"
	"fmt"
	"regexp"
)

// sqliteIdentifier matches a plain SQL identifier. Table names interpolated
// into DELETE below always come from sqlite_master (never request input),
// but this guards against ever widening that assumption unsafely.
var sqliteIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
			_ = rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_ = rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, name := range tables {
		if !sqliteIdentifier.MatchString(name) {
			_ = tx.Rollback()
			return fmt.Errorf("flush table %s: unexpected table name", name)
		}
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s"`, name)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("flush table %s: %w", name, err)
		}
	}
	return tx.Commit()
}
