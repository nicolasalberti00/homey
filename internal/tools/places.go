package tools

import (
	"context"

	"github.com/nicolasalberti00/homey/internal/events"
	"github.com/nicolasalberti00/homey/internal/inventory"
)

// This file holds the tools over the places themselves: the rooms and the
// containers, the same shape as the item tools — a definition in placeTools
// and a method here that calls the core. Places are named the way a person
// names them, and every rule that decides what may be stored, renamed, moved
// or removed stays in the core: a duplicate name, a room that still holds
// something, a container moving into its own subtree all come back from there
// as the errors the caller can act on.

const (
	// deleteRoomTool and deleteContainerTool are the names their confirmation
	// tokens are bound to, so a token confirms one deletion of one entity.
	deleteRoomTool      = "delete_room"
	deleteContainerTool = "delete_container"
)

// AddRoomInput is what add_room takes.
type AddRoomInput struct {
	Name        string `json:"name" jsonschema:"the name of the room, as the person calls it: \"Garage\", \"Cucina\""`
	Description string `json:"description,omitempty" jsonschema:"what the room is for, in a few words"`
}

// UpdateRoomInput is what update_room takes: only the fields to change, so a
// caller that does not mention a field leaves it as it is.
type UpdateRoomInput struct {
	ID          inventory.RoomID `json:"id" jsonschema:"the id of the room to change, as returned by list_location"`
	Name        *string          `json:"name,omitempty" jsonschema:"the new name"`
	Description *string          `json:"description,omitempty" jsonschema:"the new description"`
}

// DeleteRoomInput is what delete_room takes. By default nothing is removed:
// the call proposes the deletion and answers with a token, and the deletion
// happens when the token comes back.
type DeleteRoomInput struct {
	ID                inventory.RoomID `json:"id" jsonschema:"the id of the room to delete, as returned by list_location"`
	Confirm           bool             `json:"confirm,omitempty" jsonschema:"set true only when the person has already confirmed this exact deletion, to carry it out now without the token step"`
	ConfirmationToken string           `json:"confirmation_token,omitempty" jsonschema:"the token from the earlier proposal; pass it back to carry the deletion out"`
}

// DeleteRoomOutput is what delete_room answers: either the deletion happened
// (with how it was authorised) or it is proposed, carrying the token that
// confirms it.
type DeleteRoomOutput struct {
	Deleted           bool     `json:"deleted" jsonschema:"true when the room was removed; false when the call proposed a deletion instead"`
	Room              RoomView `json:"room" jsonschema:"the room that was removed, or that confirming would remove"`
	Confirmation      string   `json:"confirmation" jsonschema:"how the deletion stands: pending, confirmed or bypassed"`
	ConfirmationToken string   `json:"confirmation_token,omitempty" jsonschema:"the token to pass back to carry the deletion out; present only while the deletion is pending"`
	ExpiresInSeconds  int      `json:"expires_in_seconds,omitempty" jsonschema:"how long the confirmation token stays valid"`
}

// AddContainerInput is what add_container takes.
type AddContainerInput struct {
	Name        string `json:"name" jsonschema:"the name of the container, as the person calls it: \"Toolbox\", \"Cassetto dei bit\""`
	Location    string `json:"location" jsonschema:"where it goes: a room (\"Garage\") or a container path (\"Garage > Toolbox\")"`
	Description string `json:"description,omitempty" jsonschema:"what the container is for, in a few words"`
}

// UpdateContainerInput is what update_container takes: only the fields to
// change, so a caller that does not mention a field leaves it as it is.
// Where the container sits is move_container's answer, not this one's.
type UpdateContainerInput struct {
	ID          inventory.ContainerID `json:"id" jsonschema:"the id of the container to change, as returned by list_location"`
	Name        *string               `json:"name,omitempty" jsonschema:"the new name"`
	Description *string               `json:"description,omitempty" jsonschema:"the new description"`
}

// MoveContainerInput is what move_container takes.
type MoveContainerInput struct {
	ID          inventory.ContainerID `json:"id" jsonschema:"the id of the container to move, as returned by list_location"`
	Destination string                `json:"destination" jsonschema:"where it goes: a room (\"Cucina\") or a container path (\"Garage > Toolbox\")"`
}

// DeleteContainerInput is what delete_container takes, by default as a
// proposal that carries nothing away until its token comes back.
type DeleteContainerInput struct {
	ID                inventory.ContainerID `json:"id" jsonschema:"the id of the container to delete, as returned by list_location"`
	Confirm           bool                  `json:"confirm,omitempty" jsonschema:"set true only when the person has already confirmed this exact deletion, to carry it out now without the token step"`
	ConfirmationToken string                `json:"confirmation_token,omitempty" jsonschema:"the token from the earlier proposal; pass it back to carry the deletion out"`
}

