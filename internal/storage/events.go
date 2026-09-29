package storage

// The event log is written by the repositories: every mutation records itself
// inside its own transaction, so an event that is missing means the mutation
// did not happen. This file is the emission and the read side; the shape lives
// in internal/events.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/nicolasalberti00/homey/internal/events"
	"github.com/nicolasalberti00/homey/internal/inventory"
)

const (
	insertEventSQL = `
INSERT INTO events (type, entity_kind, entity_id, actor, tool, confirmation, payload)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id`

	recentEventsSQL = `
SELECT id, type, entity_kind, entity_id, actor, tool, confirmation, payload, at
FROM events
ORDER BY id DESC
LIMIT ?`

	entityEventsSQL = `
SELECT id, type, entity_kind, entity_id, actor, tool, confirmation, payload, at
FROM events
WHERE entity_kind = ? AND entity_id = ?
ORDER BY id`
)

// record writes one event inside the transaction the mutation is already in.
// The actor comes from the context, so a transport that forgets to say who is
// calling records an unattributed mutation instead of a wrong one.
func record(ctx context.Context, q querier, eventType string, kind events.EntityKind, id int64, payload map[string]any) error {
	event := events.Event{Type: eventType, EntityKind: kind, EntityID: id, Payload: payload}
	if actor, found := events.ActorFrom(ctx); found {
		event.Actor, event.Tool, event.Confirmation = actor.Name, actor.Tool, actor.Confirmation
	}
	if err := event.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encoding the payload of %s: %w", event.Type, err)
	}
	var stored int64
	if err := q.QueryRowContext(ctx, insertEventSQL,
		event.Type, string(event.EntityKind), event.EntityID,
		event.Actor, event.Tool, event.Confirmation, string(encoded),
	).Scan(&stored); err != nil {
		return fmt.Errorf("recording %s: %w", event.Type, err)
	}
	return nil
}

// itemEvent carries the state of an item, the way a replay or an undo needs it.
func itemEvent(item inventory.Item) map[string]any {
	return map[string]any{"item": map[string]any{
		"id":          item.ID,
		"name":        item.Name,
		"description": item.Description,
		"quantity":    item.Quantity,
		"notes":       item.Notes,
		"tags":        stringsOrEmpty(item.Tags),
		"aliases":     stringsOrEmpty(item.Aliases),
		"location":    locationValue(item.Location),
	}}
}

// containerEvent carries the state of a container.
func containerEvent(container inventory.Container) map[string]any {
	return map[string]any{"container": map[string]any{
		"id":          container.ID,
		"room_id":     container.RoomID,
		"parent_id":   nullableContainerID(container.ParentID),
		"name":        container.Name,
		"description": container.Description,
	}}
}

// roomEvent carries the state of a room.
func roomEvent(room inventory.Room) map[string]any {
	return map[string]any{"room": map[string]any{
		"id":          room.ID,
		"name":        room.Name,
		"description": room.Description,
	}}
}

// moveEvent carries the two places of a move.
func moveEvent(from, to inventory.Location) map[string]any {
	return map[string]any{"from": locationValue(from), "to": locationValue(to)}
}

// locationValue renders a location as JSON, so a payload names places the way
// the domain does.
func locationValue(location inventory.Location) map[string]any {
	return map[string]any{"kind": string(location.Kind), "id": location.ID}
}

// stringsOrEmpty keeps an absent list a list: a payload that says null where a
// reader expects a set is a payload to argue with.
func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// eventStore is the SQLite implementation of events.Store.
type eventStore struct {
	db *sql.DB
}

// NewEventStore returns the reader of the event log the repositories write.
func NewEventStore(db *sql.DB) events.Store {
	return &eventStore{db: db}
}

// Recent implements events.Store.
func (s *eventStore) Recent(ctx context.Context, limit int) ([]events.Event, error) {
	if limit < 0 {
		return nil, fmt.Errorf("%w: the limit must not be negative", events.ErrInvalid)
	}
	// SQLite treats a negative limit as no limit, which is what zero means here.
	if limit == 0 {
		limit = -1
	}
	rows, err := s.db.QueryContext(ctx, recentEventsSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("reading recent events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ForEntity implements events.Store.
func (s *eventStore) ForEntity(ctx context.Context, kind events.EntityKind, id int64) ([]events.Event, error) {
	if kind != events.EntityRoom && kind != events.EntityContainer && kind != events.EntityItem {
		return nil, fmt.Errorf("%w: unknown entity kind %q", events.ErrInvalid, kind)
	}
	if id == 0 {
		return nil, fmt.Errorf("%w: entity id must not be zero", events.ErrInvalid)
	}
	rows, err := s.db.QueryContext(ctx, entityEventsSQL, string(kind), id)
	if err != nil {
		return nil, fmt.Errorf("reading events of %s %d: %w", kind, id, err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// scanEvents reads a query result into events, decoding the payload so a
// reader gets values and never a string to parse again.
func scanEvents(rows *sql.Rows) ([]events.Event, error) {
	history := make([]events.Event, 0, 8)
	for rows.Next() {
		var (
			event events.Event
			kind  string
			raw   string
			at    string
		)
		if err := rows.Scan(&event.ID, &event.Type, &kind, &event.EntityID,
			&event.Actor, &event.Tool, &event.Confirmation, &raw, &at); err != nil {
			return nil, fmt.Errorf("scanning an event: %w", err)
		}
		event.EntityKind = events.EntityKind(kind)
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("event %d: %w", event.ID, err)
		}
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, fmt.Errorf("decoding the payload of event %d: %w", event.ID, err)
		}
		event.Payload = payload
		stampedAt, err := parseTime(at)
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", event.ID, err)
		}
		event.At = stampedAt
		history = append(history, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading events: %w", err)
	}
	return history, nil
}
