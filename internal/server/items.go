package server

// Items operations plus the move endpoint. The location is a room or a
// container; tags are free-form labels owned by the item.

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// RegisterItems adds the items operations to the API.
func RegisterItems(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-items",
		Method:      http.MethodGet,
		Path:        "/items",
		Summary:     "List items",
		Description: "With no filter every item is listed. At most one of room_id and container_id may be set.",
		Tags:        []string{"Items"},
	}, func(ctx context.Context, input *ItemListInput) (*ItemListOutput, error) {
		var (
			items []inventory.Item
			err   error
		)
		switch {
		case input.RoomID != 0 && input.ContainerID != 0:
			return nil, &huma.ErrorModel{
				Status: http.StatusUnprocessableEntity,
				Detail: "specify at most one of room_id and container_id",
			}
		case input.RoomID != 0:
			items, err = deps.Repos.Items.ListByLocation(ctx, inventory.RoomLocation(inventory.RoomID(input.RoomID)))
		case input.ContainerID != 0:
			items, err = deps.Repos.Items.ListByLocation(ctx, inventory.ContainerLocation(inventory.ContainerID(input.ContainerID)))
		default:
			items, err = deps.Repos.Items.List(ctx)
		}
		if err != nil {
			return nil, mapError(deps, err)
		}
		out := make([]ItemResponse, len(items))
		for index, item := range items {
			out[index] = newItemResponse(item)
		}
		return &ItemListOutput{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-item",
		Method:        http.MethodPost,
		Path:          "/items",
		Summary:       "Create an item",
		Tags:          []string{"Items"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *ItemCreateInput) (*ItemOutput, error) {
		item := newItem(input.Body)
		if err := deps.Repos.Items.Create(ctx, &item); err != nil {
			return nil, mapError(deps, err)
		}
		return &ItemOutput{Body: newItemResponse(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-item",
		Method:      http.MethodGet,
		Path:        "/items/{id}",
		Summary:     "Get an item",
		Tags:        []string{"Items"},
	}, func(ctx context.Context, input *ItemPath) (*ItemOutput, error) {
		item, err := deps.Repos.Items.Get(ctx, inventory.ItemID(input.ID))
		if err != nil {
			return nil, mapError(deps, err)
		}
		return &ItemOutput{Body: newItemResponse(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-item",
		Method:      http.MethodPatch,
		Path:        "/items/{id}",
		Summary:     "Update an item",
		Tags:        []string{"Items"},
	}, func(ctx context.Context, input *ItemUpdateInput) (*ItemOutput, error) {
		item, err := deps.Repos.Items.Get(ctx, inventory.ItemID(input.ID))
		if err != nil {
			return nil, mapError(deps, err)
		}
		if input.Body.Name != nil {
			item.Name = *input.Body.Name
		}
		if input.Body.Description != nil {
			item.Description = *input.Body.Description
		}
		if input.Body.Quantity != nil {
			item.Quantity = *input.Body.Quantity
		}
		if input.Body.Notes != nil {
			item.Notes = *input.Body.Notes
		}
		if input.Body.Tags != nil {
			item.Tags = *input.Body.Tags
		}
		if err := deps.Repos.Items.Update(ctx, &item); err != nil {
			return nil, mapError(deps, err)
		}
		return &ItemOutput{Body: newItemResponse(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-item",
		Method:      http.MethodDelete,
		Path:        "/items/{id}",
		Summary:     "Delete an item",
		Tags:        []string{"Items"},
	}, func(ctx context.Context, input *ItemPath) (*struct{}, error) {
		if err := deps.Repos.Items.Delete(ctx, inventory.ItemID(input.ID)); err != nil {
			return nil, mapError(deps, err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "move-item",
		Method:      http.MethodPost,
		Path:        "/items/{id}/move",
		Summary:     "Move an item",
		Description: "The destination is a room or a container; only the location changes.",
		Tags:        []string{"Items"},
	}, func(ctx context.Context, input *ItemMoveInput) (*ItemOutput, error) {
		if err := deps.Repos.Items.Move(ctx, inventory.ItemID(input.ID), toLocation(input.Body.Destination)); err != nil {
			return nil, mapError(deps, err)
		}
		item, err := deps.Repos.Items.Get(ctx, inventory.ItemID(input.ID))
		if err != nil {
			return nil, mapError(deps, err)
		}
		return &ItemOutput{Body: newItemResponse(item)}, nil
	})
}

// LocationRef identifies a location in requests and responses: exactly one of
// the two kinds is valid.
type LocationRef struct {
	Kind string `json:"kind" enum:"room,container" doc:"Whether the location is a room or a container."`
	ID   int64  `json:"id" format:"int64" doc:"ID of the room or the container."`
}

// toLocation converts the API representation into the domain one. The core
// validates the kind and the existence of the reference.
func toLocation(ref LocationRef) inventory.Location {
	return inventory.Location{Kind: inventory.LocationKind(ref.Kind), ID: ref.ID}
}

func fromLocation(location inventory.Location) LocationRef {
	return LocationRef{Kind: string(location.Kind), ID: location.ID}
}

// ItemInput is the writable part of an item.
type ItemInput struct {
	Name        string      `json:"name" minLength:"1" maxLength:"120" example:"Drill" doc:"Name of the item, unique within its location."`
	Description string      `json:"description,omitempty" maxLength:"2000" doc:"Optional description of the item."`
	Quantity    int         `json:"quantity" minimum:"0" maximum:"1000000" doc:"How many of the item exist; zero means none left."`
	Notes       string      `json:"notes,omitempty" maxLength:"4000" doc:"Optional free-form notes."`
	Tags        []string    `json:"tags,omitempty" maxItems:"20" doc:"Free-form labels, unique within the item."`
	Location    LocationRef `json:"location" doc:"Where the item lives."`
}

func newItem(input ItemInput) inventory.Item {
	return inventory.Item{
		Name:        input.Name,
		Description: input.Description,
		Quantity:    input.Quantity,
		Notes:       input.Notes,
		Tags:        input.Tags,
		Location:    toLocation(input.Location),
	}
}

// ItemPatch carries the fields of an update request. Fields left out are not
// changed; tags with an empty array clear the tags, and null means
// unchanged.
type ItemPatch struct {
	Name        *string   `json:"name,omitempty" minLength:"1" maxLength:"120" doc:"New name of the item."`
	Description *string   `json:"description,omitempty" maxLength:"2000" doc:"New description of the item."`
	Quantity    *int      `json:"quantity,omitempty" minimum:"0" maximum:"1000000" doc:"New quantity."`
	Notes       *string   `json:"notes,omitempty" maxLength:"4000" doc:"New notes."`
	Tags        *[]string `json:"tags,omitempty" maxItems:"20" doc:"New tag set, replacing the existing one."`
}

// ItemResponse is the API representation of an item.
type ItemResponse struct {
	ID          int64       `json:"id" format:"int64" doc:"Item ID."`
	Name        string      `json:"name" example:"Drill"`
	Description string      `json:"description"`
	Quantity    int         `json:"quantity" example:"1"`
	Notes       string      `json:"notes"`
	Tags        []string    `json:"tags"`
	Location    LocationRef `json:"location" doc:"Where the item lives."`
	CreatedAt   time.Time   `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time   `json:"updated_at" format:"date-time"`
}

func newItemResponse(item inventory.Item) ItemResponse {
	tags := item.Tags
	if tags == nil {
		tags = []string{}
	}
	return ItemResponse{
		ID:          int64(item.ID),
		Name:        item.Name,
		Description: item.Description,
		Quantity:    item.Quantity,
		Notes:       item.Notes,
		Tags:        tags,
		Location:    fromLocation(item.Location),
	}
}

type ItemCreateInput struct {
	Body ItemInput `json:"body"`
}

type ItemUpdateInput struct {
	ID   int64     `json:"id" path:"id" format:"int64"`
	Body ItemPatch `json:"body"`
}

type ItemPath struct {
	ID int64 `json:"id" path:"id" format:"int64" doc:"Item ID."`
}

type ItemMoveInput struct {
	ID   int64    `json:"id" path:"id" format:"int64"`
	Body MoveBody `json:"body"`
}

type ItemOutput struct {
	Body ItemResponse `json:"body"`
}

type ItemListOutput struct {
	Body []ItemResponse `json:"body"`
}

type ItemListInput struct {
	RoomID      int64 `query:"room_id,omitempty" format:"int64" doc:"Only items directly in this room. Zero lists every location."`
	ContainerID int64 `query:"container_id,omitempty" format:"int64" doc:"Only items directly in this container. Zero lists every location."`
}

// MoveBody is the shared body of the move endpoints: the destination is a
// room or a container.
type MoveBody struct {
	Destination LocationRef `json:"destination" doc:"Where to move the entity."`
}
