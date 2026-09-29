// Package backup exports the whole inventory as a portable JSON document and
// imports it back: homey's backup format (roadmap Step 7.3).
//
// The document names things instead of carrying the ids of the instance it
// came from — a room by its name, a container by its name inside its room, an
// item by its name at its place — so the same file restores a fresh instance
// and merges into a populated one. Names are compared the way the database
// compares them (scopeKey), because that is what the uniqueness rules use.
//
// Import is an upsert: an entity the document mentions is created, updated or
// left alone, and an entity it does not mention is never touched. That makes
// importing twice leave the same inventory, and makes a half-applied import
// finish on the next run.
package backup

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Format and Version identify a document, so a file from a later homey is
// refused instead of half-understood.
const (
	// Format is the value of the format field.
	Format = "homey.export"
	// Version is the value of the version field.
	Version = 1
)

// Document is a whole inventory. The fields are in the order they read.
type Document struct {
	// Format identifies the document; it must be Format.
	Format string `json:"format"`
	// Version identifies the shape of the document; it must be Version.
	Version int `json:"version"`
	// ExportedAt is when the export ran. An import ignores it, and a
	// document written by hand may leave it out.
	ExportedAt time.Time `json:"exported_at,omitempty"`
	// Rooms are the rooms of the inventory, by name. Absent means none.
	Rooms []Room `json:"rooms,omitempty" doc:"Rooms of the inventory, by name; absent when there are none."`
	// Containers are the containers, parents before children, each naming
	// the room it is in and the container it hangs from. Absent means none.
	Containers []Container `json:"containers,omitempty" doc:"Containers, parents before children; absent when there are none."`
	// Items are the items, each naming the place it lives in. Absent means
	// none.
	Items []Item `json:"items,omitempty" doc:"Items, each naming its place; absent when there are none."`
}

// Room is a room as the document carries it.
type Room struct {
	Name        string `json:"name" minLength:"1" maxLength:"120" doc:"Name of the room, unique across the inventory."`
	Description string `json:"description,omitempty" maxLength:"2000" doc:"Optional description of the room."`
}

// Container is a container as the document carries it. Room and name together
// identify it: a name is unique within its room, not across the inventory.
type Container struct {
	Room        string `json:"room" minLength:"1" maxLength:"120" doc:"Name of the room the container is in."`
	Parent      string `json:"parent,omitempty" maxLength:"120" doc:"Name of the container it hangs from, inside the same room."`
	Name        string `json:"name" minLength:"1" maxLength:"120" doc:"Name of the container, unique within its room."`
	Description string `json:"description,omitempty" maxLength:"2000" doc:"Optional description of the container."`
}

// Item is an item as the document carries it: the fields a replay needs and
// the place it lives in, named.
type Item struct {
	Name        string   `json:"name" minLength:"1" maxLength:"120" doc:"Name of the item, unique within its location."`
	Description string   `json:"description,omitempty" maxLength:"2000" doc:"Optional description of the item."`
	Quantity    int      `json:"quantity" minimum:"0" maximum:"1000000" doc:"How many of the item exist; zero means none left."`
	Notes       string   `json:"notes,omitempty" maxLength:"4000" doc:"Optional free-form notes."`
	Tags        []string `json:"tags,omitempty" maxItems:"20" doc:"Free-form labels, unique within the item."`
	Aliases     []string `json:"aliases,omitempty" maxItems:"20" doc:"Other names the item answers to."`
	Location    Location `json:"location" doc:"Where the item lives."`
}

// Location is a place named: a room, or a container inside a room. A
// container's name alone is not enough, because two rooms may hold a
// "Toolbox".
type Location struct {
	Kind      string `json:"kind" enum:"room,container" doc:"Whether the location is a room or a container."`
	Room      string `json:"room" minLength:"1" maxLength:"120" doc:"Name of the room."`
	Container string `json:"container,omitempty" maxLength:"120" doc:"Name of the container, when kind is container."`
}

// Repos is the slice of the inventory an export and an import work through:
// the same ports every other adapter uses.
type Repos struct {
	Rooms      inventory.RoomRepo
	Containers inventory.ContainerRepo
	Items      inventory.ItemRepo
}

