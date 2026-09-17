package inventorytest

import (
	"reflect"
	"slices"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// testContainers covers the inventory.ContainerRepo contract.
func testContainers(t *testing.T, newRepos NewRepos) {
	t.Run("CreateRootContainer", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		container := inventory.Container{RoomID: room.ID, Name: "Toolbox", Description: "red metal box"}
		requiresNoError(t, repos.Containers.Create(ctx, &container), "Create")

		if container.ID <= 0 {
			t.Fatalf("created container ID = %d, want > 0", container.ID)
		}
		if container.ParentID != nil {
			t.Fatalf("ParentID = %d, want nil", *container.ParentID)
		}
		if container.RoomID != room.ID {
			t.Fatalf("RoomID = %d, want %d", container.RoomID, room.ID)
		}

		got, err := repos.Containers.Get(ctx, container.ID)
		requiresNoError(t, err, "Get")
		if !reflect.DeepEqual(got, container) {
			t.Fatalf("Get = %+v, want %+v", got, container)
		}
	})

	t.Run("CreateNestedContainer", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		parent := createContainer(t, repos, room, nil, "Toolbox")
		child := createContainer(t, repos, room, &parent.ID, "Drawer 1")

		if child.ParentID == nil || *child.ParentID != parent.ID {
			t.Fatalf("ParentID = %v, want %d", child.ParentID, parent.ID)
		}
		got, err := repos.Containers.Get(ctx, child.ID)
		requiresNoError(t, err, "Get")
		if got.ParentID == nil || *got.ParentID != parent.ID {
			t.Fatalf("Get ParentID = %v, want %d", got.ParentID, parent.ID)
		}
	})

	t.Run("CreateNormalizesWhitespace", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")

		container := inventory.Container{RoomID: room.ID, Name: "  Toolbox  ", Description: "  red  "}
		requiresNoError(t, repos.Containers.Create(t.Context(), &container), "Create")
		if container.Name != "Toolbox" || container.Description != "red" {
			t.Fatalf("Create left %q / %q, want trimmed values", container.Name, container.Description)
		}
	})

	t.Run("CreateRequiresRoomReference", func(t *testing.T) {
		repos := newRepos(t)
		container := inventory.Container{RoomID: 0, Name: "Toolbox"}
		err := repos.Containers.Create(t.Context(), &container)
		requiresError(t, err, inventory.ErrValidation, "Create without room")
		requiresValidationProblem(t, err, "room_id")
	})

	t.Run("CreateUnknownRoom", func(t *testing.T) {
		repos := newRepos(t)
		container := inventory.Container{RoomID: 4242, Name: "Toolbox"}
		err := repos.Containers.Create(t.Context(), &container)
		requiresError(t, err, inventory.ErrNotFound, "Create with unknown room")
	})

	t.Run("CreateUnknownParent", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		parent := inventory.ContainerID(4242)
		container := inventory.Container{RoomID: room.ID, ParentID: &parent, Name: "Drawer"}
		err := repos.Containers.Create(t.Context(), &container)
		requiresError(t, err, inventory.ErrNotFound, "Create with unknown parent")
	})

	t.Run("CreateParentFromAnotherRoom", func(t *testing.T) {
		repos := newRepos(t)
		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		parent := createContainer(t, repos, kitchen, nil, "Cupboard")

		container := inventory.Container{RoomID: garage.ID, ParentID: &parent.ID, Name: "Drawer"}
		err := repos.Containers.Create(t.Context(), &container)
		requiresError(t, err, inventory.ErrValidation, "Create with cross-room parent")
		requiresValidationProblem(t, err, "parent_id")
	})

	t.Run("ListByRoomIsOrderedAndScoped", func(t *testing.T) {
		repos := newRepos(t)
		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")

		parent := createContainer(t, repos, garage, nil, "toolbox")
		createContainer(t, repos, garage, &parent.ID, "Drawer 1")
		createContainer(t, repos, garage, nil, "Bench")
		createContainer(t, repos, kitchen, nil, "Cupboard")

		containers, err := repos.Containers.ListByRoom(t.Context(), garage.ID)
		requiresNoError(t, err, "ListByRoom")
		want := []string{"Bench", "Drawer 1", "toolbox"}
		if got := containerNames(containers); !slices.Equal(got, want) {
			t.Fatalf("ListByRoom = %v, want %v", got, want)
		}
	})

	t.Run("ListByRoomUnknownRoom", func(t *testing.T) {
		repos := newRepos(t)
		_, err := repos.Containers.ListByRoom(t.Context(), 4242)
		requiresError(t, err, inventory.ErrNotFound, "ListByRoom")
	})

	t.Run("GetUnknownContainer", func(t *testing.T) {
		repos := newRepos(t)
		_, err := repos.Containers.Get(t.Context(), 4242)
		requiresError(t, err, inventory.ErrNotFound, "Get")
	})

	t.Run("UpdateRenamesAndReParents", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		parent := createContainer(t, repos, room, nil, "Toolbox")
		child := createContainer(t, repos, room, &parent.ID, "Drawer")
		other := createContainer(t, repos, room, nil, "Shelf")

		child.Name = "  Drawer 1  "
		child.ParentID = &other.ID
		requiresNoError(t, repos.Containers.Update(ctx, &child), "Update")
		if child.Name != "Drawer 1" {
			t.Fatalf("Name = %q, want trimmed value", child.Name)
		}

		got, err := repos.Containers.Get(ctx, child.ID)
		requiresNoError(t, err, "Get")
		if !reflect.DeepEqual(got, child) {
			t.Fatalf("Get = %+v, want %+v", got, child)
		}

		child.ParentID = nil
		requiresNoError(t, repos.Containers.Update(ctx, &child), "Update detaching parent")
		got, err = repos.Containers.Get(ctx, child.ID)
		requiresNoError(t, err, "Get")
		if got.ParentID != nil {
			t.Fatalf("ParentID = %d, want nil", *got.ParentID)
		}
	})

	t.Run("UpdateUnknownContainer", func(t *testing.T) {
		repos := newRepos(t)
		container := inventory.Container{ID: 4242, RoomID: 1, Name: "Ghost"}
		err := repos.Containers.Update(t.Context(), &container)
		requiresError(t, err, inventory.ErrNotFound, "Update")
	})

	t.Run("UpdateRejectsRoomChange", func(t *testing.T) {
		repos := newRepos(t)
		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		container := createContainer(t, repos, garage, nil, "Toolbox")

		container.RoomID = kitchen.ID
		err := repos.Containers.Update(t.Context(), &container)
		requiresError(t, err, inventory.ErrValidation, "Update with a different room")
		requiresValidationProblem(t, err, "room_id")
	})

	t.Run("UpdateRejectsCrossRoomParent", func(t *testing.T) {
		repos := newRepos(t)
		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		container := createContainer(t, repos, garage, nil, "Toolbox")
		foreign := createContainer(t, repos, kitchen, nil, "Cupboard")

		container.ParentID = &foreign.ID
		err := repos.Containers.Update(t.Context(), &container)
		requiresError(t, err, inventory.ErrValidation, "Update with a cross-room parent")
		requiresValidationProblem(t, err, "parent_id")
	})

	t.Run("UpdateRejectsSelfParent", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")

		container.ParentID = &container.ID
		err := repos.Containers.Update(t.Context(), &container)
		requiresError(t, err, inventory.ErrValidation, "Update with itself as parent")
		requiresValidationProblem(t, err, "parent_id")
	})

	t.Run("DeleteEmptyContainer", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")

		requiresNoError(t, repos.Containers.Delete(ctx, container.ID), "Delete")
		_, err := repos.Containers.Get(ctx, container.ID)
		requiresError(t, err, inventory.ErrNotFound, "Get after Delete")
		requiresError(t, repos.Containers.Delete(ctx, container.ID), inventory.ErrNotFound, "second Delete")
	})

	t.Run("DeleteContainerWithChildConflicts", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		parent := createContainer(t, repos, room, nil, "Toolbox")
		child := createContainer(t, repos, room, &parent.ID, "Drawer")

		requiresError(t, repos.Containers.Delete(ctx, parent.ID), inventory.ErrConflict, "Delete container with child")

		requiresNoError(t, repos.Containers.Delete(ctx, child.ID), "Delete child")
		requiresNoError(t, repos.Containers.Delete(ctx, parent.ID), "Delete parent after child")
	})

	t.Run("DeleteContainerWithItemConflicts", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")
		item := createItem(t, repos, "Screwdriver", inventory.ContainerLocation(container.ID))

		requiresError(t, repos.Containers.Delete(ctx, container.ID), inventory.ErrConflict, "Delete container with item")

		requiresNoError(t, repos.Items.Delete(ctx, item.ID), "Delete item")
		requiresNoError(t, repos.Containers.Delete(ctx, container.ID), "Delete container after item")
	})
}

// containerNames extracts the names of containers in listing order.
func containerNames(containers []inventory.Container) []string {
	names := make([]string, len(containers))
	for i, container := range containers {
		names[i] = container.Name
	}
	return names
}
