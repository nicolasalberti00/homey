package inventory

import (
	"fmt"
	"strings"
)

// Centralized limits and rules for the inventory domain. Every interface —
// REST API, MCP tools, Web UI, future voice adapters — relies on these, so
// adjustments are made here and nowhere else.
const (
	// MaxNameLen is the maximum length of a room, container or item name.
	MaxNameLen = 120
	// MaxDescriptionLen is the maximum length of a description.
	MaxDescriptionLen = 2000
	// MaxNotesLen is the maximum length of item notes.
	MaxNotesLen = 4000
	// MaxQuantity is the maximum item quantity accepted. The lower bound is
	// zero: negative quantities are always invalid.
	MaxQuantity = 1_000_000
)

// Normalize trims surrounding whitespace from the user-provided text fields
// of the room. Call it before persisting.
func (r *Room) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
}

// Normalize trims surrounding whitespace from the user-provided text fields
// of the container. Call it before persisting.
func (c *Container) Normalize() {
	c.Name = strings.TrimSpace(c.Name)
	c.Description = strings.TrimSpace(c.Description)
}

// Normalize trims surrounding whitespace from the user-provided text fields
// of the item. Call it before persisting.
func (i *Item) Normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Description = strings.TrimSpace(i.Description)
	i.Notes = strings.TrimSpace(i.Notes)
}

// Validate checks the room fields against the domain rules.
func (r *Room) Validate() error {
	v := &ValidationError{}
	validateName(v, "name", r.Name)
	validateLength(v, "description", r.Description, MaxDescriptionLen)
	return v.orNil()
}

// Validate checks the container fields. Parent existence, same-room nesting
// and cycle prevention need storage access and are enforced by the operations
// of this package, not here.
func (c *Container) Validate() error {
	v := &ValidationError{}
	validateName(v, "name", c.Name)
	validateLength(v, "description", c.Description, MaxDescriptionLen)
	if c.RoomID <= 0 {
		v.add("room_id", "must reference an existing room")
	}
	if c.ParentID != nil {
		if *c.ParentID <= 0 {
			v.add("parent_id", "must reference an existing container or be empty")
		} else if c.ID != 0 && *c.ParentID == c.ID {
			v.add("parent_id", "a container cannot be its own parent")
		}
	}
	return v.orNil()
}

// Validate checks the item fields, including its location.
func (i *Item) Validate() error {
	v := &ValidationError{}
	validateName(v, "name", i.Name)
	validateLength(v, "description", i.Description, MaxDescriptionLen)
	validateLength(v, "notes", i.Notes, MaxNotesLen)
	if i.Quantity < 0 {
		v.add("quantity", "must not be negative")
	} else if i.Quantity > MaxQuantity {
		v.add("quantity", fmt.Sprintf("must be at most %d", MaxQuantity))
	}
	validateLocation(v, "location", i.Location)
	return v.orNil()
}

// Validate checks that the location references exactly one existing room or
// container.
func (l Location) Validate() error {
	v := &ValidationError{}
	validateLocation(v, "location", l)
	return v.orNil()
}

func validateName(v *ValidationError, field, name string) {
	if strings.TrimSpace(name) == "" {
		v.add(field, "must not be empty")
		return
	}
	validateLength(v, field, name, MaxNameLen)
}

func validateLength(v *ValidationError, field, value string, max int) {
	if n := len([]rune(strings.TrimSpace(value))); n > max {
		v.add(field, fmt.Sprintf("must be at most %d characters", max))
	}
}

func validateLocation(v *ValidationError, field string, l Location) {
	if !l.Kind.Valid() {
		v.add(field+".kind", "must be a room or a container location")
		return
	}
	if l.ID <= 0 {
		v.add(field+".id", "must reference an existing room or container")
	}
}
