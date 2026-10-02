package tools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/nicolasalberti00/homey/internal/events"
	"github.com/nicolasalberti00/homey/internal/inventory"
)

// placeDelete runs a place deletion tool as the caller in ctx and returns the
// raw answer, so a test can read both the confirmation and the view the answer
// carries.
func (f fixture) placeDelete(t *testing.T, ctx context.Context, name, input string) (json.RawMessage, error) {
	t.Helper()
	tool, found := f.registry.Lookup(name)
	if !found {
		t.Fatalf("%s is not registered", name)
	}
	return tool.Call(ctx, json.RawMessage(input))
}

// decodeAnswer reads a raw tool answer into the type of that tool's output.
func decodeAnswer[Out any](t *testing.T, raw json.RawMessage) Out {
	t.Helper()
	var out Out
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	return out
}

func TestAddRoomAnswersWithTheNewRoom(t *testing.T) {
	f := newFixture(t)

	var added RoomView
	f.decode(t, "add_room", `{"name":"Terrazzo","description":"da asciugare"}`, &added)
	if added.ID == 0 {
		t.Fatal("the new room came back without an id")
	}
	if added.Name != "Terrazzo" || added.Description != "da asciugare" {
		t.Fatalf("added = %+v, want the room as it was asked for", added)
	}

	// list_location is how a caller sees the rooms of the home.
	var home ListLocationOutput
	f.decode(t, "list_location", `{}`, &home)
	if !slices.ContainsFunc(home.Rooms, func(room RoomView) bool { return room.Name == "Terrazzo" }) {
		t.Fatalf("rooms = %+v, want the new room among them", home.Rooms)
	}

	// The audit trail knows who created it and with which tool.
	story := f.story(t, t.Context(), events.EntityRoom, int64(added.ID))
	if len(story) == 0 {
		t.Fatal("creating a room left nothing on the audit trail")
	}
	last := story[len(story)-1]
	if last.Type != events.RoomCreated || last.Tool != "add_room" || last.Actor != "test" {
		t.Fatalf("event = %+v, want room.created through add_room by the caller", last)
	}
}

func TestAddRoomRefusesWhatItCannotStore(t *testing.T) {
	f := newFixture(t)

	// A room name fits one room only: "garage" is taken, case aside.
	err := f.fail(t, "add_room", `{"name":"garage"}`)
	if !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate room = %v, want ErrConflict", err)
	}
	if !CallerError(err) {
		t.Fatal("a duplicate room is not reported as something the caller can act on")
	}
	// An empty name is refused by the domain rules, not silently stored.
	if err := f.fail(t, "add_room", `{"name":"  "}`); !errors.Is(err, inventory.ErrValidation) {
		t.Fatalf("an empty name = %v, want ErrValidation", err)
	}
}

func TestAddContainerGoesWhereItIsNamed(t *testing.T) {
	f := newFixture(t)

	// Directly in a room, named the way a person names the place.
	var direct AddContainerOutput
	f.decode(t, "add_container", `{"name":"Cestino","location":"Garage","description":"per la carta"}`, &direct)
	if direct.Container == nil || direct.Container.ID == 0 || direct.Container.Path != "Garage > Cestino" {
		t.Fatalf("added = %+v, want a new container in the garage", direct.Container)
	}

	// Inside another container, with the whole path above it.
	var nested AddContainerOutput
	f.decode(t, "add_container", `{"name":"Trapani","location":"Garage > Toolbox"}`, &nested)
	if nested.Container == nil || nested.Container.Path != "Garage > Toolbox > Trapani" {
		t.Fatalf("added = %+v, want the container inside the toolbox", nested.Container)
	}

	// Both are where list_location says they are, with the description stored.
	var garage ListLocationOutput
	f.decode(t, "list_location", `{"location":"Garage"}`, &garage)
	if !slices.ContainsFunc(garage.Containers, func(container LocationView) bool {
		return container.Name == "Cestino" && container.Description == "per la carta"
	}) {
		t.Fatalf("containers = %+v, want the bin with its description", garage.Containers)
	}

	// The audit trail knows who created it and with which tool.
	story := f.story(t, t.Context(), events.EntityContainer, int64(nested.Container.ID))
	if len(story) == 0 {
		t.Fatal("creating a container left nothing on the audit trail")
	}
	last := story[len(story)-1]
	if last.Type != events.ContainerCreated || last.Tool != "add_container" || last.Actor != "test" {
		t.Fatalf("event = %+v, want container.created through add_container by the caller", last)
	}
}