// AddContainerOutput is what add_container answers: the new container, or a
// clarification when the place named several and nothing was created.
type AddContainerOutput struct {
	Container     *LocationView  `json:"container,omitempty" jsonschema:"the container as stored, id and path included; absent when a clarification is asked for"`
	Clarification *Clarification `json:"clarification,omitempty" jsonschema:"set when the place was ambiguous: ask which one, then call again with a full path"`
}

// MoveContainerOutput is what move_container answers: the moved container, or
// a clarification when the destination named several and nothing moved.
type MoveContainerOutput struct {
	Container     *LocationView  `json:"container,omitempty" jsonschema:"the container as stored after the move, with its new path; absent when a clarification is asked for"`
	Clarification *Clarification `json:"clarification,omitempty" jsonschema:"set when the destination was ambiguous: ask which one, then call again with a full path"`
}

// DeleteContainerOutput is what delete_container answers: either the deletion
// happened (with how it was authorised) or it is proposed, carrying the token
// that confirms it.
type DeleteContainerOutput struct {
	Deleted           bool         `json:"deleted" jsonschema:"true when the container was removed; false when the call proposed a deletion instead"`
	Container         LocationView `json:"container" jsonschema:"the container that was removed, or that confirming would remove"`
	Confirmation      string       `json:"confirmation" jsonschema:"how the deletion stands: pending, confirmed or bypassed"`
	ConfirmationToken string       `json:"confirmation_token,omitempty" jsonschema:"the token to pass back to carry the deletion out; present only while the deletion is pending"`
	ExpiresInSeconds  int          `json:"expires_in_seconds,omitempty" jsonschema:"how long the confirmation token stays valid"`
}

// placeTools returns the tools over rooms and containers, registered next to
// the item tools.
func (inv Inventory) placeTools() ([]Tool, error) {
	addRoom, err := New(Definition[AddRoomInput, RoomView]{
		Name: "add_room",
		Description: "Add a room to the home: the top level of the inventory, where containers and items live. " +
			"Use it when a new place comes up (\"aggiungi la veranda\") or the home is being set up. " +
			"The answer carries the new id, so containers can be placed in it next. " +
			"If a room with that name already exists, the call fails instead of overwriting it.",
		Permission: PermissionWrite,
		Handler:    inv.AddRoom,
	})
	if err != nil {
		return nil, err
	}

	updateRoom, err := New(Definition[UpdateRoomInput, RoomView]{
		Name: "update_room",
		Description: "Change the name or the description of a room, leaving the other field as it is. " +
			"What the room holds is not touched: containers and items stay where they are.",
		Permission:  PermissionWrite,
		Destructive: true,
		Handler:     inv.UpdateRoom,
	})
	if err != nil {
		return nil, err
	}

	deleteRoom, err := New(Definition[DeleteRoomInput, DeleteRoomOutput]{
		Name: "delete_room",
		Description: "Delete a room from the home for good; there is no undo. " +
			"It asks first: without confirm or confirmation_token it answers with the room and a confirmation_token, " +
			"and nothing is removed; call it again with that token to carry the deletion out. " +
			"Pass confirm=true only when the person has already said yes to this exact deletion. " +
			"A room that still holds containers or items is refused, so what it holds has to go elsewhere first.",
		Permission:           PermissionWrite,
		Destructive:          true,
		RequiresConfirmation: true,
		Handler:              inv.DeleteRoom,
	})
	if err != nil {
		return nil, err
	}

	addContainer, err := New(Definition[AddContainerInput, AddContainerOutput]{
		Name: "add_container",
		Description: "Add a container inside a room or inside another container that already exists: " +
			"a toolbox, a drawer, a shelf. The location is named the way a person names it: \"Garage\" or \"Garage > Toolbox\". " +
			"The answer carries the new id and the path, so items can be put in it next. " +
			"When the name fits several places the answer carries a clarification instead, and nothing is created " +
			"until you ask and call again with the full path; if a container with that name is already in that room, " +
			"the call fails instead of overwriting it.",
		Permission: PermissionWrite,
		Handler:    inv.AddContainer,
	})
	if err != nil {
		return nil, err
	}

	updateContainer, err := New(Definition[UpdateContainerInput, LocationView]{
		Name: "update_container",
		Description: "Change the name or the description of a container, leaving the other field as it is. " +
			"Where the container sits does not change here: use move_container for that. " +
			"The answer carries the path as it stands after the change.",
		Permission:  PermissionWrite,
		Destructive: true,
		Handler:     inv.UpdateContainer,
	})
	if err != nil {
		return nil, err
	}

	moveContainer, err := New(Definition[MoveContainerInput, MoveContainerOutput]{
		Name: "move_container",
		Description: "Move a container and everything inside it — the containers it holds and the items they hold — " +
			"to another room or container, leaving everything else alone. " +
			"The destination is named the way a person names it: \"Cucina\" or \"Garage > Toolbox\". " +
			"Use it for \"sposta la toolbox in cucina\" once the container id is known from list_location. " +
			"When the destination name fits several places the answer carries a clarification and the container does not move: " +
			"ask which one, then call again with the full path.",
		Permission:  PermissionWrite,
		Destructive: true,
		Handler:     inv.MoveContainer,
	})
	if err != nil {
		return nil, err
	}

	deleteContainer, err := New(Definition[DeleteContainerInput, DeleteContainerOutput]{
		Name: "delete_container",
		Description: "Delete a container from the home for good; there is no undo. " +
			"It asks first: without confirm or confirmation_token it answers with the container and a confirmation_token, " +
			"and nothing is removed; call it again with that token to carry the deletion out. " +
			"Pass confirm=true only when the person has already said yes to this exact deletion. " +
			"A container that still holds containers or items is refused, so what it holds has to go elsewhere first.",
		Permission:           PermissionWrite,
		Destructive:          true,
		RequiresConfirmation: true,
		Handler:              inv.DeleteContainer,
	})
	if err != nil {
		return nil, err
	}

	return []Tool{addRoom, updateRoom, deleteRoom, addContainer, updateContainer, moveContainer, deleteContainer}, nil
}

