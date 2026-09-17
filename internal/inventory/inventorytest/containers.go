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

	t.Run("CreateRejectsDuplicateNameInRoom", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		createContainer(t, repos, room, nil, "Toolbox")
		shelf := createContainer(t, repos, room, nil, "Shelf")

		duplicate := inventory.Container{RoomID: room.ID, Name: "toolbox"}
		requiresError(t, repos.Containers.Create(ctx, &duplicate), inventory.ErrConflict, "Create duplicate name")

		nested := inventory.Container{RoomID: room.ID, ParentID: &shelf.ID, Name: "TOOLBOX"}
		requiresError(t, repos.Containers.Create(ctx, &nested), inventory.ErrConflict, "Create duplicate name at another depth")
	})

	t.Run("CreateAllowsSameNameInDifferentRooms", func(t *testing.T) {
		repos := newRepos(t)
		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")

		createContainer(t, repos, garage, nil, "Toolbox")
		createContainer(t, repos, kitchen, nil, "Toolbox")
	})

	t.Run("UpdateRejectsDuplicateNameInRoom", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		createContainer(t, repos, room, nil, "Toolbox")
		shelf := createContainer(t, repos, room, nil, "Shelf")

		shelf.Name = " Toolbox "
		requiresError(t, repos.Containers.Update(t.Context(), &shelf), inventory.ErrConflict, "Update to a duplicate name")
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

	t.Run("PathIncludesRoomAndAncestors", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")
		drawer := createContainer(t, repos, room, &toolbox.ID, "Drawer 1")
		compartment := createContainer(t, repos, room, &drawer.ID, "Compartment")

		path, err := repos.Containers.Path(ctx, compartment.ID)
		requiresNoError(t, err, "Path")
		if path.Room.ID != room.ID {
			t.Fatalf("path room = %d, want %d", path.Room.ID, room.ID)
		}
		want := []string{"Toolbox", "Drawer 1", "Compartment"}
		if got := containerNames(path.Containers); !slices.Equal(got, want) {
			t.Fatalf("path containers = %v, want %v", got, want)
		}
		if got, want := path.String(), "Garage > Toolbox > Drawer 1 > Compartment"; got != want {
			t.Fatalf("String() = %q, want %q", got, want)
		}
	})

	t.Run("PathOfRootContainer", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")

		path, err := repos.Containers.Path(t.Context(), toolbox.ID)
		requiresNoError(t, err, "Path")
		if got, want := path.String(), "Garage > Toolbox"; got != want {
			t.Fatalf("String() = %q, want %q", got, want)
		}
	})

	t.Run("PathUnknownContainer", func(t *testing.T) {
		repos := newRepos(t)
		_, err := repos.Containers.Path(t.Context(), 4242)
		requiresError(t, err, inventory.ErrNotFound, "Path")
	})

	t.Run("MoveSubtreeToAnotherRoom", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		toolbox := createContainer(t, repos, garage, nil, "Toolbox")
		drawer := createContainer(t, repos, garage, &toolbox.ID, "Drawer")

		requiresNoError(t, repos.Containers.Move(ctx, toolbox.ID, inventory.RoomLocation(kitchen.ID)), "Move")

		gotToolbox, err := repos.Containers.Get(ctx, toolbox.ID)
		requiresNoError(t, err, "Get toolbox")
		if gotToolbox.RoomID != kitchen.ID || gotToolbox.ParentID != nil {
			t.Fatalf("toolbox after move: room %d parent %v, want room %d parent nil", gotToolbox.RoomID, gotToolbox.ParentID, kitchen.ID)
		}
		gotDrawer, err := repos.Containers.Get(ctx, drawer.ID)
		requiresNoError(t, err, "Get drawer")
		if gotDrawer.RoomID != kitchen.ID {
			t.Fatalf("drawer room after move = %d, want %d (the subtree follows)", gotDrawer.RoomID, kitchen.ID)
		}

		inGarage, err := repos.Containers.ListByRoom(ctx, garage.ID)
		requiresNoError(t, err, "ListByRoom garage")
		if len(inGarage) != 0 {
			t.Fatalf("containers stayed in the old room: %+v", inGarage)
		}
		path, err := repos.Containers.Path(ctx, drawer.ID)
		requiresNoError(t, err, "Path")
		if got, want := path.String(), "Kitchen > Toolbox > Drawer"; got != want {
			t.Fatalf("path after move = %q, want %q", got, want)
		}
	})

	t.Run("MoveUnderContainerInAnotherRoom", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		toolbox := createContainer(t, repos, garage, nil, "Toolbox")
		drawer := createContainer(t, repos, garage, &toolbox.ID, "Drawer")
		cupboard := createContainer(t, repos, kitchen, nil, "Cupboard")

		requiresNoError(t, repos.Containers.Move(ctx, toolbox.ID, inventory.ContainerLocation(cupboard.ID)), "Move")

		gotToolbox, err := repos.Containers.Get(ctx, toolbox.ID)
		requiresNoError(t, err, "Get toolbox")
		if gotToolbox.RoomID != kitchen.ID || gotToolbox.ParentID == nil || *gotToolbox.ParentID != cupboard.ID {
			t.Fatalf("toolbox after move: room %d parent %v, want room %d parent %d", gotToolbox.RoomID, gotToolbox.ParentID, kitchen.ID, cupboard.ID)
		}
		gotDrawer, err := repos.Containers.Get(ctx, drawer.ID)
		requiresNoError(t, err, "Get drawer")
		if gotDrawer.RoomID != kitchen.ID || gotDrawer.ParentID == nil || *gotDrawer.ParentID != toolbox.ID {
			t.Fatalf("drawer after move: room %d parent %v, want room %d parent %d", gotDrawer.RoomID, gotDrawer.ParentID, kitchen.ID, toolbox.ID)
		}
	})

	t.Run("MoveToRoomDestinationBecomesRoot", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")
		drawer := createContainer(t, repos, room, &toolbox.ID, "Drawer")

		requiresNoError(t, repos.Containers.Move(ctx, drawer.ID, inventory.RoomLocation(room.ID)), "Move")

		got, err := repos.Containers.Get(ctx, drawer.ID)
		requiresNoError(t, err, "Get")
		if got.ParentID != nil || got.RoomID != room.ID {
			t.Fatalf("drawer after move: room %d parent %v, want room %d parent nil", got.RoomID, got.ParentID, room.ID)
		}
	})

	t.Run("MoveRejectsCycles", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")
		drawer := createContainer(t, repos, room, &toolbox.ID, "Drawer")

		requiresError(t, repos.Containers.Move(ctx, toolbox.ID, inventory.ContainerLocation(drawer.ID)), inventory.ErrCycle, "move under a descendant")
		requiresError(t, repos.Containers.Move(ctx, toolbox.ID, inventory.ContainerLocation(toolbox.ID)), inventory.ErrCycle, "move under itself")
	})

	t.Run("MoveRejectsUnknownDestination", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")

		requiresError(t, repos.Containers.Move(ctx, toolbox.ID, inventory.RoomLocation(4242)), inventory.ErrNotFound, "Move to unknown room")
		requiresError(t, repos.Containers.Move(ctx, toolbox.ID, inventory.ContainerLocation(4242)), inventory.ErrNotFound, "Move to unknown container")
	})

	t.Run("MoveUnknownContainer", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		requiresError(t, repos.Containers.Move(t.Context(), 4242, inventory.RoomLocation(room.ID)), inventory.ErrNotFound, "Move")
	})

	t.Run("MoveRejectsNameCollisionInDestinationRoom", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		toolbox := createContainer(t, repos, garage, nil, "Toolbox")
		createContainer(t, repos, kitchen, nil, "Toolbox")

		requiresError(t, repos.Containers.Move(ctx, toolbox.ID, inventory.RoomLocation(kitchen.ID)), inventory.ErrConflict, "Move onto a duplicate name")
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

	t.Run("UpdateRejectsCycles", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")
		drawer := createContainer(t, repos, room, &toolbox.ID, "Drawer")
		compartment := createContainer(t, repos, room, &drawer.ID, "Compartment")

		toolbox.ParentID = &drawer.ID
		requiresError(t, repos.Containers.Update(ctx, &toolbox), inventory.ErrCycle, "re-parent under a direct child")

		toolbox.ParentID = &compartment.ID
		requiresError(t, repos.Containers.Update(ctx, &toolbox), inventory.ErrCycle, "re-parent under a deeper descendant")

		got, err := repos.Containers.Get(ctx, toolbox.ID)
		requiresNoError(t, err, "Get")
		if got.ParentID != nil {
			t.Fatalf("ParentID = %d, want nil after refused updates", *got.ParentID)
		}

		toolbox.ParentID = nil
		requiresNoError(t, repos.Containers.Update(ctx, &toolbox), "rename with the parent unchanged")
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
