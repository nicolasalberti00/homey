package storage

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/nicolasalberti00/homey/internal/events"
	"github.com/nicolasalberti00/homey/internal/inventory"
)

// eventFixture is a home whose mutations are watched by the event store.
type eventFixture struct {
	repos Repos
	store events.Store
}

func newEventFixture(t *testing.T) (eventFixture, context.Context) {
	t.Helper()
	db := openDB(t, mustMigrate(t))
	return eventFixture{repos: NewRepos(db), store: NewEventStore(db)}, t.Context()
}

// history reads at most limit events, newest first.
func (f eventFixture) history(t *testing.T, ctx context.Context, limit int) []events.Event {
	t.Helper()
	history, err := f.store.Recent(ctx, limit)
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}
	return history
}

// types collects the event types, in the order they were read.
func types(history []events.Event) []string {
	got := make([]string, len(history))
	for index, event := range history {
		got[index] = event.Type
	}
	return got
}

// decode renders a payload as a typed shape, the way a reader of the log does.
func decode(t *testing.T, payload map[string]any, out any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encoding a payload: %v", err)
	}
	if err := json.Unmarshal(encoded, out); err != nil {
		t.Fatalf("decoding %v: %v", payload, err)
	}
}

// itemPayload is what an item looks like inside an event.
type itemPayload struct {
	Item struct {
		ID       int64    `json:"id"`
		Name     string   `json:"name"`
		Notes    string   `json:"notes"`
		Quantity int      `json:"quantity"`
		Tags     []string `json:"tags"`
		Aliases  []string `json:"aliases"`
		Location struct {
			Kind string `json:"kind"`
			ID   int64  `json:"id"`
		} `json:"location"`
	} `json:"item"`
}

