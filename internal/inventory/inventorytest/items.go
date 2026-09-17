package inventorytest

import (
	"reflect"
	"slices"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// testItems covers the inventory.ItemRepo contract.
func testItems(t *testing.T, newRepos NewRepos) {
	t.Run("CreateInRoom", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := inventory.Item{
			Name:        "Drill",
			Description: "cordless",
			Quantity:    2,
			Notes:       "battery in the charger",
			Location:    inventory.RoomLocation(room.ID),
		}
		requiresNoError(t, repos.Items.Create(ctx, &item), "Create")

		if item.ID <= 0 {
			t.Fatalf("created item ID = %d, want > 0", item.ID)
		}
		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if !reflect.DeepEqual(got, item) {
			t.Fatalf("Get = %+v, want %+v", got, item)
		}
	})

	t.Run("CreateInContainer", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")
		location := inventory.ContainerLocation(container.ID)

		item := createItem(t, repos, "Screwdriver", location)
		if item.Location != location {
			t.Fatalf("Location = %+v, want %+v", item.Location, location)
		}

		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if got.Location != location {
			t.Fatalf("Get Location = %+v, want %+v", got.Location, location)
		}
	})

	t.Run("CreateNormalizesWhitespace", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")

		item := inventory.Item{
			Name:        "  Drill  ",
			Description: "  cordless  ",
			Notes:       "  battery  ",
			Quantity:    1,
			Location:    inventory.RoomLocation(room.ID),
		}
		requiresNoError(t, repos.Items.Create(t.Context(), &item), "Create")
		if item.Name != "Drill" || item.Description != "cordless" || item.Notes != "battery" {
			t.Fatalf("Create left %q / %q / %q, want trimmed values", item.Name, item.Description, item.Notes)
		}
	})

	t.Run("CreateValidatesFields", func(t *testing.T) {
		cases := []struct {
			name  string
			item  inventory.Item
			field string
		}{
			{"empty name", inventory.Item{Name: "  ", Quantity: 1, Location: inventory.RoomLocation(1)}, "name"},
			{"negative quantity", inventory.Item{Name: "Cups", Quantity: -1, Location: inventory.RoomLocation(1)}, "quantity"},
			{"quantity above maximum", inventory.Item{Name: "Cups", Quantity: inventory.MaxQuantity + 1, Location: inventory.RoomLocation(1)}, "quantity"},
			{"zero location ID", inventory.Item{Name: "Cups", Quantity: 1, Location: inventory.RoomLocation(0)}, "location.id"},
			{"unknown location kind", inventory.Item{Name: "Cups", Quantity: 1, Location: inventory.Location{Kind: "shelf", ID: 1}}, "location.kind"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repos := newRepos(t)
				item := tc.item
				err := repos.Items.Create(t.Context(), &item)
				requiresError(t, err, inventory.ErrValidation, "Create")
				requiresValidationProblem(t, err, tc.field)
			})
		}
	})

	t.Run("CreateUnknownLocation", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		inRoom := inventory.Item{Name: "Drill", Quantity: 1, Location: inventory.RoomLocation(4242)}
		requiresError(t, repos.Items.Create(ctx, &inRoom), inventory.ErrNotFound, "Create in unknown room")

		inContainer := inventory.Item{Name: "Drill", Quantity: 1, Location: inventory.ContainerLocation(4242)}
		requiresError(t, repos.Items.Create(ctx, &inContainer), inventory.ErrNotFound, "Create in unknown container")
	})

	t.Run("GetUnknownItem", func(t *testing.T) {
		repos := newRepos(t)
		_, err := repos.Items.Get(t.Context(), 4242)
		requiresError(t, err, inventory.ErrNotFound, "Get")
	})

	t.Run("ListIsOrderedAcrossLocations", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")

		createItem(t, repos, "drill", inventory.RoomLocation(room.ID))
		createItem(t, repos, "Allen keys", inventory.ContainerLocation(container.ID))
		createItem(t, repos, "Cups", inventory.RoomLocation(room.ID))

		items, err := repos.Items.List(t.Context())
		requiresNoError(t, err, "List")
		want := []string{"Allen keys", "Cups", "drill"}
		if got := itemNames(items); !slices.Equal(got, want) {
			t.Fatalf("List = %v, want %v", got, want)
		}
	})

	t.Run("ListByLocationScopesToTheLocation", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")
		shelf := createContainer(t, repos, room, nil, "Shelf")

		createItem(t, repos, "Cups", inventory.RoomLocation(room.ID))
		createItem(t, repos, "Plates", inventory.RoomLocation(room.ID))
		createItem(t, repos, "Screws", inventory.ContainerLocation(toolbox.ID))
		createItem(t, repos, "Paint", inventory.ContainerLocation(shelf.ID))

		inRoom, err := repos.Items.ListByLocation(ctx, inventory.RoomLocation(room.ID))
		requiresNoError(t, err, "ListByLocation room")
		if got, want := itemNames(inRoom), []string{"Cups", "Plates"}; !slices.Equal(got, want) {
			t.Fatalf("ListByLocation room = %v, want %v", got, want)
		}

		inToolbox, err := repos.Items.ListByLocation(ctx, inventory.ContainerLocation(toolbox.ID))
		requiresNoError(t, err, "ListByLocation container")
		if got, want := itemNames(inToolbox), []string{"Screws"}; !slices.Equal(got, want) {
			t.Fatalf("ListByLocation container = %v, want %v", got, want)
		}
	})

	t.Run("ListByLocationUnknown", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		_, err := repos.Items.ListByLocation(ctx, inventory.RoomLocation(4242))
		requiresError(t, err, inventory.ErrNotFound, "ListByLocation unknown room")

		_, err = repos.Items.ListByLocation(ctx, inventory.ContainerLocation(4242))
		requiresError(t, err, inventory.ErrNotFound, "ListByLocation unknown container")
	})

	t.Run("ListByLocationInvalid", func(t *testing.T) {
		repos := newRepos(t)
		_, err := repos.Items.ListByLocation(t.Context(), inventory.Location{Kind: "shelf", ID: 1})
		requiresError(t, err, inventory.ErrValidation, "ListByLocation with an invalid location")
	})

	t.Run("UpdateChangesFieldsAndLocation", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))

		item.Name = "  Hammer drill  "
		item.Quantity = 3
		item.Notes = "  in the drawer  "
		item.Location = inventory.ContainerLocation(container.ID)
		requiresNoError(t, repos.Items.Update(ctx, &item), "Update")
		if item.Name != "Hammer drill" || item.Notes != "in the drawer" {
			t.Fatalf("Update left %q / %q, want trimmed values", item.Name, item.Notes)
		}

		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if !reflect.DeepEqual(got, item) {
			t.Fatalf("Get = %+v, want %+v", got, item)
		}

		inRoom, err := repos.Items.ListByLocation(ctx, inventory.RoomLocation(room.ID))
		requiresNoError(t, err, "ListByLocation room")
		if len(inRoom) != 0 {
			t.Fatalf("item stayed in the room: %+v", inRoom)
		}
	})

	t.Run("UpdateUnknownItem", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		item := inventory.Item{ID: 4242, Name: "Ghost", Quantity: 1, Location: inventory.RoomLocation(room.ID)}
		err := repos.Items.Update(t.Context(), &item)
		requiresError(t, err, inventory.ErrNotFound, "Update")
	})

	t.Run("UpdateRejectsInvalidInput", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))

		item.Quantity = -1
		err := repos.Items.Update(t.Context(), &item)
		requiresError(t, err, inventory.ErrValidation, "Update with negative quantity")
		requiresValidationProblem(t, err, "quantity")
	})

	t.Run("DeleteRemovesItem", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))

		requiresNoError(t, repos.Items.Delete(ctx, item.ID), "Delete")
		_, err := repos.Items.Get(ctx, item.ID)
		requiresError(t, err, inventory.ErrNotFound, "Get after Delete")
		requiresError(t, repos.Items.Delete(ctx, item.ID), inventory.ErrNotFound, "second Delete")
	})
}

// itemNames extracts the names of items in listing order.
func itemNames(items []inventory.Item) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	return names
}
