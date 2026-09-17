package storage

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/nicolasalberti00/homey/migrations"
)

// newMigrator opens a dedicated database connection for migrations and wires
// the embedded migrations into golang-migrate. The caller owns both the
// migrator and the connection.
//
// The connection is deliberately dedicated: the sqlite driver's Close()
// closes the *sql.DB it was given, so sharing the serving connection would
// break the server. For the same reason the caller never calls m.Close() and
// closes the returned *sql.DB instead.
func newMigrator(dbPath string) (*migrate.Migrate, *sql.DB, error) {
	db, err := Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("loading embedded migrations: %w", err)
	}
	driver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("initializing migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("initializing migrator: %w", err)
	}
	return m, db, nil
}

// MigrateUp applies all pending migrations. Running it when the schema is
// already up to date is a no-op.
func MigrateUp(dbPath string) error {
	m, db, err := newMigrator(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}
	return nil
}

// MigrateDown rolls every migration back. Development and recovery helper.
func MigrateDown(dbPath string) error {
	m, db, err := newMigrator(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("rolling back migrations: %w", err)
	}
	return nil
}

// MigrationVersion reports the current schema version and whether the last
// migration failed halfway (dirty state).
func MigrationVersion(dbPath string) (version uint, dirty bool, err error) {
	m, db, err := newMigrator(dbPath)
	if err != nil {
		return 0, false, err
	}
	defer db.Close()
	version, dirty, err = m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return version, dirty, err
}

// MigrateForce marks the schema as being at version without running any
// migration. It is the escape hatch for a dirty state; version -1 resets to
// "no version".
func MigrateForce(dbPath string, version int) error {
	m, db, err := newMigrator(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := m.Force(version); err != nil {
		return fmt.Errorf("forcing migration version %d: %w", version, err)
	}
	return nil
}