// TestMutationsAreRecorded walks the first three mutations of an inventory and
// checks the events land next to them, with the state a replay needs.
func TestMutationsAreRecorded(t *testing.T) {
	f, ctx := newEventFixture(t)

	room := inventory.Room{Name: "Garage", Description: "dove parcheggio"}
	if err := f.repos.Rooms.Create(ctx, &room); err != nil {
		t.Fatalf("creating room: %v", err)
	}
	container := inventory.Container{RoomID: room.ID, Name: "Toolbox", Description: "cassetta"}
	if err := f.repos.Containers.Create(ctx, &container); err != nil {
		t.Fatalf("creating container: %v", err)
	}
	item := inventory.Item{
		Name: "Trapano", Description: "Bosch blu", Quantity: 1, Notes: "regalo di papà",
		Tags: []string{"officina"}, Aliases: []string{"avvitatore"},
		Location: inventory.ContainerLocation(container.ID),
	}
	if err := f.repos.Items.Create(ctx, &item); err != nil {
		t.Fatalf("creating item: %v", err)
	}

	history := f.history(t, ctx, 10)
	if got := types(history); !slices.Equal(got, []string{
		events.ItemCreated, events.ContainerCreated, events.RoomCreated,
	}) {
		t.Fatalf("event types = %v, want one per mutation, newest first", got)
	}

	var itemSeen itemPayload
	decode(t, history[0].Payload, &itemSeen)
	if itemSeen.Item.ID != int64(item.ID) || itemSeen.Item.Name != "Trapano" || itemSeen.Item.Quantity != 1 {
		t.Fatalf("item = %+v, want the item as stored", itemSeen.Item)
	}
	if !slices.Equal(itemSeen.Item.Tags, []string{"officina"}) || !slices.Equal(itemSeen.Item.Aliases, []string{"avvitatore"}) {
		t.Fatalf("item = %+v, want the tags and the aliases", itemSeen.Item)
	}
	if itemSeen.Item.Location.Kind != "container" || itemSeen.Item.Location.ID != int64(container.ID) {
		t.Fatalf("location = %+v, want the toolbox", itemSeen.Item.Location)
	}

	var containerSeen struct {
		Container struct {
			ID          int64  `json:"id"`
			RoomID      int64  `json:"room_id"`
			ParentID    *int64 `json:"parent_id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"container"`
	}
	decode(t, history[1].Payload, &containerSeen)
	if containerSeen.Container.Name != "Toolbox" || containerSeen.Container.RoomID != int64(room.ID) {
		t.Fatalf("container = %+v, want the toolbox as stored", containerSeen.Container)
	}

	var roomSeen struct {
		Room struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"room"`
	}
	decode(t, history[2].Payload, &roomSeen)
	if roomSeen.Room.ID != int64(room.ID) || roomSeen.Room.Name != "Garage" || roomSeen.Room.Description != "dove parcheggio" {
		t.Fatalf("room = %+v, want the room as stored", roomSeen.Room)
	}
}

// TestUpdatedAndDeletedCarryTheState checks the two mutations a replay needs
// the most context for: an update answers with the state as stored, a delete
// with the state as it was.
func TestUpdatedAndDeletedCarryTheState(t *testing.T) {
	f, ctx := newEventFixture(t)
	room := inventory.Room{Name: "Garage"}
	if err := f.repos.Rooms.Create(ctx, &room); err != nil {
		t.Fatalf("creating room: %v", err)
	}
	item := inventory.Item{
		Name: "Trapano", Quantity: 1, Tags: []string{"officina"},
		Location: inventory.RoomLocation(room.ID),
	}
	if err := f.repos.Items.Create(ctx, &item); err != nil {
		t.Fatalf("creating item: %v", err)
	}

	item.Quantity = 3
	item.Tags = []string{"officina", "nuovo"}
	if err := f.repos.Items.Update(ctx, &item); err != nil {
		t.Fatalf("updating item: %v", err)
	}
	if err := f.repos.Items.Delete(ctx, item.ID); err != nil {
		t.Fatalf("deleting item: %v", err)
	}

	history := f.history(t, ctx, 10)
	if got := types(history); !slices.Equal(got, []string{
		events.ItemDeleted, events.ItemUpdated, events.ItemCreated, events.RoomCreated,
	}) {
		t.Fatalf("event types = %v, want one per mutation", got)
	}

	var updated itemPayload
	decode(t, history[1].Payload, &updated)
	if updated.Item.Quantity != 3 || !slices.Equal(updated.Item.Tags, []string{"nuovo", "officina"}) {
		t.Fatalf("updated = %+v, want the state as stored", updated.Item)
	}

	var deleted itemPayload
	decode(t, history[0].Payload, &deleted)
	if deleted.Item.Name != "Trapano" || deleted.Item.Quantity != 3 {
		t.Fatalf("deleted = %+v, want the state as it was", deleted.Item)
	}
	if !slices.Equal(deleted.Item.Tags, []string{"nuovo", "officina"}) {
		t.Fatalf("deleted = %+v, want the tags it had", deleted.Item)
	}
}

// TestMovesRecordWhereFromAndTo checks the move events say both places, for the
// item and for the container that takes its subtree with it.
func TestMovesRecordWhereFromAndTo(t *testing.T) {
	f, ctx := newEventFixture(t)
	garage := inventory.Room{Name: "Garage"}
	if err := f.repos.Rooms.Create(ctx, &garage); err != nil {
		t.Fatalf("creating garage: %v", err)
	}
	item := inventory.Item{Name: "Trapano", Quantity: 1, Location: inventory.RoomLocation(garage.ID)}
	if err := f.repos.Items.Create(ctx, &item); err != nil {
		t.Fatalf("creating item: %v", err)
	}

	container := inventory.Container{RoomID: garage.ID, Name: "Toolbox"}
	if err := f.repos.Containers.Create(ctx, &container); err != nil {
		t.Fatalf("creating container: %v", err)
	}

	if err := f.repos.Items.Move(ctx, item.ID, inventory.ContainerLocation(container.ID)); err != nil {
		t.Fatalf("moving item: %v", err)
	}
	cucina := inventory.Room{Name: "Cucina"}
	if err := f.repos.Rooms.Create(ctx, &cucina); err != nil {
		t.Fatalf("creating kitchen: %v", err)
	}
	if err := f.repos.Containers.Move(ctx, container.ID, inventory.RoomLocation(cucina.ID)); err != nil {
		t.Fatalf("moving container: %v", err)
	}

	history := f.history(t, ctx, 10)
	if got := types(history); !slices.Equal(got, []string{
		events.ContainerMoved, events.RoomCreated, events.ItemMoved,
		events.ContainerCreated, events.ItemCreated, events.RoomCreated,
	}) {
		t.Fatalf("event types = %v, want one per mutation", got)
	}

	var itemMove struct {
		From locationPayload `json:"from"`
		To   locationPayload `json:"to"`
	}
	decode(t, history[2].Payload, &itemMove)
	if itemMove.From.Kind != "room" || itemMove.From.ID != int64(garage.ID) {
		t.Fatalf("from = %+v, want the garage", itemMove.From)
	}
	if itemMove.To.Kind != "container" || itemMove.To.ID != int64(container.ID) {
		t.Fatalf("to = %+v, want the toolbox", itemMove.To)
	}

	var containerMove struct {
		From locationPayload `json:"from"`
		To   locationPayload `json:"to"`
	}
	decode(t, history[0].Payload, &containerMove)
	if containerMove.From.Kind != "room" || containerMove.From.ID != int64(garage.ID) {
		t.Fatalf("from = %+v, want the garage", containerMove.From)
	}
	if containerMove.To.Kind != "room" || containerMove.To.ID != int64(cucina.ID) {
		t.Fatalf("to = %+v, want the kitchen", containerMove.To)
	}
}

// TestEventsRollBackWithTheMutation is the guarantee the events table stands
// on: a mutation that never happened leaves no event behind.
func TestEventsRollBackWithTheMutation(t *testing.T) {
	f, ctx := newEventFixture(t)
	room := inventory.Room{Name: "Garage"}
	if err := f.repos.Rooms.Create(ctx, &room); err != nil {
		t.Fatalf("creating room: %v", err)
	}
	item := inventory.Item{Name: "Trapano", Quantity: 1, Location: inventory.RoomLocation(room.ID)}
	if err := f.repos.Items.Create(ctx, &item); err != nil {
		t.Fatalf("creating item: %v", err)
	}

	twin := inventory.Item{Name: "Trapano", Quantity: 1, Location: inventory.RoomLocation(room.ID)}
	err := f.repos.Items.Create(ctx, &twin)
	if !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("creating a duplicate = %v, want ErrConflict", err)
	}

	history := f.history(t, ctx, 10)
	if got := types(history); len(got) != 2 || got[0] != events.ItemCreated {
		t.Fatalf("event types = %v, want the room and the item only", got)
	}
}

// TestActorIsRecorded checks the audit half: who did it, through which tool,
// and how the destructive step was authorised. Without an actor the mutation
// is the system's own.
func TestActorIsRecorded(t *testing.T) {
	f, _ := newEventFixture(t)

	garage := inventory.Room{Name: "Garage"}
	roomCtx := events.WithActor(context.Background(), events.Actor{Name: "nick"})
	if err := f.repos.Rooms.Create(roomCtx, &garage); err != nil {
		t.Fatalf("creating room: %v", err)
	}

	item := inventory.Item{Name: "Trapano", Quantity: 1, Location: inventory.RoomLocation(garage.ID)}
	itemCtx := events.WithActor(context.Background(), events.Actor{
		Name: "nick", Tool: "add_item",
	})
	if err := f.repos.Items.Create(itemCtx, &item); err != nil {
		t.Fatalf("creating item: %v", err)
	}
	if err := f.repos.Items.Delete(events.WithActor(context.Background(), events.Actor{
		Name: "nick", Tool: "delete_item", Confirmation: "bypassed",
	}), item.ID); err != nil {
		t.Fatalf("deleting item: %v", err)
	}
	if err := f.repos.Items.Delete(context.Background(), item.ID); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("deleting a missing item = %v, want ErrNotFound", err)
	}

	history := f.history(t, context.Background(), 10)
	if got := types(history); !slices.Equal(got, []string{
		events.ItemDeleted, events.ItemCreated, events.RoomCreated,
	}) {
		t.Fatalf("event types = %v, want one per mutation", got)
	}
	if history[0].Actor != "nick" || history[0].Tool != "delete_item" || history[0].Confirmation != "bypassed" {
		t.Fatalf("event = %+v, want the LLM audit columns", history[0])
	}
	if history[1].Tool != "add_item" || history[1].Confirmation != "" {
		t.Fatalf("event = %+v, want the tool without a confirmation", history[1])
	}
	if history[2].Actor != "nick" || history[2].Tool != "" || history[2].Confirmation != "" {
		t.Fatalf("event = %+v, want the caller without a tool", history[2])
	}
}

// TestRecentAndForEntity checks the read side: a limit cuts the newest, and one
// entity's story comes back oldest first.
func TestRecentAndForEntity(t *testing.T) {
	f, ctx := newEventFixture(t)
	room := inventory.Room{Name: "Garage"}
	if err := f.repos.Rooms.Create(ctx, &room); err != nil {
		t.Fatalf("creating room: %v", err)
	}
	item := inventory.Item{Name: "Trapano", Quantity: 1, Location: inventory.RoomLocation(room.ID)}
	if err := f.repos.Items.Create(ctx, &item); err != nil {
		t.Fatalf("creating item: %v", err)
	}
	item.Quantity = 2
	if err := f.repos.Items.Update(ctx, &item); err != nil {
		t.Fatalf("updating item: %v", err)
	}

	history := f.history(t, ctx, 2)
	if got := types(history); !slices.Equal(got, []string{events.ItemUpdated, events.ItemCreated}) {
		t.Fatalf("recent = %v, want the two newest", got)
	}

	story, err := f.store.ForEntity(ctx, events.EntityItem, int64(item.ID))
	if err != nil {
		t.Fatalf("reading the entity's events: %v", err)
	}
	if got := types(story); !slices.Equal(got, []string{events.ItemCreated, events.ItemUpdated}) {
		t.Fatalf("story = %v, want the item's events oldest first", got)
	}
	other, err := f.store.ForEntity(ctx, events.EntityContainer, int64(item.ID))
	if err != nil {
		t.Fatalf("reading an entity with no events: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("story = %v, want none for an entity that was never touched", other)
	}

	if _, err := f.store.Recent(ctx, -1); err == nil {
		t.Fatal("a negative limit was accepted")
	}
	if _, err := f.store.Recent(ctx, 0); err != nil {
		t.Fatalf("a zero limit = %v, want everything", err)
	}
}

// TestDeletesAndUpdatesOfPlaces checks the rooms and containers write their own
// events, and that a room that cannot go (it is not empty) leaves nothing.
func TestContainersAndRoomsLifecycle(t *testing.T) {
	f, ctx := newEventFixture(t)
	garage := inventory.Room{Name: "Garage"}
	if err := f.repos.Rooms.Create(ctx, &garage); err != nil {
		t.Fatalf("creating garage: %v", err)
	}
	container := inventory.Container{RoomID: garage.ID, Name: "Toolbox", Description: "cassetta"}
	if err := f.repos.Containers.Create(ctx, &container); err != nil {
		t.Fatalf("creating container: %v", err)
	}

	// A non-empty room cannot go, and nothing is recorded for the attempt.
	if err := f.repos.Rooms.Delete(ctx, garage.ID); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("deleting a non-empty room = %v, want ErrConflict", err)
	}
	history := f.history(t, ctx, 10)
	if got := types(history); len(got) != 2 {
		t.Fatalf("event types = %v, want the room and the container only", got)
	}

	container.Name = "Cassetta attrezzi"
	if err := f.repos.Containers.Update(ctx, &container); err != nil {
		t.Fatalf("updating container: %v", err)
	}
	item := inventory.Item{Name: "Trapano", Quantity: 1, Location: inventory.ContainerLocation(container.ID)}
	if err := f.repos.Items.Create(ctx, &item); err != nil {
		t.Fatalf("creating item: %v", err)
	}
	if err := f.repos.Items.Delete(ctx, item.ID); err != nil {
		t.Fatalf("deleting item: %v", err)
	}
	if err := f.repos.Containers.Delete(ctx, container.ID); err != nil {
		t.Fatalf("deleting container: %v", err)
	}
	if err := f.repos.Rooms.Delete(ctx, garage.ID); err != nil {
		t.Fatalf("deleting room: %v", err)
	}

	history = f.history(t, ctx, 10)
	if got := types(history); !slices.Equal(got, []string{
		events.RoomDeleted, events.ContainerDeleted, events.ItemDeleted,
		events.ItemCreated, events.ContainerUpdated, events.ContainerCreated, events.RoomCreated,
	}) {
		t.Fatalf("event types = %v, want one per mutation", got)
	}

	// The room the container lived in is in its story, oldest first.
	story, err := f.store.ForEntity(ctx, events.EntityContainer, int64(container.ID))
	if err != nil {
		t.Fatalf("reading the container's story: %v", err)
	}
	if got := types(story); !slices.Equal(got, []string{events.ContainerCreated, events.ContainerUpdated, events.ContainerDeleted}) {
		t.Fatalf("container story = %v, want the three events", got)
	}
}

// TestEventStoreRefusesAnUnreadableEvent checks the store does not hand back a
// row it cannot parse: a broken payload is a bug, not an event.
func TestEventStoreKeepsTimeAndIDs(t *testing.T) {
	f, ctx := newEventFixture(t)
	room := inventory.Room{Name: "Garage"}
	if err := f.repos.Rooms.Create(ctx, &room); err != nil {
		t.Fatalf("creating room: %v", err)
	}

	history := f.history(t, ctx, 10)
	if len(history) != 1 {
		t.Fatalf("history = %v, want one event", history)
	}
	if history[0].ID == 0 {
		t.Fatal("the event came back without an id")
	}
	if history[0].At.IsZero() {
		t.Fatal("the event came back without a timestamp")
	}
}

// locationPayload is a place as it appears in a move event.
type locationPayload struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}
