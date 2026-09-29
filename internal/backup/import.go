package backup

// Import: validate the whole document, then apply it. Nothing is written
// before every entry has been checked, so a bad entry at the end never leaves
// the good ones behind, and what does get written is audited by the storage
// layer like any other mutation.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Report says what an import did, per kind of entity: what it had to create,
// what it had to change, and what already stood as the document says. A second
// import of the same document reports everything unchanged.
type Report struct {
	Rooms      Counts `json:"rooms"`
	Containers Counts `json:"containers"`
	Items      Counts `json:"items"`
}

// Counts is the fate of one kind of entity during an import.
type Counts struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

// Import applies a document to the inventory behind r.
//
// Every entry is matched by name within its scope — the same scope the
// uniqueness rules use — so importing the same document twice leaves the same
// inventory, and importing into a fresh instance reproduces the one the
// document came from. An entry that already matches is left alone rather than
// rewritten, so a repeat import is also quiet in the event log.
//
// Import never deletes: an entity the document does not mention is somebody's
// data, not an import's to remove. What the document does mention is
// authoritative for that entity, including the fields it leaves out.
func Import(ctx context.Context, r Repos, doc Document) (Report, error) {
	if doc.Format != Format {
		return Report{}, inventory.NewValidationError("format",
			fmt.Sprintf("unknown format %q; want %q", doc.Format, Format))
	}
	if doc.Version != Version {
		return Report{}, inventory.NewValidationError("version",
			fmt.Sprintf("unsupported version %d; want %d", doc.Version, Version))
	}

	rooms, err := r.Rooms.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("listing rooms: %w", err)
	}
	containers, err := r.Containers.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("listing containers: %w", err)
	}
	items, err := r.Items.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("listing items: %w", err)
	}

	w, err := check(doc, rooms, containers, items)
	if err != nil {
		return Report{}, err
	}
	return w.apply(ctx, r, doc)
}

// world is the inventory as it stands, indexed the way the document names it,
// kept in step while the import writes: an entry created a line ago can be
// referenced by the next one. known holds every name the document may point
// at — what stands now plus what the document itself declares.
type world struct {
	rooms      map[string]*inventory.Room
	containers map[containerKey]*inventory.Container
	items      map[itemKey]*inventory.Item

	knownRooms      map[string]bool
	knownContainers map[containerKey]bool
}

// check validates every entry of doc against what stands now, and returns the
// world it validated against. It writes nothing: the document is accepted or
// refused as a whole.
func check(doc Document, rooms []inventory.Room, containers []inventory.Container, items []inventory.Item) (*world, error) {
	w := &world{
		rooms:           make(map[string]*inventory.Room, len(rooms)),
		containers:      make(map[containerKey]*inventory.Container, len(containers)),
		items:           make(map[itemKey]*inventory.Item, len(items)),
		knownRooms:      make(map[string]bool, len(rooms)),
		knownContainers: make(map[containerKey]bool, len(containers)),
	}

	roomNames := make(map[inventory.RoomID]string, len(rooms))
	for index := range rooms {
		w.rooms[scopeKey(rooms[index].Name)] = &rooms[index]
		w.knownRooms[scopeKey(rooms[index].Name)] = true
		roomNames[rooms[index].ID] = rooms[index].Name
	}

	containerNames := make(map[inventory.ContainerID]Location, len(containers))
	for index := range containers {
		room, known := roomNames[containers[index].RoomID]
		if !known {
			return nil, fmt.Errorf("container %q points at room %d, which does not exist",
				containers[index].Name, containers[index].RoomID)
		}
		key := containerKey{room: scopeKey(room), name: scopeKey(containers[index].Name)}
		w.containers[key] = &containers[index]
		w.knownContainers[key] = true
		containerNames[containers[index].ID] = Location{
			Kind: "container", Room: room, Container: containers[index].Name,
		}
	}

	for index := range items {
		key, err := keyOfLocation(items[index], roomNames, containerNames)
		if err != nil {
			return nil, err
		}
		w.items[key] = &items[index]
	}

	if err := checkRooms(doc, w); err != nil {
		return nil, err
	}
	if err := checkContainers(doc, w); err != nil {
		return nil, err
	}
	if err := checkItems(doc, w); err != nil {
		return nil, err
	}
	// The order the containers can be applied in, and with it the refusal of
	// a document whose containers hang from each other in a circle.
	if _, err := orderContainers(doc.Containers); err != nil {
		return nil, err
	}
	return w, nil
}

