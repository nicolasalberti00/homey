package inventory

import "context"

// This file defines the persistence ports of the inventory core: interfaces
// implemented by storage adapters (currently the SQLite implementation in
// internal/storage). The dependency points one way — adapters import the
// core, never the reverse — so business logic stays independent of any
// database. imports_test.go enforces this.
//
// Every implementation of these interfaces follows the same contract:
//
//   - Create and Update normalize the entity (trimming surrounding
//     whitespace) and validate it before writing. Invalid input fails with an
//     error matching ErrValidation and carrying a *ValidationError.
//   - Lookups and writes that reference a missing entity fail with an error
//     matching ErrNotFound.
//   - State-dependent refusals (deleting a non-empty room or container) fail
//     with an error matching ErrConflict.
//   - Names are unique within their scope: a room name across the inventory,
//     a container name within its room, an item name within its location.
//     Duplicate names fail with ErrConflict; comparison is case-insensitive.
//   - A container can never become its own ancestor: re-parenting it under
//     itself or under one of its descendants fails with ErrCycle.
//   - Item tags are trimmed, case-insensitively unique within their item,
//     limited to MaxTagsPerItem tags of MaxTagLen characters, and returned
//     ordered case-insensitively.
//   - Move changes only the location: an item keeps every other field, and a
//     container carries its whole subtree into the destination room.
//   - Listings are ordered by name, case-insensitively, then by ID, and
//     return a non-nil empty slice when nothing matches.
//   - Entities read from storage carry their ID and UTC timestamps.
//
// All methods accept a context and are safe for concurrent use.

// RoomRepo is the persistence port for rooms, the top level of the
// inventory.
type RoomRepo interface {
	// Create persists room and fills in its ID and timestamps.
	Create(ctx context.Context, room *Room) error
	// Get returns the room with the given ID.
	Get(ctx context.Context, id RoomID) (Room, error)
	// List returns every room.
	List(ctx context.Context) ([]Room, error)
	// Update replaces the stored name and description of room and refreshes
	// its updated timestamp.
	Update(ctx context.Context, room *Room) error
	// Delete removes an empty room. It fails with ErrConflict while the room
	// still contains containers or items.
	Delete(ctx context.Context, id RoomID) error
}

// ContainerRepo is the persistence port for containers, the nested units
// inside rooms.
type ContainerRepo interface {
	// Create persists container and fills in its ID and timestamps. A
	// non-nil ParentID must reference an existing container in the same
	// room.
	Create(ctx context.Context, container *Container) error
	// Get returns the container with the given ID.
	Get(ctx context.Context, id ContainerID) (Container, error)
	// ListByRoom returns every container of a room, at any nesting depth.
	ListByRoom(ctx context.Context, roomID RoomID) ([]Container, error)
	// Path returns the location of a container: its room and the chain of
	// containers from the top down, ending with the container itself.
	Path(ctx context.Context, id ContainerID) (ContainerPath, error)
	// Update replaces the stored parent, name and description of container
	// and refreshes its updated timestamp. Containers cannot change room;
	// moving them is a separate operation. Re-parenting is refused with
	// ErrCycle when the new parent is the container itself or one of its
	// descendants.
	Update(ctx context.Context, container *Container) error
	// Move relocates the container and its whole subtree atomically: a room
	// destination makes it a root of that room, a container destination makes
	// it a child of that container and the subtree adopts its room. It fails
	// with ErrCycle when the destination is the container itself or one of
	// its descendants, and with ErrConflict when a name would collide in the
	// destination room.
	Move(ctx context.Context, id ContainerID, destination Location) error
	// Delete removes a container that has no child containers and no items.
	// It fails with ErrConflict otherwise.
	Delete(ctx context.Context, id ContainerID) error
}

// ItemRepo is the persistence port for items and their location.
type ItemRepo interface {
	// Create persists item and fills in its ID and timestamps. Its location
	// must reference an existing room or container.
	Create(ctx context.Context, item *Item) error
	// Get returns the item with the given ID.
	Get(ctx context.Context, id ItemID) (Item, error)
	// List returns every item, across all locations.
	List(ctx context.Context) ([]Item, error)
	// ListByLocation returns the items that sit directly in the given room
	// or container.
	ListByLocation(ctx context.Context, location Location) ([]Item, error)
	// Update replaces the stored fields of item, including its location, and
	// refreshes its updated timestamp.
	Update(ctx context.Context, item *Item) error
	// Move relocates the item to another room or container without touching
	// its other fields. The destination must reference an existing room or
	// container; when an item with the same name already lives there the move
	// fails with ErrConflict.
	Move(ctx context.Context, id ItemID, destination Location) error
	// Delete removes the item.
	Delete(ctx context.Context, id ItemID) error
}
