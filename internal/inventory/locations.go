package inventory

import (
	"fmt"
	"slices"
	"strings"
)

// LocationIndex answers the questions the rest of the system asks about
// locations: which entity a location points at, how a location reads as a
// path, which containers sit inside it, and which location a name or a path
// names. It is built from the rooms and containers of one inventory and holds
// no reference to storage, so every caller that already has those lists — a
// search, a tool call — shares one implementation.
//
// The zero value resolves nothing.
type LocationIndex struct {
	rooms       map[RoomID]Room
	containers  map[ContainerID]Container
	roomList    []Room
	containList []Container
	byPath      map[string]Location
	places      []place
}

// place is one named location, kept in indexing order so lookups by name and
// the messages they produce are ordered the same way every time.
type place struct {
	name     string
	path     string
	location Location
}

// NewLocationIndex indexes the rooms and containers of an inventory.
func NewLocationIndex(rooms []Room, containers []Container) LocationIndex {
	index := LocationIndex{
		rooms:       make(map[RoomID]Room, len(rooms)),
		containers:  make(map[ContainerID]Container, len(containers)),
		roomList:    rooms,
		containList: containers,
		byPath:      make(map[string]Location, len(rooms)+len(containers)),
		places:      make([]place, 0, len(rooms)+len(containers)),
	}
	for _, room := range rooms {
		index.rooms[room.ID] = room
		index.byPath[pathKey(room.Name)] = RoomLocation(room.ID)
		index.places = append(index.places, place{
			name:     FoldText(room.Name),
			path:     room.Name,
			location: RoomLocation(room.ID),
		})
	}
	// Every container goes in the map before any path is built: a listing is
	// ordered by name, so a drawer can come before the toolbox that holds it,
	// and a path must not depend on that.
	for _, container := range containers {
		index.containers[container.ID] = container
	}
	for _, container := range containers {
		path := index.pathOf(container)
		if path == "" {
			continue
		}
		index.byPath[pathKey(path)] = ContainerLocation(container.ID)
		index.places = append(index.places, place{
			name:     FoldText(container.Name),
			path:     path,
			location: ContainerLocation(container.ID),
		})
	}
	return index
}

// Path renders a location as "Garage > Toolbox > Drawer 1". A location the
// index cannot resolve renders as an empty path: a caller never fails because
// of a dangling reference.
func (i LocationIndex) Path(location Location) string {
	if roomID, ok := location.RoomID(); ok {
		return i.rooms[roomID].Name
	}
	containerID, ok := location.ContainerID()
	if !ok {
		return ""
	}
	container, ok := i.containers[containerID]
	if !ok {
		return ""
	}
	return i.pathOf(container)
}

// Children returns the containers that sit directly inside a location, in
// listing order.
func (i LocationIndex) Children(location Location) []Container {
	var children []Container
	roomID, isRoom := location.RoomID()
	parentID, isContainer := location.ContainerID()
	if isRoom {
		if _, ok := i.rooms[roomID]; !ok {
			return nil
		}
	}
	if isContainer {
		if _, ok := i.containers[parentID]; !ok {
			return nil
		}
	}
	for _, container := range i.containList {
		switch {
		case isRoom && container.RoomID == roomID && i.isRoot(container):
			children = append(children, container)
		case isContainer && container.ParentID != nil && *container.ParentID == parentID:
			children = append(children, container)
		}
	}
	return children
}

// isRoot reports whether a container sits directly in its room: either it has
// no parent, or the parent it names is not in the inventory, in which case it
// is as close to the room as the tree allows. Path renders it that way too.
func (i LocationIndex) isRoot(container Container) bool {
	if container.ParentID == nil {
		return true
	}
	_, known := i.containers[*container.ParentID]
	return !known
}

// Resolve turns a name or a path such as "Garage" or "Garage > Toolbox" into a
// location. Both sides are folded, so case and accents never decide, and a
// name that only one place carries works on its own. A name shared by several
// places fails with ErrAmbiguous, carrying the candidates so the caller can
// ask which one was meant.
func (i LocationIndex) Resolve(name string) (Location, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Location{}, fmt.Errorf("%w: location must not be empty", ErrValidation)
	}
	if location, ok := i.byPath[pathKey(trimmed)]; ok {
		return location, nil
	}
	// A path that does not match stays unmatched: answering with a place in
	// another room because the last segment happens to be unique would be a
	// silent wrong answer.
	if strings.Contains(trimmed, ">") {
		return Location{}, fmt.Errorf("%w: no place called %q", ErrNotFound, trimmed)
	}

	// A name on its own still names one place, as long as only one place
	// carries it: "Toolbox" is enough when there is one toolbox, and
	// ambiguous when there are two.
	wanted := FoldText(trimmed)
	var matches []place
	for _, candidate := range i.places {
		if candidate.name == wanted {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return Location{}, fmt.Errorf("%w: no place called %q", ErrNotFound, trimmed)
	case 1:
		return matches[0].location, nil
	}
	paths := make([]string, len(matches))
	for index, match := range matches {
		paths[index] = match.path
	}
	return Location{}, &AmbiguousError{Name: trimmed, Candidates: paths}
}

// Ancestors returns the places that hold a location, from the location itself
// up to its room, so a caller can ask about a place as a whole: the ancestors
// of an item in "Garage > Toolbox > Cassetto 1" are that drawer, the toolbox
// and the garage. The walk stops at a dangling or cyclic parent, so it always
// terminates, and a location the index does not know has no ancestors.
func (i LocationIndex) Ancestors(location Location) []Location {
	if roomID, ok := location.RoomID(); ok {
		if _, known := i.rooms[roomID]; !known {
			return nil
		}
		return []Location{location}
	}
	containerID, ok := location.ContainerID()
	if !ok {
		return nil
	}
	container, ok := i.containers[containerID]
	if !ok {
		return nil
	}

	ancestors := []Location{ContainerLocation(container.ID)}
	seen := map[ContainerID]bool{container.ID: true}
	for {
		if container.ParentID == nil {
			break
		}
		parent, ok := i.containers[*container.ParentID]
		if !ok || seen[parent.ID] {
			break
		}
		seen[parent.ID] = true
		ancestors = append(ancestors, ContainerLocation(parent.ID))
		container = parent
	}
	room, ok := i.rooms[container.RoomID]
	if !ok {
		return ancestors
	}
	return append(ancestors, RoomLocation(room.ID))
}

// pathOf walks a container up to its room, stopping at a dangling or cyclic
// parent so a broken tree renders as the part that is still connected. It
// returns an empty path when the room itself is missing.
func (i LocationIndex) pathOf(container Container) string {
	chain := make([]Container, 0, 4)
	seen := make(map[ContainerID]bool, 4)
	for {
		if seen[container.ID] {
			break
		}
		seen[container.ID] = true
		chain = append(chain, container)
		if container.ParentID == nil {
			break
		}
		parent, ok := i.containers[*container.ParentID]
		if !ok {
			break
		}
		container = parent
	}
	if len(chain) == 0 {
		return ""
	}
	room, ok := i.rooms[chain[0].RoomID]
	if !ok {
		return ""
	}
	slices.Reverse(chain)
	return ContainerPath{Room: room, Containers: chain}.String()
}

// pathKey folds a path and normalises its separators, so "garage>toolbox",
// "Garage >  Toolbox" and "Garage > Toolbox" are the same key.
func pathKey(path string) string {
	segments := strings.Split(path, ">")
	for index, segment := range segments {
		segments[index] = FoldText(strings.TrimSpace(segment))
	}
	return strings.Join(segments, ">")
}