// keyOfLocation keys an item by the names of its place and its own name. The
// document carries names; the inventory carries ids, which are resolved
// against the names loaded above.
func keyOfLocation(item inventory.Item, rooms map[inventory.RoomID]string, containers map[inventory.ContainerID]Location) (itemKey, error) {
	key := itemKey{kind: string(item.Location.Kind), name: scopeKey(item.Name)}
	switch item.Location.Kind {
	case inventory.LocationRoom:
		room, known := rooms[inventory.RoomID(item.Location.ID)]
		if !known {
			return itemKey{}, fmt.Errorf("item %q points at room %d, which does not exist", item.Name, item.Location.ID)
		}
		key.room = scopeKey(room)
	case inventory.LocationContainer:
		container, known := containers[inventory.ContainerID(item.Location.ID)]
		if !known {
			return itemKey{}, fmt.Errorf("item %q points at container %d, which does not exist", item.Name, item.Location.ID)
		}
		key.room, key.container = scopeKey(container.Room), scopeKey(container.Container)
	default:
		return itemKey{}, fmt.Errorf("item %q has an unknown location kind %q", item.Name, item.Location.Kind)
	}
	return key, nil
}

// checkRooms validates the rooms of the document.
func checkRooms(doc Document, w *world) error {
	indexes := make(map[string]int, len(doc.Rooms))
	for index, entry := range doc.Rooms {
		target := inventory.Room{Name: entry.Name, Description: entry.Description}
		target.Normalize()
		if err := target.Validate(); err != nil {
			return at(fmt.Sprintf("rooms[%d]", index), err)
		}
		key := scopeKey(target.Name)
		if previous, duplicate := indexes[key]; duplicate {
			return inventory.NewValidationError(fmt.Sprintf("rooms[%d].name", index),
				fmt.Sprintf("duplicates rooms[%d]", previous))
		}
		indexes[key] = index
		// A room the document declares counts as known for the entries that
		// come after it, even though nothing has been written yet.
		w.knownRooms[key] = true
	}
	return nil
}

// checkContainers validates the containers in three passes: fields and rooms
// first, then the container each one hangs from — with every declared
// container known by now — then the order, where a circle has none.
func checkContainers(doc Document, w *world) error {
	indexes := make(map[containerKey]int, len(doc.Containers))

	for index, entry := range doc.Containers {
		key := containerKey{room: scopeKey(entry.Room), name: scopeKey(entry.Name)}
		if previous, duplicate := indexes[key]; duplicate {
			return inventory.NewValidationError(fmt.Sprintf("containers[%d].name", index),
				fmt.Sprintf("duplicates containers[%d] in the same room", previous))
		}
		indexes[key] = index

		if !w.knownRooms[scopeKey(entry.Room)] {
			return inventory.NewValidationError(fmt.Sprintf("containers[%d].room", index),
				"references a room that is neither in the document nor in the inventory")
		}

		// References are resolved when the entry is applied, by which time a
		// room the document creates has an id; Validate only needs to see
		// that one is coming, so a sentinel stands in for it here.
		target := inventory.Container{Name: entry.Name, Description: entry.Description, RoomID: sentinelRoomID}
		target.Normalize()
		if err := target.Validate(); err != nil {
			return at(fmt.Sprintf("containers[%d]", index), err)
		}
		w.knownContainers[key] = true
	}

	for index, entry := range doc.Containers {
		if entry.Parent == "" {
			continue
		}
		if scopeKey(entry.Parent) == scopeKey(entry.Name) {
			return inventory.NewValidationError(fmt.Sprintf("containers[%d].parent", index),
				"a container cannot be its own parent")
		}
		key := containerKey{room: scopeKey(entry.Room), name: scopeKey(entry.Parent)}
		if !w.knownContainers[key] {
			return inventory.NewValidationError(fmt.Sprintf("containers[%d].parent", index),
				"references a container that is not in this room, in the document or in the inventory")
		}
	}
	return nil
}

