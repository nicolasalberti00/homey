package storage

// The data at rest half of the security review: the database holds
// the whole inventory and the token hashes, so a fresh one is created for its
// owner only.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDatabaseFileIsCreatedOwnerOnly: SQLite takes the permissions of the file
// it finds, and the default umask would leave the inventory readable by every
// account on the host. The file is created before the connection opens, and
// the WAL files that follow inherit its mode.
func TestDatabaseFileIsCreatedOwnerOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "secrets", "homey.db")
	if err := MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db := openDB(t, dbPath)

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("database mode = %o, want 600 (owner only)", got)
	}

	// Write, so the WAL files exist, and check they are no wider than the
	// database itself.
	if _, err := db.Exec(`INSERT INTO rooms (name) VALUES ('Garage')`); err != nil {
		t.Fatalf("inserting: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		side, err := os.Stat(dbPath + suffix)
		if err != nil {
			// A clean close may have removed them; that is fine, the file
			// the mode comes from is already checked.
			continue
		}
		if got := side.Mode().Perm(); got&0o077 != 0 {
			t.Errorf("%s mode = %o, want no access for group or others", suffix, got)
		}
	}
}

// TestAnExistingDatabaseKeepsItsMode: an operator who widened the file for a
// backup tool keeps that choice — the review tightens new installs, it does
// not silently undo a decision already taken.
func TestAnExistingDatabaseKeepsItsMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "homey.db")
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("creating the file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if err := MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	db := openDB(t, dbPath)
	if _, err := db.Exec(`SELECT count(*) FROM rooms`); err != nil {
		t.Fatalf("reading: %v", err)
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("database mode = %o, want 644 as it was created", got)
	}
}
