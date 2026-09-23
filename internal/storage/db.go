// Package storage implements homey's persistence: the SQLite database and
// the schema migrations.
package storage

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nicolasalberti00/homey/internal/inventory"
	"modernc.org/sqlite"
)

// FoldFunction is the SQL name of inventory.FoldText. The search queries
// compare folded text on both sides, so the accent a user types — or forgets —
// never decides a match; naming it here keeps the registration and the SQL in
// step (a test checks that the statements use it).
const FoldFunction = "homey_fold"

// registerFold installs FoldFunction on the driver. The driver offers a
// registered function to the connections opened after the registration, which
// is why Open calls this before sql.Open; sync.Once makes repeated Opens (the
// tests do that) harmless.
var registerFold = sync.OnceFunc(func() {
	sqlite.MustRegisterDeterministicScalarFunction(FoldFunction, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			text, ok := args[0].(string)
			if !ok {
				// NULL folds to NULL, like every other SQL function.
				return nil, nil
			}
			return inventory.FoldText(text), nil
		})
})

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
	registerFold()
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
