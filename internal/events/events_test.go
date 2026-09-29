package events

import (
	"errors"
	"strings"
	"testing"
)

// TestActorTravelsInTheContext checks the transport can hand an identity down
// to the storage layer without the repositories knowing about tokens.
func TestActorTravelsInTheContext(t *testing.T) {
	if _, found := ActorFrom(t.Context()); found {
		t.Fatal("a bare context already carried an actor")
	}
	want := Actor{Name: "nick", Tool: "delete_item", Confirmation: "bypassed"}
	got, found := ActorFrom(WithActor(t.Context(), want))
	if !found {
		t.Fatal("the actor did not travel in the context")
	}
	if got != want {
		t.Fatalf("actor = %+v, want %+v", got, want)
	}
}

// TestValidateRefusesWhatItCannotRecord keeps a malformed event out of the log
// instead of writing one a reader cannot trust.
func TestValidateRefusesWhatItCannotRecord(t *testing.T) {
	valid := func(mutate func(*Event)) Event {
		event := Event{Type: ItemCreated, EntityKind: EntityItem, EntityID: 7}
		mutate(&event)
		return event
	}
	cases := []struct {
		name  string
		event Event
	}{
		{"no type", valid(func(e *Event) { e.Type = "" })},
		{"a type that is not entity.action", valid(func(e *Event) { e.Type = "item" })},
		{"an overlong type", valid(func(e *Event) {
			e.Type = strings.Repeat("a", maxTypeLen+1) + ".created"
		})},
		{"an unknown entity", valid(func(e *Event) { e.EntityKind = "truck" })},
		{"no entity", valid(func(e *Event) { e.EntityID = 0 })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.event.Validate()
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate = %v, want ErrInvalid", err)
			}
		})
	}

	for _, event := range []Event{
		{Type: ItemMoved, EntityKind: EntityItem, EntityID: 3},
		{Type: RoomDeleted, EntityKind: EntityRoom, EntityID: 9},
		{Type: ContainerUpdated, EntityKind: EntityContainer, EntityID: 1},
	} {
		if err := event.Validate(); err != nil {
			t.Fatalf("Validate of %+v = %v, want it valid", event, err)
		}
	}
}

// TestStringIsOneLine checks the log form of an event stays one line.
func TestStringIsOneLine(t *testing.T) {
	event := Event{
		Type: ItemDeleted, EntityKind: EntityItem, EntityID: 3,
		Actor: "automation", Tool: "delete_item", Confirmation: "bypassed",
		Payload: map[string]any{"item": map[string]any{"name": "Cavo"}},
	}
	line := event.String()
	if strings.Count(line, "\n") != 0 || !strings.Contains(line, "item.deleted item/3") {
		t.Fatalf("String = %q, want one line naming type and entity", line)
	}
}