// checkItems validates the items of the document.
func checkItems(doc Document, w *world) error {
	indexes := make(map[itemKey]int, len(doc.Items))
	for index, entry := range doc.Items {
		target := inventory.Item{
			Name:        entry.Name,
			Description: entry.Description,
			Quantity:    entry.Quantity,
			Notes:       entry.Notes,
			Tags:        entry.Tags,
			Aliases:     entry.Aliases,
			Location:    sentinelLocation(entry.Location.Kind),
		}
		target.Normalize()
		if err := target.Validate(); err != nil {
			return at(fmt.Sprintf("items[%d]", index), err)
		}

		key := itemKey{
			kind:      entry.Location.Kind,
			room:      scopeKey(entry.Location.Room),
			container: scopeKey(entry.Location.Container),
			name:      scopeKey(target.Name),
		}
		if entry.Location.Kind == "container" {
			if !w.knownContainers[containerKey{room: key.room, name: key.container}] {
				return inventory.NewValidationError(fmt.Sprintf("items[%d].location.container", index),
					"references a container that is not in this room, in the document or in the inventory")
			}
		} else if !w.knownRooms[key.room] {
			return inventory.NewValidationError(fmt.Sprintf("items[%d].location.room", index),
				"references a room that is neither in the document nor in the inventory")
		}

		if previous, duplicate := indexes[key]; duplicate {
			return inventory.NewValidationError(fmt.Sprintf("items[%d].name", index),
				fmt.Sprintf("duplicates items[%d] at the same place", previous))
		}
		indexes[key] = index
	}
	return nil
}

// sentinelRoomID stands in for the id of a room the document is about to
// create: Validate asks that an id be present, not that it exist yet, and the
// reference itself was checked by name.
const sentinelRoomID = inventory.RoomID(1)

// sentinelLocation gives Validate the shape of a place — the kind the document
// claims and a positive id — while the name behind it is still to be
// resolved.
func sentinelLocation(kind string) inventory.Location {
	return inventory.Location{Kind: inventory.LocationKind(kind), ID: int64(sentinelRoomID)}
}

// apply writes the document, entry by entry, keeping the world in step so the
// next entry can reference what this one just created.
func (w *world) apply(ctx context.Context, r Repos, doc Document) (Report, error) {
	var report Report
	if err := w.applyRooms(ctx, r, doc, &report); err != nil {
		return report, err
	}
	if err := w.applyContainers(ctx, r, doc, &report); err != nil {
		return report, err
	}
	if err := w.applyItems(ctx, r, doc, &report); err != nil {
		return report, err
	}
	return report, nil
}

func (w *world) applyRooms(ctx context.Context, r Repos, doc Document, report *Report) error {
	for index, entry := range doc.Rooms {
		target := inventory.Room{Name: entry.Name, Description: entry.Description}
		target.Normalize()
		key := scopeKey(target.Name)
		current, exists := w.rooms[key]
		switch {
		case !exists:
			if err := r.Rooms.Create(ctx, &target); err != nil {
				return at(fmt.Sprintf("rooms[%d]", index), err)
			}
			w.rooms[key] = &target
			report.Rooms.Created++
		case current.Name == target.Name && current.Description == target.Description:
			report.Rooms.Unchanged++
		default:
			target.ID, target.CreatedAt = current.ID, current.CreatedAt
			if err := r.Rooms.Update(ctx, &target); err != nil {
				return at(fmt.Sprintf("rooms[%d]", index), err)
			}
			w.rooms[key] = &target
			report.Rooms.Updated++
		}
	}
	return nil
}

func (w *world) applyContainers(ctx context.Context, r Repos, doc Document, report *Report) error {
	indexes := make(map[containerKey]int, len(doc.Containers))
	for index, entry := range doc.Containers {
		indexes[containerKey{room: scopeKey(entry.Room), name: scopeKey(entry.Name)}] = index
	}
	ordered, err := orderContainers(doc.Containers)
	if err != nil {
		return err
	}

	for _, entry := range ordered {
		key := containerKey{room: scopeKey(entry.Room), name: scopeKey(entry.Name)}
		index := indexes[key]

		target := inventory.Container{Name: entry.Name, Description: entry.Description}
		target.Normalize()
		room, known := w.rooms[scopeKey(entry.Room)]
		if !known {
			// check() refused anything else.
			return fmt.Errorf("room %q disappeared while importing", entry.Room)
		}
		target.RoomID = room.ID
		if entry.Parent != "" {
			parent, known := w.containers[containerKey{room: scopeKey(entry.Room), name: scopeKey(entry.Parent)}]
			if !known {
				return fmt.Errorf("container %q disappeared while importing", entry.Parent)
			}
			parentID := parent.ID
			target.ParentID = &parentID
		}

		current, exists := w.containers[key]
		switch {
		case !exists:
			if err := r.Containers.Create(ctx, &target); err != nil {
				return at(fmt.Sprintf("containers[%d]", index), err)
			}
			w.containers[key] = &target
			report.Containers.Created++
		case containersMatch(current, &target):
			report.Containers.Unchanged++
		default:
			target.ID, target.CreatedAt = current.ID, current.CreatedAt
			if err := r.Containers.Update(ctx, &target); err != nil {
				return at(fmt.Sprintf("containers[%d]", index), err)
			}
			w.containers[key] = &target
			report.Containers.Updated++
		}
	}
	return nil
}

