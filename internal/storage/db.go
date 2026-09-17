// Package storage implements homey's persistence: the SQLite database and
// the schema migrations.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

// Open opens (creating if needed) the SQLite database at path.
//
// SQLite is configured with WAL journaling, foreign key enforcement and a
// busy timeout, which are the sane defaults for a single-node self-hosted
// deployment.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path must not be empty")
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating database directory: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging sqlite database: %w", err)
	}
	return db, nil
}
