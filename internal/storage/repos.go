package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Repos bundles the SQLite implementations of the inventory repository
// ports. The repositories share the database handle passed to NewRepos.
type Repos struct {
	Rooms      inventory.RoomRepo
	Containers inventory.ContainerRepo
	Items      inventory.ItemRepo
}

// NewRepos builds the repositories backed by db.
func NewRepos(db *sql.DB) Repos {
	return Repos{
		Rooms:      &roomRepo{db: db},
		Containers: &containerRepo{db: db},
		Items:      &itemRepo{db: db},
	}
}

// querier is the subset of database/sql shared by *sql.DB and *sql.Tx, so
// helpers work both inside and outside transactions.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// rowScanner is implemented by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// withTx runs fn inside a transaction, rolling back when fn fails. Checks
// that span several statements (reference checks before writes, content
// checks before deletes) run inside one, so concurrent writers cannot slip
// between them.
func withTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// notFound reports a missing entity in the shape of inventory.ErrNotFound.
func notFound(entity string, id int64) error {
	return fmt.Errorf("%s %d: %w", entity, id, inventory.ErrNotFound)
}

// requireRoom fails with ErrNotFound when the room reference does not exist.
func requireRoom(ctx context.Context, q querier, id inventory.RoomID) error {
	var one int
	err := q.QueryRowContext(ctx, roomExistsSQL, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("room", int64(id))
	}
	if err != nil {
		return fmt.Errorf("loading room %d: %w", id, err)
	}
	return nil
}

// parseTime converts a stored timestamp into UTC.
func parseTime(value string) (time.Time, error) {
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", value, err)
	}
	return ts.UTC(), nil
}
