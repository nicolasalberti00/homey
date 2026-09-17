package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func mustMigrate(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "homey.db")
	if err := MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	return dbPath
}

func openDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateUpCreatesSchema(t *testing.T) {
	db := openDB(t, mustMigrate(t))
	for _, table := range []string{"rooms", "containers", "items", "item_tags", "api_tokens", "schema_migrations"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}

func TestMigrateUpIsIdempotent(t *testing.T) {
	dbPath := mustMigrate(t)
	if err := MigrateUp(dbPath); err != nil {
		t.Fatalf("second MigrateUp: %v", err)
	}
}

func TestMigrationVersion(t *testing.T) {
	version, dirty, err := MigrationVersion(mustMigrate(t))
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version != 4 || dirty {
		t.Fatalf("version = %d dirty = %t, want 4/false", version, dirty)
	}
}

func TestMigrateDownRemovesSchema(t *testing.T) {
	dbPath := mustMigrate(t)
	if err := MigrateDown(dbPath); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}
	db := openDB(t, dbPath)
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'items'`).Scan(&name)
	if err == nil {
		t.Fatal("items table should be gone after down")
	}
}

func TestItemsRequireExactlyOneLocation(t *testing.T) {
	db := openDB(t, mustMigrate(t))
	if _, err := db.Exec(`INSERT INTO rooms (name) VALUES ('Garage')`); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO items (name) VALUES ('screwdriver')`); err == nil {
		t.Fatal("item with no location should be rejected")
	}
	if _, err := db.Exec(`INSERT INTO items (room_id, name) VALUES (1, 'drill')`); err != nil {
		t.Fatalf("item in a room should be accepted: %v", err)
	}
}

func TestNestedContainers(t *testing.T) {
	db := openDB(t, mustMigrate(t))
	if _, err := db.Exec(`INSERT INTO rooms (name) VALUES ('Garage')`); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO containers (room_id, name) VALUES (1, 'Toolbox')`); err != nil {
		t.Fatalf("insert container: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO containers (room_id, parent_id, name) VALUES (1, 1, 'Drawer 1')`); err != nil {
		t.Fatalf("insert nested container: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO items (container_id, name) VALUES (2, 'Screwdriver')`); err != nil {
		t.Fatalf("insert item in nested container: %v", err)
	}
}

func TestForeignKeysAreEnforced(t *testing.T) {
	db := openDB(t, mustMigrate(t))
	if _, err := db.Exec(`INSERT INTO containers (room_id, name) VALUES (999, 'Toolbox')`); err == nil {
		t.Fatal("container with a missing room should be rejected")
	}
}

func TestQuantityConstraint(t *testing.T) {
	db := openDB(t, mustMigrate(t))
	if _, err := db.Exec(`INSERT INTO rooms (name) VALUES ('Kitchen')`); err != nil {
		t.Fatalf("insert room: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO items (room_id, name, quantity) VALUES (1, 'Cups', -1)`); err == nil {
		t.Fatal("negative quantity should be rejected")
	}
}

func TestDestructiveConfirmationConstraint(t *testing.T) {
	db := openDB(t, mustMigrate(t))
	if _, err := db.Exec(`INSERT INTO api_tokens (name, token_hash, destructive_confirmation) VALUES ('bad', 'hash', 'whatever')`); err == nil {
		t.Fatal("invalid destructive_confirmation should be rejected")
	}
	if _, err := db.Exec(`INSERT INTO api_tokens (name, token_hash) VALUES ('ok', 'hash')`); err != nil {
		t.Fatalf("default policy should be accepted: %v", err)
	}
}

func TestUniqueNameConstraints(t *testing.T) {
	db := openDB(t, mustMigrate(t))

	mustExec(t, db, `INSERT INTO rooms (name) VALUES ('Garage')`)
	if _, err := db.Exec(`INSERT INTO rooms (name) VALUES ('garage')`); err == nil {
		t.Fatal("duplicate room name should be rejected case-insensitively")
	}

	mustExec(t, db, `INSERT INTO containers (room_id, name) VALUES (1, 'Toolbox')`)
	if _, err := db.Exec(`INSERT INTO containers (room_id, name) VALUES (1, 'toolbox')`); err == nil {
		t.Fatal("duplicate container name in the same room should be rejected")
	}
	mustExec(t, db, `INSERT INTO rooms (name) VALUES ('Kitchen')`)
	if _, err := db.Exec(`INSERT INTO containers (room_id, name) VALUES (2, 'Toolbox')`); err != nil {
		t.Fatalf("the same container name in another room should be accepted: %v", err)
	}

	mustExec(t, db, `INSERT INTO items (room_id, name) VALUES (1, 'Drill')`)
	if _, err := db.Exec(`INSERT INTO items (room_id, name) VALUES (1, 'drill')`); err == nil {
		t.Fatal("duplicate item name in the same room should be rejected")
	}
	mustExec(t, db, `INSERT INTO items (container_id, name) VALUES (1, 'Drill')`)
	if _, err := db.Exec(`INSERT INTO items (container_id, name) VALUES (1, 'DRILL')`); err == nil {
		t.Fatal("duplicate item name in the same container should be rejected")
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}

func TestItemTagsCascadeOnDelete(t *testing.T) {
	db := openDB(t, mustMigrate(t))

	mustExec(t, db, `INSERT INTO rooms (name) VALUES ('Garage')`)
	mustExec(t, db, `INSERT INTO items (room_id, name) VALUES (1, 'Drill')`)
	mustExec(t, db, `INSERT INTO item_tags (item_id, tag) VALUES (1, 'Strumenti')`)

	if _, err := db.Exec(`INSERT INTO item_tags (item_id, tag) VALUES (1, 'strumenti')`); err == nil {
		t.Fatal("case-insensitive duplicate tag on the same item should be rejected")
	}

	mustExec(t, db, `DELETE FROM items WHERE id = 1`)

	var tags int
	if err := db.QueryRow(`SELECT count(*) FROM item_tags`).Scan(&tags); err != nil {
		t.Fatalf("counting tags: %v", err)
	}
	if tags != 0 {
		t.Fatalf("item_tags rows after deleting the item = %d, want 0 (ON DELETE CASCADE)", tags)
	}
}