// AddRoom puts a new room in the home and answers with it, id included, so a
// caller can place containers and items in it next.
func (inv Inventory) AddRoom(ctx context.Context, input AddRoomInput) (RoomView, error) {
	room := inventory.Room{Name: input.Name, Description: input.Description}
	if err := inv.Rooms.Create(ctx, &room); err != nil {
		return RoomView{}, err
	}
	return roomView(room), nil
}

// UpdateRoom changes the fields that were given and leaves the others alone.
// What the room holds is not part of this: the core keeps containers and
// items where they are.
func (inv Inventory) UpdateRoom(ctx context.Context, input UpdateRoomInput) (RoomView, error) {
	room, err := inv.Rooms.Get(ctx, input.ID)
	if err != nil {
		return RoomView{}, err
	}
	if input.Name != nil {
		room.Name = *input.Name
	}
	if input.Description != nil {
		room.Description = *input.Description
	}
	if err := inv.Rooms.Update(ctx, &room); err != nil {
		return RoomView{}, err
	}
	return roomView(room), nil
}

// AddContainer puts a new container in a place that already exists: a room,
// or another container, resolved the way a person names it. When the place
// named several it asks which one instead of guessing, and nothing is
// created.
func (inv Inventory) AddContainer(ctx context.Context, input AddContainerInput) (AddContainerOutput, error) {
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return AddContainerOutput{}, err
	}
	place, err := index.Resolve(input.Location)
	if err != nil {
		if clarification, ok := clarify("location", input.Location, err); ok {
			return AddContainerOutput{Clarification: clarification}, nil
		}
		return AddContainerOutput{}, err
	}

	container := inventory.Container{Name: input.Name, Description: input.Description}
	if place.IsRoom() {
		container.RoomID = inventory.RoomID(place.ID)
	} else {
		// The parent carries the room the new container belongs to, at every
		// nesting depth.
		parent, err := inv.Containers.Get(ctx, inventory.ContainerID(place.ID))
		if err != nil {
			return AddContainerOutput{}, err
		}
		container.RoomID = parent.RoomID
		container.ParentID = &parent.ID
	}
	if err := inv.Containers.Create(ctx, &container); err != nil {
		return AddContainerOutput{}, err
	}

	view, err := inv.containerView(ctx, container.ID)
	if err != nil {
		return AddContainerOutput{}, err
	}
	return AddContainerOutput{Container: &view}, nil
}

// UpdateContainer changes the fields that were given and leaves the others
// alone. Where it sits stays put: moving is move_container.
func (inv Inventory) UpdateContainer(ctx context.Context, input UpdateContainerInput) (LocationView, error) {
	container, err := inv.Containers.Get(ctx, input.ID)
	if err != nil {
		return LocationView{}, err
	}
	if input.Name != nil {
		container.Name = *input.Name
	}
	if input.Description != nil {
		container.Description = *input.Description
	}
	if err := inv.Containers.Update(ctx, &container); err != nil {
		return LocationView{}, err
	}
	// The name is part of every path that runs through this container, so
	// the answer is read back from a view built after the change.
	return inv.containerView(ctx, container.ID)
}

