package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens (creating parent dirs as needed) a SQLite database at path and
// configures it for the simulator's single-writer workload: WAL journaling,
// foreign keys on, and a single pooled connection (mirrors the previous
// better-sqlite3 single-connection behavior and avoids WAL/pragma
// inconsistency across pooled connections, especially for ":memory:" DBs
// where each connection would otherwise be a distinct empty database).
func Open(path string) (*sql.DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("set journal_mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}

	return db, nil
}