func (w *world) applyItems(ctx context.Context, r Repos, doc Document, report *Report) error {
	for index, entry := range doc.Items {
		target := inventory.Item{
			Name:        entry.Name,
			Description: entry.Description,
			Quantity:    entry.Quantity,
			Notes:       entry.Notes,
			Tags:        entry.Tags,
			Aliases:     entry.Aliases,
		}
		target.Normalize()
		location, err := w.resolve(entry.Location)
		if err != nil {
			return at(fmt.Sprintf("items[%d]", index), err)
		}
		target.Location = location

		key := itemKey{
			kind:      entry.Location.Kind,
			room:      scopeKey(entry.Location.Room),
			container: scopeKey(entry.Location.Container),
			name:      scopeKey(target.Name),
		}
		current, exists := w.items[key]
		switch {
		case !exists:
			if err := r.Items.Create(ctx, &target); err != nil {
				return at(fmt.Sprintf("items[%d]", index), err)
			}
			w.items[key] = &target
			report.Items.Created++
		case itemsMatch(current, &target):
			report.Items.Unchanged++
		default:
			target.ID, target.CreatedAt = current.ID, current.CreatedAt
			if err := r.Items.Update(ctx, &target); err != nil {
				return at(fmt.Sprintf("items[%d]", index), err)
			}
			w.items[key] = &target
			report.Items.Updated++
		}
	}
	return nil
}

// resolve turns a named place into the id the repositories work with.
func (w *world) resolve(location Location) (inventory.Location, error) {
	switch location.Kind {
	case "room":
		room, known := w.rooms[scopeKey(location.Room)]
		if !known {
			return inventory.Location{}, inventory.NewValidationError("location.room",
				"references a room that is neither in the document nor in the inventory")
		}
		return inventory.RoomLocation(room.ID), nil
	case "container":
		container, known := w.containers[containerKey{room: scopeKey(location.Room), name: scopeKey(location.Container)}]
		if !known {
			return inventory.Location{}, inventory.NewValidationError("location.container",
				"references a container that is not in this room, in the document or in the inventory")
		}
		return inventory.ContainerLocation(container.ID), nil
	default:
		return inventory.Location{}, inventory.NewValidationError("location.kind",
			"must be a room or a container location")
	}
}

// containersMatch reports whether what stands already says what the document
// says about a container.
func containersMatch(current, target *inventory.Container) bool {
	if current.Name != target.Name || current.Description != target.Description ||
		current.RoomID != target.RoomID {
		return false
	}
	switch {
	case current.ParentID == nil && target.ParentID == nil:
		return true
	case current.ParentID == nil || target.ParentID == nil:
		return false
	default:
		return *current.ParentID == *target.ParentID
	}
}

// itemsMatch reports whether what stands already says what the document says
// about an item. An absent list and an empty one are the same thing: the
// document leaves a tag list out, the database reads it back as empty.
func itemsMatch(current, target *inventory.Item) bool {
	return current.Name == target.Name &&
		current.Description == target.Description &&
		current.Quantity == target.Quantity &&
		current.Notes == target.Notes &&
		current.Location == target.Location &&
		slices.Equal(current.Tags, target.Tags) &&
		slices.Equal(current.Aliases, target.Aliases)
}

// at points a problem at the entry of the document it came from, so a caller
// learns which line to open: "items[3] quantity must not be negative".
func at(path string, err error) error {
	var validation *inventory.ValidationError
	if !errors.As(err, &validation) {
		return fmt.Errorf("%s: %w", path, err)
	}
	pointed := &inventory.ValidationError{}
	for _, problem := range validation.Problems {
		pointed.Problems = append(pointed.Problems, inventory.FieldProblem{
			Field:   path + "." + problem.Field,
			Problem: problem.Problem,
		})
	}
	return pointed
}
