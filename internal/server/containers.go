package server

// Containers operations. Nesting is expressed with parent_id (a container in
// a room or inside another container of the same room); changing room is a
// move, not an update.

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// RegisterContainers adds the containers operations to the API.
func RegisterContainers(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-containers",
		Method:      http.MethodGet,
		Path:        "/containers",
		Summary:     "List containers",
		Tags:        []string{"Containers"},
	}, func(ctx context.Context, input *ContainerListInput) (*ContainerListOutput, error) {
		var (
			containers []inventory.Container
			err        error
		)
		if input.RoomID == 0 {
			containers, err = deps.Repos.Containers.List(ctx)
		} else {
			containers, err = deps.Repos.Containers.ListByRoom(ctx, inventory.RoomID(input.RoomID))
		}
		if err != nil {
			return nil, mapError(deps, err)
		}
		out := make([]ContainerResponse, len(containers))
		for index, container := range containers {
			out[index] = newContainerResponse(container)
		}
		return &ContainerListOutput{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-container",
		Method:        http.MethodPost,
		Path:          "/containers",
		Summary:       "Create a container",
		Tags:          []string{"Containers"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *ContainerCreateInput) (*ContainerOutput, error) {
		container := inventory.Container{
			RoomID:      inventory.RoomID(input.Body.RoomID),
			Name:        input.Body.Name,
			Description: input.Body.Description,
		}
		if input.Body.ParentID != nil {
			parent := inventory.ContainerID(*input.Body.ParentID)
			container.ParentID = &parent
		}
		if err := deps.Repos.Containers.Create(ctx, &container); err != nil {
			return nil, mapError(deps, err)
		}
		return &ContainerOutput{Body: newContainerResponse(container)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-container",
		Method:      http.MethodGet,
		Path:        "/containers/{id}",
		Summary:     "Get a container",
		Tags:        []string{"Containers"},
	}, func(ctx context.Context, input *ContainerPath) (*ContainerDetailOutput, error) {
		id := inventory.ContainerID(input.ID)
		container, err := deps.Repos.Containers.Get(ctx, id)
		if err != nil {
			return nil, mapError(deps, err)
		}
		path, err := deps.Repos.Containers.Path(ctx, id)
		if err != nil {
			return nil, mapError(deps, err)
		}
		detail := newContainerDetailResponse(container, path)
		return &ContainerDetailOutput{Body: detail}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-container",
		Method:      http.MethodPatch,
		Path:        "/containers/{id}",
		Summary:     "Update a container",
		Tags:        []string{"Containers"},
	}, func(ctx context.Context, input *ContainerUpdateInput) (*ContainerOutput, error) {
		container, err := deps.Repos.Containers.Get(ctx, inventory.ContainerID(input.ID))
		if err != nil {
			return nil, mapError(deps, err)
		}
		if input.Body.Name != nil {
			container.Name = *input.Body.Name
		}
		if input.Body.Description != nil {
			container.Description = *input.Body.Description
		}
		if err := deps.Repos.Containers.Update(ctx, &container); err != nil {
			return nil, mapError(deps, err)
		}
		return &ContainerOutput{Body: newContainerResponse(container)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-container",
		Method:      http.MethodDelete,
		Path:        "/containers/{id}",
		Summary:     "Delete an empty container",
		Tags:        []string{"Containers"},
	}, func(ctx context.Context, input *ContainerPath) (*struct{}, error) {
		if err := deps.Repos.Containers.Delete(ctx, inventory.ContainerID(input.ID)); err != nil {
			return nil, mapError(deps, err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "move-container",
		Method:      http.MethodPost,
		Path:        "/containers/{id}/move",
		Summary:     "Move a container and its subtree",
		Description: "The destination is a room (the container becomes a root of that room) or a container (it becomes a child of it and the whole subtree adopts its room). The move is atomic.",
		Tags:        []string{"Containers"},
	}, func(ctx context.Context, input *ContainerMoveInput) (*ContainerDetailOutput, error) {
		if err := deps.Repos.Containers.Move(ctx, inventory.ContainerID(input.ID), toLocation(input.Body.Destination)); err != nil {
			return nil, mapError(deps, err)
		}
		container, err := deps.Repos.Containers.Get(ctx, inventory.ContainerID(input.ID))
		if err != nil {
			return nil, mapError(deps, err)
		}
		path, err := deps.Repos.Containers.Path(ctx, container.ID)
		if err != nil {
			return nil, mapError(deps, err)
		}
		return &ContainerDetailOutput{Body: newContainerDetailResponse(container, path)}, nil
	})
}

// ContainerInput is the writable part of a container. parent_id is optional:
// when set it must reference an existing container in the same room.
type ContainerInput struct {
	Name        string `json:"name" minLength:"1" maxLength:"120" example:"Toolbox" doc:"Name of the container, unique within its room."`
	Description string `json:"description,omitempty" maxLength:"2000" doc:"Optional description of the container."`
	RoomID      int64  `json:"room_id" format:"int64" doc:"ID of the room the container sits in."`
	ParentID    *int64 `json:"parent_id,omitempty" format:"int64" doc:"ID of the containing container, or empty when the container sits directly in the room."`
}

// ContainerPatch carries the fields of an update request. Moving a container
// to another room is a move (POST /containers/{id}/move), not a patch.
type ContainerPatch struct {
	Name        *string `json:"name,omitempty" minLength:"1" maxLength:"120" doc:"New name of the container."`
	Description *string `json:"description,omitempty" maxLength:"2000" doc:"New description of the container."`
}

// ContainerResponse is the API representation of a container in listings.
type ContainerResponse struct {
	ID          int64     `json:"id" format:"int64" doc:"Container ID."`
	RoomID      int64     `json:"room_id" format:"int64" doc:"ID of the room the container belongs to."`
	ParentID    *int64    `json:"parent_id,omitempty" format:"int64" doc:"ID of the containing container, or empty when the container sits directly in the room."`
	Name        string    `json:"name" example:"Toolbox"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

// ContainerDetail is a container plus its computed location path.
type ContainerDetail struct {
	ContainerResponse
	Path string `json:"path" example:"Garage > Toolbox > Drawer 1" doc:"Location of the container: room and containers from the top down."`
}

func newContainerResponse(container inventory.Container) ContainerResponse {
	var parent *int64
	if container.ParentID != nil {
		value := int64(*container.ParentID)
		parent = &value
	}
	return ContainerResponse{
		ID:          int64(container.ID),
		RoomID:      int64(container.RoomID),
		ParentID:    parent,
		Name:        container.Name,
		Description: container.Description,
		CreatedAt:   container.CreatedAt,
		UpdatedAt:   container.UpdatedAt,
	}
}

func newContainerDetailResponse(container inventory.Container, path inventory.ContainerPath) ContainerDetail {
	return ContainerDetail{
		ContainerResponse: newContainerResponse(container),
		Path:              path.String(),
	}
}

type ContainerCreateInput struct {
	Body ContainerInput `json:"body"`
}

type ContainerUpdateInput struct {
	ID   int64          `json:"id" path:"id" format:"int64"`
	Body ContainerPatch `json:"body"`
}

type ContainerPath struct {
	ID int64 `json:"id" path:"id" format:"int64" doc:"Container ID."`
}

type ContainerMoveInput struct {
	ID   int64    `json:"id" path:"id" format:"int64"`
	Body MoveBody `json:"body"`
}

type ContainerOutput struct {
	Body ContainerResponse `json:"body"`
}

type ContainerDetailOutput struct {
	Body ContainerDetail `json:"body"`
}

type ContainerListInput struct {
	RoomID int64 `query:"room_id" format:"int64" doc:"Only containers of this room. Zero lists every room."`
}

type ContainerListOutput struct {
	Body []ContainerResponse `json:"body"`
}