func TestAddContainerRefusesWhatItCannotPlace(t *testing.T) {
	f := newFixture(t)

	if err := f.fail(t, "add_container", `{"name":"Rastrelli","location":"Giardino"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown place = %v, want ErrNotFound", err)
	}
	// Two toolboxes: which one? The tool asks instead of picking one, and
	// nothing is created until the question is answered.
	var ambiguous AddContainerOutput
	f.decode(t, "add_container", `{"name":"Cacciaviti","location":"Toolbox"}`, &ambiguous)
	if ambiguous.Container != nil {
		t.Fatalf("container = %+v, want nothing created", ambiguous.Container)
	}
	if ambiguous.Clarification == nil {
		t.Fatal("a shared place name came back without a clarification")
	}
	if !slices.Equal(ambiguous.Clarification.Candidates, []string{"Garage > Toolbox", "Cucina > Toolbox"}) {
		t.Fatalf("candidates = %v, want both toolboxes", ambiguous.Clarification.Candidates)
	}
	// A container name is unique in its room, however deep it is stored.
	if err := f.fail(t, "add_container", `{"name":"Cassetto 1","location":"Garage"}`); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate name in the room = %v, want ErrConflict", err)
	}
}

func TestUpdateRoomChangesOnlyWhatItIsGiven(t *testing.T) {
	f := newFixture(t)
	var created RoomView
	f.decode(t, "add_room", `{"name":"Veranda","description":"piante"}`, &created)

	// Rename only: the description stays as it was.
	var renamed RoomView
	f.decode(t, "update_room", `{"id":`+itoa(int64(created.ID))+`,"name":"Limonaia"}`, &renamed)
	if renamed.Name != "Limonaia" {
		t.Fatalf("name = %q, want the new one", renamed.Name)
	}
	if renamed.Description != "piante" {
		t.Fatalf("description = %q, want the one that was not given", renamed.Description)
	}

	// What is stored is what the answer says.
	var stored ListLocationOutput
	f.decode(t, "list_location", `{}`, &stored)
	if !slices.ContainsFunc(stored.Rooms, func(room RoomView) bool {
		return room.Name == "Limonaia" && room.Description == "piante"
	}) {
		t.Fatalf("rooms = %+v, want the rename with the old description", stored.Rooms)
	}

	// The change is on the audit trail, through this tool.
	story := f.story(t, t.Context(), events.EntityRoom, int64(created.ID))
	last := story[len(story)-1]
	if last.Type != events.RoomUpdated || last.Tool != "update_room" || last.Actor != "test" {
		t.Fatalf("event = %+v, want room.updated through update_room by the caller", last)
	}
}

func TestUpdateRoomRefusesWhatItCannotChange(t *testing.T) {
	f := newFixture(t)
	kitchen := f.rooms["kitchen"]

	if err := f.fail(t, "update_room", `{"id":4242,"name":"Soffitta"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown room = %v, want ErrNotFound", err)
	}
	// The name is the identity of a room: taking another room's is refused.
	if err := f.fail(t, "update_room", `{"id":`+itoa(int64(kitchen))+`,"name":"Garage"}`); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate name = %v, want ErrConflict", err)
	}
	if err := f.fail(t, "update_room", `{"id":`+itoa(int64(kitchen))+`,"name":"  "}`); !errors.Is(err, inventory.ErrValidation) {
		t.Fatalf("an empty name = %v, want ErrValidation", err)
	}
	// And the kitchen is exactly as it was.
	var stored ListLocationOutput
	f.decode(t, "list_location", `{}`, &stored)
	if !slices.ContainsFunc(stored.Rooms, func(room RoomView) bool {
		return room.ID == kitchen && room.Name == "Cucina"
	}) {
		t.Fatalf("rooms = %+v, want the kitchen untouched", stored.Rooms)
	}
}

