package server

// Rooms operations: the thinnest possible layer. Handlers parse the request
// into DTOs, call the inventory core and map errors with mapError; business
// logic lives in internal/inventory only.

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// RegisterRooms adds the rooms operations to the API.
func RegisterRooms(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-rooms",
		Method:      http.MethodGet,
		Path:        "/rooms",
		Summary:     "List rooms",
		Tags:        []string{"Rooms"},
	}, func(ctx context.Context, input *struct{}) (*RoomListOutput, error) {
		rooms, err := deps.Repos.Rooms.List(ctx)
		if err != nil {
			return nil, mapError(deps, err)
		}
		out := make([]RoomResponse, len(rooms))
		for index, room := range rooms {
			out[index] = newRoomResponse(room)
		}
		return &RoomListOutput{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-room",
		Method:        http.MethodPost,
		Path:          "/rooms",
		Summary:       "Create a room",
		Tags:          []string{"Rooms"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *RoomCreateInput) (*RoomOutput, error) {
		room := inventory.Room{Name: input.Body.Name, Description: input.Body.Description}
		if err := deps.Repos.Rooms.Create(ctx, &room); err != nil {
			return nil, mapError(deps, err)
		}
		return &RoomOutput{Body: newRoomResponse(room)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-room",
		Method:      http.MethodGet,
		Path:        "/rooms/{id}",
		Summary:     "Get a room",
		Tags:        []string{"Rooms"},
	}, func(ctx context.Context, input *RoomPath) (*RoomOutput, error) {
		room, err := deps.Repos.Rooms.Get(ctx, inventory.RoomID(input.ID))
		if err != nil {
			return nil, mapError(deps, err)
		}
		return &RoomOutput{Body: newRoomResponse(room)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-room",
		Method:      http.MethodPatch,
		Path:        "/rooms/{id}",
		Summary:     "Update a room",
		Tags:        []string{"Rooms"},
	}, func(ctx context.Context, input *RoomUpdateInput) (*RoomOutput, error) {
		room, err := deps.Repos.Rooms.Get(ctx, inventory.RoomID(input.ID))
		if err != nil {
			return nil, mapError(deps, err)
		}
		if input.Body.Name != nil {
			room.Name = *input.Body.Name
		}
		if input.Body.Description != nil {
			room.Description = *input.Body.Description
		}
		if err := deps.Repos.Rooms.Update(ctx, &room); err != nil {
			return nil, mapError(deps, err)
		}
		return &RoomOutput{Body: newRoomResponse(room)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-room",
		Method:      http.MethodDelete,
		Path:        "/rooms/{id}",
		Summary:     "Delete an empty room",
		Tags:        []string{"Rooms"},
	}, func(ctx context.Context, input *RoomPath) (*struct{}, error) {
		if err := deps.Repos.Rooms.Delete(ctx, inventory.RoomID(input.ID)); err != nil {
			return nil, mapError(deps, err)
		}
		return nil, nil
	})
}

// RoomInput is the writable part of a room. The schema-level tags only
// constrain the format; the inventory rules (normalization, uniqueness,
// length in runes) are enforced by the core.
type RoomInput struct {
	Name        string `json:"name" minLength:"1" maxLength:"120" example:"Garage" doc:"Name of the room, unique across the inventory."`
	Description string `json:"description,omitempty" maxLength:"2000" doc:"Optional description of the room."`
}

// RoomPatch carries the fields of an update request. Fields left out are not
// changed; null and missing are the same thing.
type RoomPatch struct {
	Name        *string `json:"name,omitempty" minLength:"1" maxLength:"120" doc:"New name of the room."`
	Description *string `json:"description,omitempty" maxLength:"2000" doc:"New description of the room."`
}

// RoomResponse is the API representation of a room.
type RoomResponse struct {
	ID          int64     `json:"id" format:"int64" doc:"Room ID."`
	Name        string    `json:"name" example:"Garage"`
	Description string    `json:"description" example:"Cars and tools"`
	CreatedAt   time.Time `json:"created_at" format:"date-time" doc:"When the room was created."`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time" doc:"When the room was last updated."`
}

func newRoomResponse(room inventory.Room) RoomResponse {
	return RoomResponse{
		ID:          int64(room.ID),
		Name:        room.Name,
		Description: room.Description,
		CreatedAt:   room.CreatedAt,
		UpdatedAt:   room.UpdatedAt,
	}
}

type RoomCreateInput struct {
	Body RoomInput `json:"body"`
}

type RoomUpdateInput struct {
	ID   int64     `json:"id" path:"id" format:"int64"`
	Body RoomPatch `json:"body"`
}

type RoomPath struct {
	ID int64 `json:"id" path:"id" format:"int64" doc:"Room ID."`
}

type RoomOutput struct {
	Body RoomResponse `json:"body"`
}

type RoomListOutput struct {
	Body []RoomResponse `json:"body"`
}
