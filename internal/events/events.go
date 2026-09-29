// Package events is the audit trail of the inventory: one record per mutation,
// written in the same transaction as the mutation it describes, so the history
// is complete and a future undo has the state it needs to restore.
//
// Emission lives with the mutations in the storage adapter, which stamps the
// actor it finds in the context; a transport (the REST middleware, the MCP
// adapter) puts that actor in. This package only owns the shape and the
// read side.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// EntityKind is the kind of entity an event is about.
type EntityKind string

const (
	// EntityRoom is a room event.
	EntityRoom EntityKind = "room"
	// EntityContainer is a container event.
	EntityContainer EntityKind = "container"
	// EntityItem is an item event.
	EntityItem EntityKind = "item"
)

// The event types, one per mutation.
const (
	RoomCreated      = "room.created"
	RoomUpdated      = "room.updated"
	RoomDeleted      = "room.deleted"
	ContainerCreated = "container.created"
	ContainerUpdated = "container.updated"
	ContainerDeleted = "container.deleted"
	ContainerMoved   = "container.moved"
	ItemCreated      = "item.created"
	ItemUpdated      = "item.updated"
	ItemDeleted      = "item.deleted"
	ItemMoved        = "item.moved"
)

// Actor is who is behind a mutation, as the transport that authenticated the
// request knows it. The zero value is the system acting on its own.
type Actor struct {
	// Name identifies the caller: the token's name.
	Name string
	// Tool is the MCP tool that carried the mutation out, when one did.
	Tool string
	// Confirmation says how the destructive step was authorised, when the
	// tool had to ask: confirmed or bypassed.
	Confirmation string
}

// ActorKey is the context key the actor travels under, exported so a transport
// that wraps the context it does not own (a Huma middleware, say) can attach it.
type ActorKey struct{}

// WithActor returns a context carrying who is behind the mutations it drives.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, ActorKey{}, actor)
}

// ActorFrom returns the actor in ctx. Without one the mutation is the system's
// own, which is recorded with an empty name.
func ActorFrom(ctx context.Context) (Actor, bool) {
	actor, found := ctx.Value(ActorKey{}).(Actor)
	return actor, found
}

// Event is one thing that happened, in the shape the audit trail needs and a
// future undo can replay.
type Event struct {
	ID           int64
	Type         string
	EntityKind   EntityKind
	EntityID     int64
	Actor        string
	Tool         string
	Confirmation string
	// Payload carries what the event needs to be replayed: the state of the
	// entity as it now stands, the state it was in when it went away, or the
	// places of a move.
	Payload map[string]any
	At      time.Time
}

// Store is the read side of the event log: the mutations write, this reads.
type Store interface {
	// Recent returns at most limit events, newest first.
	Recent(ctx context.Context, limit int) ([]Event, error)
	// ForEntity returns the events of one entity, oldest first, so its story
	// can be replayed in order.
	ForEntity(ctx context.Context, kind EntityKind, id int64) ([]Event, error)
}

// maxTypeLen keeps a type like "item.moved" a type.
const maxTypeLen = 40

// eventPattern is what a type looks like: entity and action, dot-joined.
var eventPattern = regexp.MustCompile(`^[a-z]+\.[a-z_]+$`)

// Validate checks the fields a writer fills, so a malformed event is refused
// rather than recorded in a way a reader cannot trust.
func (e Event) Validate() error {
	switch {
	case !eventPattern.MatchString(e.Type) || len(e.Type) > maxTypeLen:
		return fmt.Errorf("%w: event type %q must be \"entity.action\" up to %d characters",
			ErrInvalid, e.Type, maxTypeLen)
	case e.EntityKind != EntityRoom && e.EntityKind != EntityContainer && e.EntityKind != EntityItem:
		return fmt.Errorf("%w: unknown entity kind %q", ErrInvalid, e.EntityKind)
	case e.EntityID == 0:
		return fmt.Errorf("%w: entity id must not be zero", ErrInvalid)
	}
	return nil
}

// ErrInvalid reports an event that cannot be recorded as it is.
var ErrInvalid = errors.New("events: invalid event")

// String renders the event for logs, in one line with what matters.
func (e Event) String() string {
	payload, _ := json.Marshal(e.Payload)
	return fmt.Sprintf("%s %s/%d by %s via %s %s", e.Type, e.EntityKind, e.EntityID, e.Actor, e.Tool, payload)
}