func TestUpdateContainerChangesOnlyWhatItIsGiven(t *testing.T) {
	f := newFixture(t)
	drawer := f.containers["drawer"]

	// A description only: the name and the place stay put.
	var described LocationView
	f.decode(t, "update_container", `{"id":`+itoa(int64(drawer))+`,"description":"per i bit"}`, &described)
	if described.Name != "Cassetto 1" || described.Path != "Garage > Toolbox > Cassetto 1" {
		t.Fatalf("updated = %+v, want the name and the place untouched", described)
	}
	if described.Description != "per i bit" {
		t.Fatalf("description = %q, want the new one", described.Description)
	}

	// Renaming a container rewrites every path that runs through it.
	toolbox := f.containers["toolbox"]
	var renamed LocationView
	f.decode(t, "update_container", `{"id":`+itoa(int64(toolbox))+`,"name":"Scatolone"}`, &renamed)
	if renamed.Path != "Garage > Scatolone" {
		t.Fatalf("path = %q, want the new name", renamed.Path)
	}
	var drawerView ListLocationOutput
	f.decode(t, "list_location", `{"location":"Garage > Scatolone"}`, &drawerView)
	if drawerView.Location != "Garage > Scatolone" ||
		!slices.ContainsFunc(drawerView.Containers, func(container LocationView) bool {
			return container.Name == "Cassetto 1" && container.Path == "Garage > Scatolone > Cassetto 1"
		}) {
		t.Fatalf("listing = %+v, want the drawer under the new path", drawerView)
	}

	// The change is on the audit trail, through this tool.
	story := f.story(t, t.Context(), events.EntityContainer, int64(toolbox))
	last := story[len(story)-1]
	if last.Type != events.ContainerUpdated || last.Tool != "update_container" || last.Actor != "test" {
		t.Fatalf("event = %+v, want container.updated through update_container by the caller", last)
	}
}