// Export reads the whole inventory into a document.
//
// Everything is ordered by name, never by id: two instances that hold the
// same inventory produce the same document, which is what makes a backup diff
// say what changed rather than where each row happens to live.
func Export(ctx context.Context, r Repos) (Document, error) {
	rooms, err := r.Rooms.List(ctx)
	if err != nil {
		return Document{}, fmt.Errorf("listing rooms: %w", err)
	}
	containers, err := r.Containers.List(ctx)
	if err != nil {
		return Document{}, fmt.Errorf("listing containers: %w", err)
	}
	items, err := r.Items.List(ctx)
	if err != nil {
		return Document{}, fmt.Errorf("listing items: %w", err)
	}

	roomNames := make(map[inventory.RoomID]string, len(rooms))
	docRooms := make([]Room, 0, len(rooms))
	for _, room := range rooms {
		roomNames[room.ID] = room.Name
		docRooms = append(docRooms, Room{Name: room.Name, Description: room.Description})
	}
	sortRooms(docRooms)

	// The containers are read in listing order, where a child can come before
	// its parent: names are resolved in a first pass, ordered in a second.
	containerNames := make(map[inventory.ContainerID]Location, len(containers))
	for _, container := range containers {
		room, known := roomNames[container.RoomID]
		if !known {
			return Document{}, fmt.Errorf(
				"container %q points at room %d, which does not exist", container.Name, container.RoomID)
		}
		containerNames[container.ID] = Location{Kind: "container", Room: room, Container: container.Name}
	}

	docContainers := make([]Container, 0, len(containers))
	for _, container := range containers {
		entry := Container{
			Room:        roomNames[container.RoomID],
			Name:        container.Name,
			Description: container.Description,
		}
		if container.ParentID != nil {
			parent, known := containerNames[*container.ParentID]
			if !known {
				return Document{}, fmt.Errorf(
					"container %q hangs from container %d, which does not exist", container.Name, *container.ParentID)
			}
			entry.Parent = parent.Container
		}
		docContainers = append(docContainers, entry)
	}
	ordered, err := orderContainers(docContainers)
	if err != nil {
		return Document{}, err
	}

	docItems := make([]Item, 0, len(items))
	for _, item := range items {
		location := Location{Kind: string(item.Location.Kind)}
		switch item.Location.Kind {
		case inventory.LocationRoom:
			room, known := roomNames[inventory.RoomID(item.Location.ID)]
			if !known {
				return Document{}, fmt.Errorf("item %q points at room %d, which does not exist", item.Name, item.Location.ID)
			}
			location.Room = room
		case inventory.LocationContainer:
			container, known := containerNames[inventory.ContainerID(item.Location.ID)]
			if !known {
				return Document{}, fmt.Errorf(
					"item %q points at container %d, which does not exist", item.Name, item.Location.ID)
			}
			location.Room, location.Container = container.Room, container.Container
		default:
			return Document{}, fmt.Errorf("item %q has an unknown location kind %q", item.Name, item.Location.Kind)
		}
		docItems = append(docItems, Item{
			Name:        item.Name,
			Description: item.Description,
			Quantity:    item.Quantity,
			Notes:       item.Notes,
			Tags:        item.Tags,
			Aliases:     item.Aliases,
			Location:    location,
		})
	}
	sortItems(docItems)

	return Document{
		Format:     Format,
		Version:    Version,
		ExportedAt: time.Now().UTC(),
		Rooms:      docRooms,
		Containers: ordered,
		Items:      docItems,
	}, nil
}

// containerKey identifies a container by its room and its name, folded the
// way the database folds them.
type containerKey struct {
	room string
	name string
}

// itemKey identifies an item by its place and its name, folded the way the
// database folds them.
type itemKey struct {
	kind      string
	room      string
	container string
	name      string
}

// scopeKey folds a name the way SQLite's NOCASE does: ASCII letters only.
// That is exactly what the uniqueness rules compare names with, so two names
// the database would call a duplicate become one key here, and two it keeps
// apart — "Caffè" and "Caffe" — stay apart.
func scopeKey(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, name)
}

// sortRooms orders rooms by their folded name — the database's own order.
func sortRooms(rooms []Room) {
	sort.SliceStable(rooms, func(i, j int) bool {
		if left, right := scopeKey(rooms[i].Name), scopeKey(rooms[j].Name); left != right {
			return left < right
		}
		return rooms[i].Name < rooms[j].Name
	})
}

// sortItems orders items by name and then by place, so a document reads like
// a list and two instances agree on the order even where names repeat.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if left, right := scopeKey(items[i].Name), scopeKey(items[j].Name); left != right {
			return left < right
		}
		left, right := items[i].Location, items[j].Location
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if a, b := scopeKey(left.Room), scopeKey(right.Room); a != b {
			return a < b
		}
		if a, b := scopeKey(left.Container), scopeKey(right.Container); a != b {
			return a < b
		}
		return items[i].Name < items[j].Name
	})
}

// orderContainers groups containers by room and puts parents before children,
// which is the order an import can apply top to bottom. A circle has no such
// order, so it is refused here.
func orderContainers(containers []Container) ([]Container, error) {
	positions := make(map[containerKey]int, len(containers))
	for index, container := range containers {
		key := containerKey{room: scopeKey(container.Room), name: scopeKey(container.Name)}
		if previous, duplicate := positions[key]; duplicate {
			return nil, inventory.NewValidationError(
				fmt.Sprintf("containers[%d].name", index),
				fmt.Sprintf("duplicates containers[%d] in the same room", previous))
		}
		positions[key] = index
	}

	// The depth of a container is how many ancestors it has: parents sort
	// before children within their room, and rooms sort by folded name.
	depths := make([]int, len(containers))
	for index := range containers {
		ancestors := make(map[int]bool, len(containers))
		for current := index; ; {
			if ancestors[current] {
				return nil, inventory.NewValidationError(
					"containers", "must not contain a cycle: a container hangs from itself")
			}
			ancestors[current] = true
			parent := containers[current].Parent
			if parent == "" {
				break
			}
			next, known := positions[containerKey{room: scopeKey(containers[current].Room), name: scopeKey(parent)}]
			if !known {
				// A parent outside the document: nothing to order before.
				break
			}
			current = next
			depths[index]++
		}
	}

	placed := make([]struct {
		entry Container
		depth int
	}, len(containers))
	for index, container := range containers {
		placed[index].entry = container
		placed[index].depth = depths[index]
	}
	sort.SliceStable(placed, func(i, j int) bool {
		left, right := placed[i], placed[j]
		if a, b := scopeKey(left.entry.Room), scopeKey(right.entry.Room); a != b {
			return a < b
		}
		if left.depth != right.depth {
			return left.depth < right.depth
		}
		if a, b := scopeKey(left.entry.Name), scopeKey(right.entry.Name); a != b {
			return a < b
		}
		return left.entry.Name < right.entry.Name
	})

	ordered := make([]Container, len(containers))
	for index, entry := range placed {
		ordered[index] = entry.entry
	}
	return ordered, nil
}
