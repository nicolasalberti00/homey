package inventory

import (
	"strings"
	"time"
)

// RoomID identifies a room. IDs are assigned by the storage layer; a zero ID
// means "not persisted yet".
type RoomID int64

// ContainerID identifies a container.
type ContainerID int64

// ItemID identifies an item.
type ItemID int64

// Room is the top level of the inventory: a physical room such as "Kitchen",
// "Garage" or "Office".
type Room struct {
	ID          RoomID
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Container is a storage unit inside a room, such as a toolbox, a drawer or a
// shelf. Containers can nest: ParentID is nil when the container sits
// directly in its room, otherwise it points at the containing container.
type Container struct {
	ID          ContainerID
	RoomID      RoomID
	ParentID    *ContainerID
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ContainerPath is the location of a container: its room and the nested
// containers that lead to it, ordered from the top down and ending with the
// container itself.
type ContainerPath struct {
	Room       Room
	Containers []Container
}

// String renders the path as "Garage > Toolbox > Drawer 1".
func (p ContainerPath) String() string {
	parts := make([]string, 0, len(p.Containers)+1)
	parts = append(parts, p.Room.Name)
	for _, container := range p.Containers {
		parts = append(parts, container.Name)
	}
	return strings.Join(parts, " > ")
}

// Item is a single object in the inventory. It lives in exactly one location:
// a room or a container.
type Item struct {
	ID          ItemID
	Name        string
	Description string
	Quantity    int
	Notes       string
	Location    Location
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// LocationKind distinguishes the two kinds of item location. It is a string
// enum so it stays self-describing in JSON, OpenAPI and MCP tool schemas.
type LocationKind string

const (
	// LocationRoom means the item sits directly in a room.
	LocationRoom LocationKind = "room"
	// LocationContainer means the item sits inside a container.
	LocationContainer LocationKind = "container"
)

// Valid reports whether k is a known location kind. The zero value is not
// valid, so a Location must be built with RoomLocation or ContainerLocation.
func (k LocationKind) Valid() bool {
	return k == LocationRoom || k == LocationContainer
}

// Location says where an item lives: a room or a container. Exactly one of
// the two kinds is set; build one with RoomLocation or ContainerLocation.
type Location struct {
	Kind LocationKind
	// ID is a RoomID when Kind is LocationRoom and a ContainerID when Kind is
	// LocationContainer.
	ID int64
}

// RoomLocation builds a location pointing at a room.
func RoomLocation(id RoomID) Location {
	return Location{Kind: LocationRoom, ID: int64(id)}
}

// ContainerLocation builds a location pointing at a container.
func ContainerLocation(id ContainerID) Location {
	return Location{Kind: LocationContainer, ID: int64(id)}
}

// IsRoom reports whether the location points at a room.
func (l Location) IsRoom() bool { return l.Kind == LocationRoom }

// IsContainer reports whether the location points at a container.
func (l Location) IsContainer() bool { return l.Kind == LocationContainer }

// RoomID returns the referenced room ID. The second result is false when the
// location is not a room.
func (l Location) RoomID() (RoomID, bool) {
	if l.Kind != LocationRoom {
		return 0, false
	}
	return RoomID(l.ID), true
}

// ContainerID returns the referenced container ID. The second result is false
// when the location is not a container.
func (l Location) ContainerID() (ContainerID, bool) {
	if l.Kind != LocationContainer {
		return 0, false
	}
	return ContainerID(l.ID), true
}
