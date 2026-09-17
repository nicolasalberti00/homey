// Package inventorytest provides a reusable contract test suite for
// inventory repository implementations.
//
// Adapters run the suite from their own tests, so every implementation — the
// SQLite adapter today, another database or an in-memory double tomorrow —
// is held to the same observable behaviour:
//
//	inventorytest.RunContract(t, func(t *testing.T) inventorytest.Repos {
//		return inventorytest.Repos{Rooms: rooms, Containers: containers, Items: items}
//	})
//
// Error mapping (ErrValidation carrying a *ValidationError, ErrNotFound,
// ErrConflict), normalization and listing order are documented on the
// interfaces in package inventory; this suite is the executable form of that
// contract.
package inventorytest

import (
	"errors"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Repos bundles the repository implementations under test.
type Repos struct {
	Rooms      inventory.RoomRepo
	Containers inventory.ContainerRepo
	Items      inventory.ItemRepo
}

// NewRepos returns a fresh, empty set of repositories. Every call must
// provide isolated storage, because each subtest starts from an empty
// database.
type NewRepos func(t *testing.T) Repos

// RunContract runs the repository contract suite.
func RunContract(t *testing.T, newRepos NewRepos) {
	t.Helper()
	t.Run("Rooms", func(t *testing.T) { testRooms(t, newRepos) })
	t.Run("Containers", func(t *testing.T) { testContainers(t, newRepos) })
	t.Run("Items", func(t *testing.T) { testItems(t, newRepos) })
}

// requiresNoError fails the test when err is not nil.
func requiresNoError(t *testing.T, err error, action string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

// requiresError fails the test unless err matches target with errors.Is.
func requiresError(t *testing.T, err error, target error, action string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: error = %v, want %v", action, err, target)
	}
}

// requiresValidationProblem fails the test unless err is a
// *inventory.ValidationError carrying a problem for field.
func requiresValidationProblem(t *testing.T, err error, field string) {
	t.Helper()
	var validation *inventory.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *inventory.ValidationError", err)
	}
	for _, problem := range validation.Problems {
		if problem.Field == field {
			return
		}
	}
	t.Fatalf("validation error %v has no problem for field %q", err, field)
}

// createRoom creates a room and returns it.
func createRoom(t *testing.T, repos Repos, name string) inventory.Room {
	t.Helper()
	room := inventory.Room{Name: name}
	requiresNoError(t, repos.Rooms.Create(t.Context(), &room), "creating room "+name)
	return room
}

// createContainer creates a container and returns it.
func createContainer(t *testing.T, repos Repos, room inventory.Room, parent *inventory.ContainerID, name string) inventory.Container {
	t.Helper()
	container := inventory.Container{RoomID: room.ID, ParentID: parent, Name: name}
	requiresNoError(t, repos.Containers.Create(t.Context(), &container), "creating container "+name)
	return container
}

// createItem creates an item with quantity 1 and returns it.
func createItem(t *testing.T, repos Repos, name string, location inventory.Location) inventory.Item {
	t.Helper()
	item := inventory.Item{Name: name, Quantity: 1, Location: location}
	requiresNoError(t, repos.Items.Create(t.Context(), &item), "creating item "+name)
	return item
}
