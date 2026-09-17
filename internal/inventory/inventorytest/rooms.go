package inventorytest

import (
	"reflect"
	"slices"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// testRooms covers the inventory.RoomRepo contract.
func testRooms(t *testing.T, newRepos NewRepos) {
	t.Run("CreateAssignsIDAndTimestamps", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := inventory.Room{Name: "Garage", Description: "cars and tools"}
		requiresNoError(t, repos.Rooms.Create(ctx, &room), "Create")

		if room.ID <= 0 {
			t.Fatalf("created room ID = %d, want > 0", room.ID)
		}
		if room.CreatedAt.IsZero() || room.UpdatedAt.IsZero() {
			t.Fatalf("created room has zero timestamps: %+v", room)
		}
		if room.UpdatedAt.Before(room.CreatedAt) {
			t.Fatalf("UpdatedAt %v is before CreatedAt %v", room.UpdatedAt, room.CreatedAt)
		}

		got, err := repos.Rooms.Get(ctx, room.ID)
		requiresNoError(t, err, "Get")
		if !reflect.DeepEqual(got, room) {
			t.Fatalf("Get = %+v, want %+v", got, room)
		}
	})

	t.Run("CreateNormalizesWhitespace", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := inventory.Room{Name: "  Garage  ", Description: "  cars and tools  "}
		requiresNoError(t, repos.Rooms.Create(ctx, &room), "Create")
		if room.Name != "Garage" || room.Description != "cars and tools" {
			t.Fatalf("Create left %q / %q, want trimmed values", room.Name, room.Description)
		}

		got, err := repos.Rooms.Get(ctx, room.ID)
		requiresNoError(t, err, "Get")
		if got.Name != "Garage" || got.Description != "cars and tools" {
			t.Fatalf("Get = %+v, want trimmed values", got)
		}
	})

	t.Run("CreateRejectsInvalidInput", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := inventory.Room{Name: "   "}
		err := repos.Rooms.Create(ctx, &room)
		requiresError(t, err, inventory.ErrValidation, "Create with blank name")
		requiresValidationProblem(t, err, "name")

		rooms, err := repos.Rooms.List(ctx)
		requiresNoError(t, err, "List")
		if len(rooms) != 0 {
			t.Fatalf("invalid room was persisted: %+v", rooms)
		}
	})

	t.Run("CreateRejectsDuplicateName", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		createRoom(t, repos, "Garage")
		for _, name := range []string{"Garage", "garage", "  Garage  "} {
			room := inventory.Room{Name: name}
			requiresError(t, repos.Rooms.Create(ctx, &room), inventory.ErrConflict, "Create duplicate "+name)
		}

		rooms, err := repos.Rooms.List(ctx)
		requiresNoError(t, err, "List")
		if len(rooms) != 1 {
			t.Fatalf("persisted %d rooms, want 1: %+v", len(rooms), rooms)
		}
	})

	t.Run("UpdateRejectsDuplicateName", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		kitchen.Name = "garage"

		requiresError(t, repos.Rooms.Update(ctx, &kitchen), inventory.ErrConflict, "Update to a duplicate name")

		got, err := repos.Rooms.Get(ctx, kitchen.ID)
		requiresNoError(t, err, "Get")
		if got.Name != "Kitchen" {
			t.Fatalf("stored name = %q, want Kitchen", got.Name)
		}
	})

	t.Run("UpdateAcceptsTheStoredName", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		room.Description = "cars and tools"
		requiresNoError(t, repos.Rooms.Update(ctx, &room), "Update with its own name")
	})

	t.Run("DeleteFreesTheName", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		requiresNoError(t, repos.Rooms.Delete(ctx, room.ID), "Delete")
		createRoom(t, repos, "Garage")
	})

	t.Run("GetUnknownRoom", func(t *testing.T) {
		repos := newRepos(t)
		_, err := repos.Rooms.Get(t.Context(), 4242)
		requiresError(t, err, inventory.ErrNotFound, "Get")
	})

	t.Run("ListOrdersByNameIgnoringCase", func(t *testing.T) {
		repos := newRepos(t)
		for _, name := range []string{"garage", "Attic", "cellar"} {
			createRoom(t, repos, name)
		}

		rooms, err := repos.Rooms.List(t.Context())
		requiresNoError(t, err, "List")
		if got, want := roomNames(rooms), []string{"Attic", "cellar", "garage"}; !slices.Equal(got, want) {
			t.Fatalf("List = %v, want %v", got, want)
		}
	})

	t.Run("ListReturnsEmptySlice", func(t *testing.T) {
		repos := newRepos(t)
		rooms, err := repos.Rooms.List(t.Context())
		requiresNoError(t, err, "List")
		if rooms == nil {
			t.Fatal("List returned nil, want an empty slice")
		}
		if len(rooms) != 0 {
			t.Fatalf("List = %+v, want empty", rooms)
		}
	})

	t.Run("HostileNamesRoundTripVerbatim", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		name := `Garage'); DROP TABLE items; --`
		room := createRoom(t, repos, name)

		got, err := repos.Rooms.Get(ctx, room.ID)
		requiresNoError(t, err, "Get")
		if got.Name != name {
			t.Fatalf("name = %q, want %q", got.Name, name)
		}
		_, err = repos.Items.List(ctx)
		requiresNoError(t, err, "List items after hostile name")
	})

	t.Run("UpdateChangesFields", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		room.Name = "  Workshop  "
		room.Description = "  tools  "
		requiresNoError(t, repos.Rooms.Update(ctx, &room), "Update")
		if room.Name != "Workshop" || room.Description != "tools" {
			t.Fatalf("Update left %q / %q, want trimmed values", room.Name, room.Description)
		}

		got, err := repos.Rooms.Get(ctx, room.ID)
		requiresNoError(t, err, "Get")
		if got.Name != "Workshop" || got.Description != "tools" {
			t.Fatalf("Get = %+v, want updated values", got)
		}
		if got.UpdatedAt.Before(got.CreatedAt) {
			t.Fatalf("UpdatedAt %v is before CreatedAt %v", got.UpdatedAt, got.CreatedAt)
		}
	})

	t.Run("UpdateUnknownRoom", func(t *testing.T) {
		repos := newRepos(t)
		room := inventory.Room{ID: 4242, Name: "Ghost"}
		err := repos.Rooms.Update(t.Context(), &room)
		requiresError(t, err, inventory.ErrNotFound, "Update")
	})

	t.Run("UpdateRejectsInvalidInput", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		room.Name = "   "
		err := repos.Rooms.Update(ctx, &room)
		requiresError(t, err, inventory.ErrValidation, "Update with blank name")
		requiresValidationProblem(t, err, "name")
	})

	t.Run("DeleteRemovesRoom", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		requiresNoError(t, repos.Rooms.Delete(ctx, room.ID), "Delete")

		_, err := repos.Rooms.Get(ctx, room.ID)
		requiresError(t, err, inventory.ErrNotFound, "Get after Delete")

		requiresError(t, repos.Rooms.Delete(ctx, room.ID), inventory.ErrNotFound, "second Delete")
	})

	t.Run("DeleteNonEmptyRoomConflicts", func(t *testing.T) {
		repos := newRepos(t)

		withContainer := createRoom(t, repos, "Garage")
		createContainer(t, repos, withContainer, nil, "Toolbox")
		requiresError(t, repos.Rooms.Delete(t.Context(), withContainer.ID), inventory.ErrConflict, "Delete room with container")

		withItem := createRoom(t, repos, "Kitchen")
		createItem(t, repos, "Cups", inventory.RoomLocation(withItem.ID))
		requiresError(t, repos.Rooms.Delete(t.Context(), withItem.ID), inventory.ErrConflict, "Delete room with item")
	})
}

// roomNames extracts the names of rooms in listing order.
func roomNames(rooms []inventory.Room) []string {
	names := make([]string, len(rooms))
	for i, room := range rooms {
		names[i] = room.Name
	}
	return names
}
