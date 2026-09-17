package storage

import (
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory/inventorytest"
)

// TestRepositoryContract runs the shared repository contract suite against
// the SQLite adapter. Any future implementation (another database, an
// in-memory double) runs the same suite, so behaviour stays identical across
// adapters.
func TestRepositoryContract(t *testing.T) {
	inventorytest.RunContract(t, func(t *testing.T) inventorytest.Repos {
		db := openDB(t, mustMigrate(t))
		repos := NewRepos(db)
		return inventorytest.Repos{
			Rooms:      repos.Rooms,
			Containers: repos.Containers,
			Items:      repos.Items,
		}
	})
}