func TestUpdateContainerRefusesWhatItCannotChange(t *testing.T) {
	f := newFixture(t)
	drawer := f.containers["drawer"]

	if err := f.fail(t, "update_container", `{"id":4242,"name":"Cassetto 2"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown container = %v, want ErrNotFound", err)
	}
	// The toolbox already carries that name in this room.
	if err := f.fail(t, "update_container", `{"id":`+itoa(int64(drawer))+`,"name":"Toolbox"}`); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate name in the room = %v, want ErrConflict", err)
	}
	if err := f.fail(t, "update_container", `{"id":`+itoa(int64(drawer))+`,"name":"  "}`); !errors.Is(err, inventory.ErrValidation) {
		t.Fatalf("an empty name = %v, want ErrValidation", err)
	}
	// The name in the other room is a different container's, not a clash.
	kitchenToolbox := f.containers["kitchen toolbox"]
	f.call(t, "update_container", `{"id":`+itoa(int64(kitchenToolbox))+`,"name":"Cassetto 1"}`)
	var kitchenList ListLocationOutput
	f.decode(t, "list_location", `{"location":"Cucina"}`, &kitchenList)
	if !slices.ContainsFunc(kitchenList.Containers, func(container LocationView) bool {
		return container.ID == kitchenToolbox && container.Name == "Cassetto 1"
	}) {
		t.Fatalf("containers = %+v, want the renamed container", kitchenList.Containers)
	}
}

// TestMoveContainerCarriesItsSubtree is the question move_container exists
// for: the drawer moves to the kitchen with the container inside it, the
// drill that sits at the bottom, and every path that renders them.
func TestMoveContainerCarriesItsSubtree(t *testing.T) {
	f := newFixture(t)
	drawer := f.containers["drawer"]

	// Give the drawer something of its own to carry.
	var child AddContainerOutput
	f.decode(t, "add_container", `{"name":"Cacciaviti","location":"Garage > Toolbox > Cassetto 1"}`, &child)
	if child.Container == nil {
		t.Fatal("the container inside the drawer was not created")
	}

	var moved MoveContainerOutput
	f.decode(t, "move_container", `{"id":`+itoa(int64(drawer))+`,"destination":"Cucina"}`, &moved)
	if moved.Container == nil || moved.Container.Path != "Cucina > Cassetto 1" {
		t.Fatalf("moved = %+v, want the drawer in the kitchen", moved.Container)
	}

	// The container inside it came along, and so did the drill stored there.
	var drawerView ListLocationOutput
	f.decode(t, "list_location", `{"location":"Cucina > Cassetto 1"}`, &drawerView)
	if !slices.ContainsFunc(drawerView.Containers, func(container LocationView) bool {
		return container.Name == "Cacciaviti" && container.Path == "Cucina > Cassetto 1 > Cacciaviti"
	}) {
		t.Fatalf("containers = %+v, want the child under the moved drawer", drawerView.Containers)
	}
	var found SearchInventoryOutput
	f.decode(t, "search_inventory", `{"query":"bosch"}`, &found)
	if found.Count != 1 || found.Results[0].Location != "Cucina > Cassetto 1" {
		t.Fatalf("search = %+v, want the drill rendered where the drawer now sits", found.Results)
	}
	// The garage keeps the toolbox, which is now empty.
	var toolboxView ListLocationOutput
	f.decode(t, "list_location", `{"location":"Garage > Toolbox"}`, &toolboxView)
	if len(toolboxView.Containers) != 0 {
		t.Fatalf("containers = %+v, want the drawer gone from the toolbox", toolboxView.Containers)
	}

	// The move is on the audit trail, through this tool.
	story := f.story(t, t.Context(), events.EntityContainer, int64(drawer))
	last := story[len(story)-1]
	if last.Type != events.ContainerMoved || last.Tool != "move_container" || last.Actor != "test" {
		t.Fatalf("event = %+v, want container.moved through move_container by the caller", last)
	}
}

func TestMoveContainerRefusesWhatItCannotDo(t *testing.T) {
	f := newFixture(t)
	toolbox := f.containers["toolbox"]
	drawer := f.containers["drawer"]
	kitchenToolbox := f.containers["kitchen toolbox"]

	if err := f.fail(t, "move_container", `{"id":4242,"destination":"Cucina"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown container = %v, want ErrNotFound", err)
	}
	if err := f.fail(t, "move_container", `{"id":`+itoa(int64(toolbox))+`,"destination":"Giardino"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown destination = %v, want ErrNotFound", err)
	}

	// A shared destination name is a question, not a guess: nothing moves.
	var ambiguous MoveContainerOutput
	f.decode(t, "move_container", `{"id":`+itoa(int64(drawer))+`,"destination":"Toolbox"}`, &ambiguous)
	if ambiguous.Container != nil {
		t.Fatalf("container = %+v, want nothing moved", ambiguous.Container)
	}
	if ambiguous.Clarification == nil {
		t.Fatal("a shared destination came back without a clarification")
	}
	var garage ListLocationOutput
	f.decode(t, "list_location", `{"location":"Garage"}`, &garage)
	if !slices.ContainsFunc(garage.Containers, func(container LocationView) bool {
		return container.ID == toolbox
	}) {
		t.Fatalf("containers = %+v, want the toolbox still in the garage", garage.Containers)
	}

	// Two containers in one room cannot share a name: the kitchen toolbox
	// would become the garage's twin.
	if err := f.fail(t, "move_container", `{"id":`+itoa(int64(kitchenToolbox))+`,"destination":"Garage"}`); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate name in the destination = %v, want ErrConflict", err)
	}

	// A container cannot go inside itself or its own subtree, and that
	// refusal is something the caller can act on rather than a failure of
	// the server.
	err := f.fail(t, "move_container", `{"id":`+itoa(int64(toolbox))+`,"destination":"Garage > Toolbox > Cassetto 1"}`)
	if !errors.Is(err, inventory.ErrCycle) {
		t.Fatalf("a move under itself = %v, want ErrCycle", err)
	}
	if !CallerError(err) {
		t.Fatal("a cycle is not reported as something the caller can act on")
	}
}

func TestDeleteRoomAsksBeforeDeleting(t *testing.T) {
	f := newFixture(t)
	var room RoomView
	f.decode(t, "add_room", `{"name":"Ripostiglio"}`, &room)

	raw, err := f.placeDelete(t, writerContext(t), "delete_room", `{"id":`+itoa(int64(room.ID))+`}`)
	if err != nil {
		t.Fatalf("delete_room: %v", err)
	}
	proposal := decodeAnswer[DeleteRoomOutput](t, raw)
	if proposal.Deleted {
		t.Fatal("the room was deleted before anyone confirmed")
	}
	if proposal.Confirmation != "pending" || proposal.ConfirmationToken == "" {
		t.Fatalf("proposal = %+v, want a pending confirmation with a token", proposal)
	}
	if proposal.ExpiresInSeconds <= 0 {
		t.Fatalf("expires_in_seconds = %d, want a positive lifetime", proposal.ExpiresInSeconds)
	}
	if proposal.Room.Name != "Ripostiglio" {
		t.Fatalf("proposal room = %+v, want the one that would go", proposal.Room)
	}

	// Nothing was removed.
	var home ListLocationOutput
	f.decode(t, "list_location", `{}`, &home)
	if !slices.ContainsFunc(home.Rooms, func(room RoomView) bool { return room.Name == "Ripostiglio" }) {
		t.Fatalf("rooms = %+v, want the room still there", home.Rooms)
	}
	// A proposal is not a deletion: it leaves no event.
	story := f.story(t, t.Context(), events.EntityRoom, int64(room.ID))
	if len(story) != 1 {
		t.Fatalf("events = %v, want only the creation", eventTypes(story))
	}
}

func TestDeleteRoomCarriesOutAConfirmedDeletion(t *testing.T) {
	f := newFixture(t)
	var room RoomView
	f.decode(t, "add_room", `{"name":"Ripostiglio"}`, &room)
	id := itoa(int64(room.ID))

	raw, err := f.placeDelete(t, writerContext(t), "delete_room", `{"id":`+id+`}`)
	if err != nil {
		t.Fatalf("delete_room: %v", err)
	}
	proposal := decodeAnswer[DeleteRoomOutput](t, raw)
	raw, err = f.placeDelete(t, writerContext(t), "delete_room",
		`{"id":`+id+`,"confirmation_token":`+quote(proposal.ConfirmationToken)+`}`)
	if err != nil {
		t.Fatalf("delete_room with the token: %v", err)
	}
	done := decodeAnswer[DeleteRoomOutput](t, raw)
	if !done.Deleted || done.Confirmation != "confirmed" {
		t.Fatalf("confirmed deletion = %+v, want it carried out", done)
	}
	if done.ConfirmationToken != "" {
		t.Fatal("a completed deletion handed back a token")
	}

	// The room is gone from storage, not just hidden.
	var home ListLocationOutput
	f.decode(t, "list_location", `{}`, &home)
	if slices.ContainsFunc(home.Rooms, func(room RoomView) bool { return room.Name == "Ripostiglio" }) {
		t.Fatalf("rooms = %+v, want the room deleted", home.Rooms)
	}
	if err := f.fail(t, "update_room", `{"id":`+id+`,"name":"Torna"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("updating the deleted room = %v, want ErrNotFound", err)
	}

	// And the deletion is on the audit trail, authorised by confirmation.
	story := f.story(t, t.Context(), events.EntityRoom, int64(room.ID))
	last := story[len(story)-1]
	if last.Type != events.RoomDeleted || last.Tool != "delete_room" ||
		last.Confirmation != "confirmed" || last.Actor != "test" {
		t.Fatalf("event = %+v, want a confirmed delete through delete_room", last)
	}
}

func TestDeleteRoomBypassesWithConfirm(t *testing.T) {
	f := newFixture(t)
	var room RoomView
	f.decode(t, "add_room", `{"name":"Ripostiglio"}`, &room)

	raw, err := f.placeDelete(t, writerContext(t), "delete_room",
		`{"id":`+itoa(int64(room.ID))+`,"confirm":true}`)
	if err != nil {
		t.Fatalf("delete_room: %v", err)
	}
	done := decodeAnswer[DeleteRoomOutput](t, raw)
	if !done.Deleted || done.Confirmation != "bypassed" {
		t.Fatalf("confirm=true got %+v, want a bypassed deletion", done)
	}
	if done.ConfirmationToken != "" {
		t.Fatal("a bypass handed back a token")
	}

	// The bypass is on the audit trail, written as it was called.
	story := f.story(t, t.Context(), events.EntityRoom, int64(room.ID))
	last := story[len(story)-1]
	if last.Type != events.RoomDeleted || last.Confirmation != "bypassed" {
		t.Fatalf("event = %+v, want a bypassed delete", last)
	}
}

func TestDeleteRoomRefusesWhatItCannotDelete(t *testing.T) {
	f := newFixture(t)

	if _, err := f.placeDelete(t, writerContext(t), "delete_room", `{"id":4242}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("delete_room on a missing room = %v, want ErrNotFound", err)
	}

	// A token nobody issued.
	var room RoomView
	f.decode(t, "add_room", `{"name":"Ripostiglio"}`, &room)
	roomID := itoa(int64(room.ID))
	raw, err := f.placeDelete(t, writerContext(t), "delete_room", `{"id":`+roomID+`}`)
	if err != nil {
		t.Fatalf("delete_room: %v", err)
	}
	proposal := decodeAnswer[DeleteRoomOutput](t, raw)
	if _, err := f.placeDelete(t, writerContext(t), "delete_room",
		`{"id":`+roomID+`,"confirmation_token":"made-up"}`); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("an invented token = %v, want ErrConfirmation", err)
	}
	// A token issued for another room.
	var other RoomView
	f.decode(t, "add_room", `{"name":"Cantina"}`, &other)
	if _, err := f.placeDelete(t, writerContext(t), "delete_room",
		`{"id":`+itoa(int64(other.ID))+`,"confirmation_token":`+quote(proposal.ConfirmationToken)+`}`); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("a token for another room = %v, want ErrConfirmation", err)
	}
	// A token issued by another tool: delete_room does not confirm a
	// container, and the same holds the other way round.
	raw, err = f.placeDelete(t, writerContext(t), "delete_container", `{"id":`+itoa(int64(f.containers["kitchen toolbox"]))+`}`)
	if err != nil {
		t.Fatalf("delete_container: %v", err)
	}
	containerProposal := decodeAnswer[DeleteContainerOutput](t, raw)
	if _, err := f.placeDelete(t, writerContext(t), "delete_room",
		`{"id":`+roomID+`,"confirmation_token":`+quote(containerProposal.ConfirmationToken)+`}`); !errors.Is(err, ErrConfirmation) {
		t.Fatalf("a token from another tool = %v, want ErrConfirmation", err)
	}

	// A room that still holds something is refused by the core, so nothing
	// is ever destroyed by mistake.
	garage := f.rooms["garage"]
	garageID := itoa(int64(garage))
	raw, err = f.placeDelete(t, writerContext(t), "delete_room", `{"id":`+garageID+`}`)
	if err != nil {
		t.Fatalf("proposing the garage = %v, want a proposal", err)
	}
	garageProposal := decodeAnswer[DeleteRoomOutput](t, raw)
	if _, err := f.placeDelete(t, writerContext(t), "delete_room",
		`{"id":`+garageID+`,"confirmation_token":`+quote(garageProposal.ConfirmationToken)+`}`); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("deleting a room that holds things = %v, want ErrConflict", err)
	}
	var home ListLocationOutput
	f.decode(t, "list_location", `{}`, &home)
	if !slices.ContainsFunc(home.Rooms, func(room RoomView) bool { return room.ID == garage }) {
		t.Fatalf("rooms = %+v, want the garage still standing", home.Rooms)
	}
	// Refused confirmations are not deletions, so they leave no event.
	story := f.story(t, t.Context(), events.EntityRoom, int64(garage))
	if slices.Contains(eventTypes(story), events.RoomDeleted) {
		t.Fatalf("event types = %v, want no deletion recorded", eventTypes(story))
	}
}

func TestDeleteContainerGoesThroughTheTwoSteps(t *testing.T) {
	f := newFixture(t)
	kitchenToolbox := f.containers["kitchen toolbox"]
	id := itoa(int64(kitchenToolbox))

	raw, err := f.placeDelete(t, writerContext(t), "delete_container", `{"id":`+id+`}`)
	if err != nil {
		t.Fatalf("delete_container: %v", err)
	}
	proposal := decodeAnswer[DeleteContainerOutput](t, raw)
	if proposal.Deleted || proposal.Confirmation != "pending" || proposal.ConfirmationToken == "" {
		t.Fatalf("proposal = %+v, want a pending confirmation with a token", proposal)
	}
	// The proposal says what would go, so a caller can show it and ask.
	if proposal.Container.ID != kitchenToolbox || proposal.Container.Path != "Cucina > Toolbox" {
		t.Fatalf("proposal container = %+v, want the toolbox with its path", proposal.Container)
	}
	var listed ListLocationOutput
	f.decode(t, "list_location", `{"location":"Cucina"}`, &listed)
	if !slices.ContainsFunc(listed.Containers, func(container LocationView) bool { return container.ID == kitchenToolbox }) {
		t.Fatalf("containers = %+v, want the container still there", listed.Containers)
	}

	// The token carries the deletion out.
	raw, err = f.placeDelete(t, writerContext(t), "delete_container",
		`{"id":`+id+`,"confirmation_token":`+quote(proposal.ConfirmationToken)+`}`)
	if err != nil {
		t.Fatalf("delete_container with the token: %v", err)
	}
	done := decodeAnswer[DeleteContainerOutput](t, raw)
	if !done.Deleted || done.Confirmation != "confirmed" {
		t.Fatalf("confirmed deletion = %+v, want it carried out", done)
	}
	var after ListLocationOutput
	f.decode(t, "list_location", `{"location":"Cucina"}`, &after)
	if len(after.Containers) != 0 {
		t.Fatalf("containers = %+v, want the toolbox deleted", after.Containers)
	}
	story := f.story(t, t.Context(), events.EntityContainer, int64(kitchenToolbox))
	last := story[len(story)-1]
	if last.Type != events.ContainerDeleted || last.Tool != "delete_container" ||
		last.Confirmation != "confirmed" || last.Actor != "test" {
		t.Fatalf("event = %+v, want a confirmed delete through delete_container", last)
	}
}

func TestDeleteContainerRefusesWhatItCannotDelete(t *testing.T) {
	f := newFixture(t)

	if _, err := f.placeDelete(t, writerContext(t), "delete_container", `{"id":4242}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("delete_container on a missing container = %v, want ErrNotFound", err)
	}

	// The toolbox holds the drawer: the second call is refused and the
	// toolbox stays.
	toolbox := f.containers["toolbox"]
	toolboxID := itoa(int64(toolbox))
	raw, err := f.placeDelete(t, writerContext(t), "delete_container", `{"id":`+toolboxID+`}`)
	if err != nil {
		t.Fatalf("proposing the toolbox = %v, want a proposal", err)
	}
	proposal := decodeAnswer[DeleteContainerOutput](t, raw)
	if _, err := f.placeDelete(t, writerContext(t), "delete_container",
		`{"id":`+toolboxID+`,"confirmation_token":`+quote(proposal.ConfirmationToken)+`}`); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("deleting a container that holds things = %v, want ErrConflict", err)
	}
	var garage ListLocationOutput
	f.decode(t, "list_location", `{"location":"Garage"}`, &garage)
	if !slices.ContainsFunc(garage.Containers, func(container LocationView) bool { return container.ID == toolbox }) {
		t.Fatalf("containers = %+v, want the toolbox still standing", garage.Containers)
	}
	story := f.story(t, t.Context(), events.EntityContainer, int64(toolbox))
	if slices.Contains(eventTypes(story), events.ContainerDeleted) {
		t.Fatalf("event types = %v, want no deletion recorded", eventTypes(story))
	}

	// A trusted caller deletes without the extra step, and the bypass is
	// what the audit trail records.
	kitchenToolbox := f.containers["kitchen toolbox"]
	ctx := WithCaller(t.Context(), Caller{Name: "automation", CanWrite: true, BypassConfirmation: true})
	raw, err = f.placeDelete(t, ctx, "delete_container", `{"id":`+itoa(int64(kitchenToolbox))+`}`)
	if err != nil {
		t.Fatalf("delete_container: %v", err)
	}
	done := decodeAnswer[DeleteContainerOutput](t, raw)
	if !done.Deleted || done.Confirmation != "bypassed" {
		t.Fatalf("trusted caller got %+v, want a bypassed deletion", done)
	}
	story = f.story(t, t.Context(), events.EntityContainer, int64(kitchenToolbox))
	last := story[len(story)-1]
	if last.Type != events.ContainerDeleted || last.Confirmation != "bypassed" || last.Actor != "automation" {
		t.Fatalf("event = %+v, want the trusted caller recorded as bypassed", last)
	}
}

// TestPlaceToolsAreWrites checks the annotations a host uses to decide
// whether a tool needs care: adding a place is additive, changing and moving
// one is not, and the deletions ask before they act.
func TestPlaceToolsAreWrites(t *testing.T) {
	f := newFixture(t)

	cases := []struct {
		name                 string
		destructive          bool
		requiresConfirmation bool
	}{
		{"add_room", false, false},
		{"update_room", true, false},
		{"delete_room", true, true},
		{"add_container", false, false},
		{"update_container", true, false},
		{"move_container", true, false},
		{"delete_container", true, true},
	}
	for _, tc := range cases {
		tool, found := f.registry.Lookup(tc.name)
		if !found {
			t.Fatalf("%s is not registered", tc.name)
		}
		if tool.Permission() != PermissionWrite {
			t.Fatalf("%s needs %s, want write", tc.name, tool.Permission())
		}
		if tool.Destructive() != tc.destructive {
			t.Fatalf("%s is destructive=%t, want %t", tc.name, tool.Destructive(), tc.destructive)
		}
		if tool.RequiresConfirmation() != tc.requiresConfirmation {
			t.Fatalf("%s asks for confirmation=%t, want %t", tc.name, tool.RequiresConfirmation(), tc.requiresConfirmation)
		}
	}
}