// MoveContainer takes a container and its whole subtree to another place
// without touching anything else. The destination is a room or a path such as
// "Garage > Toolbox", resolved the way a person names it. When the
// destination named several places it asks which one instead of guessing.
func (inv Inventory) MoveContainer(ctx context.Context, input MoveContainerInput) (MoveContainerOutput, error) {
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return MoveContainerOutput{}, err
	}
	destination, err := index.Resolve(input.Destination)
	if err != nil {
		if clarification, ok := clarify("destination", input.Destination, err); ok {
			return MoveContainerOutput{Clarification: clarification}, nil
		}
		return MoveContainerOutput{}, err
	}
	if err := inv.Containers.Move(ctx, input.ID, destination); err != nil {
		return MoveContainerOutput{}, err
	}

	// The subtree moved, so the path is rendered from a view built after
	// the move rather than from the index the destination was resolved with.
	view, err := inv.containerView(ctx, input.ID)
	if err != nil {
		return MoveContainerOutput{}, err
	}
	return MoveContainerOutput{Container: &view}, nil
}

// DeleteRoom removes a room for good. It asks first, like every destructive
// tool: a call without confirm or confirmation_token proposes the deletion and
// answers with a token, and the room stays until that token comes back. The
// core decides what may go: a room that still holds containers or items is
// refused when the deletion is carried out, and nothing is destroyed by
// mistake.
func (inv Inventory) DeleteRoom(ctx context.Context, input DeleteRoomInput) (DeleteRoomOutput, error) {
	// Read the room first: the answer says what is going, a missing room is
	// an error the caller can act on, and the proposal carries it.
	room, err := inv.Rooms.Get(ctx, input.ID)
	if err != nil {
		return DeleteRoomOutput{}, err
	}
	view := roomView(room)

	mode, pending, err := authoriseDestructive(ctx, inv.confirmer(),
		deleteRoomTool, "room", int64(input.ID), input.Confirm, input.ConfirmationToken)
	if err != nil {
		return DeleteRoomOutput{}, err
	}
	if pending != nil {
		return DeleteRoomOutput{
			Deleted:           false,
			Room:              view,
			Confirmation:      mode,
			ConfirmationToken: pending.token,
			ExpiresInSeconds:  pending.expires,
		}, nil
	}

	caller, _ := CallerFrom(ctx)
	if err := inv.Rooms.Delete(events.WithActor(ctx, events.Actor{
		Name: caller.Name, Tool: deleteRoomTool, Confirmation: mode,
	}), input.ID); err != nil {
		return DeleteRoomOutput{}, err
	}
	return DeleteRoomOutput{Deleted: true, Room: view, Confirmation: mode}, nil
}

// DeleteContainer removes a container for good, asking first the same way
// delete_room does. The core refuses it while it holds containers or items.
func (inv Inventory) DeleteContainer(ctx context.Context, input DeleteContainerInput) (DeleteContainerOutput, error) {
	view, err := inv.containerView(ctx, input.ID)
	if err != nil {
		return DeleteContainerOutput{}, err
	}

	mode, pending, err := authoriseDestructive(ctx, inv.confirmer(),
		deleteContainerTool, "container", int64(input.ID), input.Confirm, input.ConfirmationToken)
	if err != nil {
		return DeleteContainerOutput{}, err
	}
	if pending != nil {
		return DeleteContainerOutput{
			Deleted:           false,
			Container:         view,
			Confirmation:      mode,
			ConfirmationToken: pending.token,
			ExpiresInSeconds:  pending.expires,
		}, nil
	}

	caller, _ := CallerFrom(ctx)
	if err := inv.Containers.Delete(events.WithActor(ctx, events.Actor{
		Name: caller.Name, Tool: deleteContainerTool, Confirmation: mode,
	}), input.ID); err != nil {
		return DeleteContainerOutput{}, err
	}
	return DeleteContainerOutput{Deleted: true, Container: view, Confirmation: mode}, nil
}

// containerView reads a container back with the path it has now: the caller
// asked after a change, so the index is built after the write rather than
// before it.
func (inv Inventory) containerView(ctx context.Context, id inventory.ContainerID) (LocationView, error) {
	container, err := inv.Containers.Get(ctx, id)
	if err != nil {
		return LocationView{}, err
	}
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return LocationView{}, err
	}
	return LocationView{
		ID:          container.ID,
		Name:        container.Name,
		Description: container.Description,
		Path:        index.Path(inventory.ContainerLocation(container.ID)),
	}, nil
}
