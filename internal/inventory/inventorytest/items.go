package inventorytest

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
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

	t.Run("CreateRejectsDuplicateNameInLocation", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")
		createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))
		createItem(t, repos, "Screws", inventory.ContainerLocation(container.ID))

		inRoom := inventory.Item{Name: "drill", Quantity: 1, Location: inventory.RoomLocation(room.ID)}
		requiresError(t, repos.Items.Create(ctx, &inRoom), inventory.ErrConflict, "Create duplicate name in a room")

		inContainer := inventory.Item{Name: "SCREWS", Quantity: 1, Location: inventory.ContainerLocation(container.ID)}
		requiresError(t, repos.Items.Create(ctx, &inContainer), inventory.ErrConflict, "Create duplicate name in a container")
	})

	t.Run("CreateAllowsSameNameInDifferentLocations", func(t *testing.T) {
		repos := newRepos(t)
		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		toolbox := createContainer(t, repos, garage, nil, "Toolbox")
		shelf := createContainer(t, repos, kitchen, nil, "Shelf")

		createItem(t, repos, "Cable", inventory.RoomLocation(garage.ID))
		createItem(t, repos, "Cable", inventory.RoomLocation(kitchen.ID))
		createItem(t, repos, "Cable", inventory.ContainerLocation(toolbox.ID))
		createItem(t, repos, "Cable", inventory.ContainerLocation(shelf.ID))
	})

	t.Run("CreatePersistsTags", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := inventory.Item{
			Name:     "Drill",
			Quantity: 1,
			Location: inventory.RoomLocation(room.ID),
			Tags:     []string{"  Strumenti  ", "bagno"},
		}
		requiresNoError(t, repos.Items.Create(ctx, &item), "Create")

		want := []string{"bagno", "Strumenti"}
		if !slices.Equal(item.Tags, want) {
			t.Fatalf("Create left tags %v, want %v", item.Tags, want)
		}
		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if !slices.Equal(got.Tags, want) {
			t.Fatalf("Get tags = %v, want %v", got.Tags, want)
		}
	})

	t.Run("CreateWithoutTagsReturnsEmpty", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))
		if item.Tags == nil || len(item.Tags) != 0 {
			t.Fatalf("created item tags = %v, want non-nil empty", item.Tags)
		}
	})

	t.Run("CreateRejectsInvalidTags", func(t *testing.T) {
		tooMany := make([]string, inventory.MaxTagsPerItem+1)
		for index := range tooMany {
			tooMany[index] = fmt.Sprintf("tag-%d", index)
		}
		cases := []struct {
			name string
			tags []string
		}{
			{"empty tag", []string{"bagno", "   "}},
			{"tag above maximum length", []string{strings.Repeat("a", inventory.MaxTagLen+1)}},
			{"case-insensitive duplicate", []string{"Bagno", "bagno"}},
			{"too many tags", tooMany},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repos := newRepos(t)
				room := createRoom(t, repos, "Garage")
				item := inventory.Item{
					Name:     "Drill",
					Quantity: 1,
					Location: inventory.RoomLocation(room.ID),
					Tags:     tc.tags,
				}
				err := repos.Items.Create(t.Context(), &item)
				requiresError(t, err, inventory.ErrValidation, "Create")
				requiresValidationProblem(t, err, "tags")
			})
		}
	})

	t.Run("SameTagOnDifferentItemsIsAllowed", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")

		first := inventory.Item{Name: "Drill", Quantity: 1, Location: inventory.RoomLocation(room.ID), Tags: []string{"Strumenti"}}
		second := inventory.Item{Name: "Saw", Quantity: 1, Location: inventory.RoomLocation(room.ID), Tags: []string{"strumenti"}}
		requiresNoError(t, repos.Items.Create(t.Context(), &first), "Create first")
		requiresNoError(t, repos.Items.Create(t.Context(), &second), "Create second")
	})

	t.Run("UpdateReplacesTags", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))

		item.Tags = []string{"Strumenti", "Officina"}
		requiresNoError(t, repos.Items.Update(ctx, &item), "Update with tags")
		if want := []string{"Officina", "Strumenti"}; !slices.Equal(item.Tags, want) {
			t.Fatalf("Update left tags %v, want %v", item.Tags, want)
		}

		item.Tags = []string{"Bagno"}
		requiresNoError(t, repos.Items.Update(ctx, &item), "Update with one tag")
		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if want := []string{"Bagno"}; !slices.Equal(got.Tags, want) {
			t.Fatalf("Get tags = %v, want %v", got.Tags, want)
		}
	})

	t.Run("ListIncludesTags", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		item := inventory.Item{
			Name:     "Drill",
			Quantity: 1,
			Location: inventory.RoomLocation(room.ID),
			Tags:     []string{"Strumenti"},
		}
		requiresNoError(t, repos.Items.Create(t.Context(), &item), "Create")

		items, err := repos.Items.List(t.Context())
		requiresNoError(t, err, "List")
		if len(items) != 1 || !slices.Equal(items[0].Tags, []string{"Strumenti"}) {
			t.Fatalf("List = %+v, want one item tagged Strumenti", items)
		}
	})

	t.Run("CreatePersistsAliases", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := inventory.Item{
			Name:     "Cacciavite",
			Quantity: 1,
			Location: inventory.RoomLocation(room.ID),
			Aliases:  []string{"  Giravite  ", "cacciavite a stella"},
		}
		requiresNoError(t, repos.Items.Create(ctx, &item), "Create")

		want := []string{"cacciavite a stella", "Giravite"}
		if !slices.Equal(item.Aliases, want) {
			t.Fatalf("Create left aliases %v, want %v", item.Aliases, want)
		}
		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if !slices.Equal(got.Aliases, want) {
			t.Fatalf("Get aliases = %v, want %v", got.Aliases, want)
		}
	})

	t.Run("CreateWithoutAliasesReturnsEmpty", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))
		if item.Aliases == nil || len(item.Aliases) != 0 {
			t.Fatalf("created item aliases = %v, want non-nil empty", item.Aliases)
		}
	})

	t.Run("CreateRejectsInvalidAliases", func(t *testing.T) {
		tooMany := make([]string, inventory.MaxAliasesPerItem+1)
		for index := range tooMany {
			tooMany[index] = fmt.Sprintf("alias-%d", index)
		}
		cases := []struct {
			name    string
			aliases []string
		}{
			{"empty alias", []string{"giravite", "   "}},
			{"alias above maximum length", []string{strings.Repeat("a", inventory.MaxNameLen+1)}},
			{"case-insensitive duplicate", []string{"Giravite", "giravite"}},
			{"too many aliases", tooMany},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repos := newRepos(t)
				room := createRoom(t, repos, "Garage")
				item := inventory.Item{
					Name:     "Cacciavite",
					Quantity: 1,
					Location: inventory.RoomLocation(room.ID),
					Aliases:  tc.aliases,
				}
				err := repos.Items.Create(t.Context(), &item)
				requiresError(t, err, inventory.ErrValidation, "Create")
				requiresValidationProblem(t, err, "aliases")
			})
		}
	})

	t.Run("SameAliasOnDifferentItemsIsAllowed", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")

		first := inventory.Item{Name: "Cacciavite", Quantity: 1, Location: inventory.RoomLocation(room.ID), Aliases: []string{"Giravite"}}
		second := inventory.Item{Name: "Cacciavite piatto", Quantity: 1, Location: inventory.RoomLocation(room.ID), Aliases: []string{"giravite"}}
		requiresNoError(t, repos.Items.Create(t.Context(), &first), "Create first")
		requiresNoError(t, repos.Items.Create(t.Context(), &second), "Create second")
	})

	t.Run("UpdateReplacesAliases", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Cacciavite", inventory.RoomLocation(room.ID))

		item.Aliases = []string{"Giravite", "Cacciavite a stella"}
		requiresNoError(t, repos.Items.Update(ctx, &item), "Update with aliases")
		if want := []string{"Cacciavite a stella", "Giravite"}; !slices.Equal(item.Aliases, want) {
			t.Fatalf("Update left aliases %v, want %v", item.Aliases, want)
		}

		item.Aliases = []string{"Giravite"}
		requiresNoError(t, repos.Items.Update(ctx, &item), "Update with one alias")
		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if want := []string{"Giravite"}; !slices.Equal(got.Aliases, want) {
			t.Fatalf("Get aliases = %v, want %v", got.Aliases, want)
		}
	})

	t.Run("ListIncludesAliases", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		item := inventory.Item{
			Name:     "Cacciavite",
			Quantity: 1,
			Location: inventory.RoomLocation(room.ID),
			Aliases:  []string{"Giravite"},
		}
		requiresNoError(t, repos.Items.Create(t.Context(), &item), "Create")

		items, err := repos.Items.List(t.Context())
		requiresNoError(t, err, "List")
		if len(items) != 1 || !slices.Equal(items[0].Aliases, []string{"Giravite"}) {
			t.Fatalf("List = %+v, want one item aliased Giravite", items)
		}
	})

	t.Run("MoveToAnotherRoom", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		item := inventory.Item{
			Name:     "Drill",
			Quantity: 2,
			Notes:    "cordless",
			Location: inventory.RoomLocation(garage.ID),
			Tags:     []string{"Strumenti"},
		}
		requiresNoError(t, repos.Items.Create(ctx, &item), "Create")

		requiresNoError(t, repos.Items.Move(ctx, item.ID, inventory.RoomLocation(kitchen.ID)), "Move")

		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if !got.Location.IsRoom() || got.Location.ID != int64(kitchen.ID) {
			t.Fatalf("location after move = %+v, want room %d", got.Location, kitchen.ID)
		}
		if got.Name != "Drill" || got.Quantity != 2 || got.Notes != "cordless" || !slices.Equal(got.Tags, []string{"Strumenti"}) {
			t.Fatalf("move changed other fields: %+v", got)
		}

		inGarage, err := repos.Items.ListByLocation(ctx, inventory.RoomLocation(garage.ID))
		requiresNoError(t, err, "ListByLocation garage")
		if len(inGarage) != 0 {
			t.Fatalf("item stayed in the old room: %+v", inGarage)
		}
		inKitchen, err := repos.Items.ListByLocation(ctx, inventory.RoomLocation(kitchen.ID))
		requiresNoError(t, err, "ListByLocation kitchen")
		if len(inKitchen) != 1 {
			t.Fatalf("item missing from the new room: %+v", inKitchen)
		}
	})

	t.Run("MoveIntoContainer", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		toolbox := createContainer(t, repos, room, nil, "Toolbox")
		item := createItem(t, repos, "Screwdriver", inventory.RoomLocation(room.ID))

		requiresNoError(t, repos.Items.Move(ctx, item.ID, inventory.ContainerLocation(toolbox.ID)), "Move")

		got, err := repos.Items.Get(ctx, item.ID)
		requiresNoError(t, err, "Get")
		if !got.Location.IsContainer() || got.Location.ID != int64(toolbox.ID) {
			t.Fatalf("location after move = %+v, want container %d", got.Location, toolbox.ID)
		}
	})

	t.Run("MoveRejectsUnknownDestination", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))

		requiresError(t, repos.Items.Move(ctx, item.ID, inventory.RoomLocation(4242)), inventory.ErrNotFound, "Move to unknown room")
		requiresError(t, repos.Items.Move(ctx, item.ID, inventory.ContainerLocation(4242)), inventory.ErrNotFound, "Move to unknown container")
	})

	t.Run("MoveUnknownItem", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		requiresError(t, repos.Items.Move(t.Context(), 4242, inventory.RoomLocation(room.ID)), inventory.ErrNotFound, "Move")
	})

	t.Run("MoveRejectsNameCollisionAtDestination", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		createItem(t, repos, "Cable", inventory.RoomLocation(kitchen.ID))
		item := createItem(t, repos, "Cable", inventory.RoomLocation(garage.ID))

		requiresError(t, repos.Items.Move(ctx, item.ID, inventory.RoomLocation(kitchen.ID)), inventory.ErrConflict, "Move onto a duplicate name")
	})

	t.Run("MoveRejectsInvalidDestination", func(t *testing.T) {
		repos := newRepos(t)
		room := createRoom(t, repos, "Garage")
		item := createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))
		err := repos.Items.Move(t.Context(), item.ID, inventory.Location{Kind: "shelf", ID: 1})
		requiresError(t, err, inventory.ErrValidation, "Move with an invalid destination")
	})

	t.Run("UpdateRejectsDuplicateNameInLocation", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		garage := createRoom(t, repos, "Garage")
		kitchen := createRoom(t, repos, "Kitchen")
		createItem(t, repos, "Cable", inventory.RoomLocation(garage.ID))
		item := createItem(t, repos, "Cable", inventory.RoomLocation(kitchen.ID))

		item.Location = inventory.RoomLocation(garage.ID)
		requiresError(t, repos.Items.Update(ctx, &item), inventory.ErrConflict, "Move onto a duplicate name")
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

	t.Run("SearchMatchesNameAndDescription", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		container := createContainer(t, repos, room, nil, "Toolbox")
		drill := inventory.Item{
			Name:        "Bosch drill",
			Description: "cordless",
			Quantity:    1,
			Location:    inventory.ContainerLocation(container.ID),
		}
		requiresNoError(t, repos.Items.Create(ctx, &drill), "creating item")
		createItem(t, repos, "Hammer", inventory.RoomLocation(room.ID))

		byName, err := repos.Items.Search(ctx, "drill")
		requiresNoError(t, err, "Search by name")
		if names := itemNames(byName); !slices.Equal(names, []string{"Bosch drill"}) {
			t.Fatalf("Search by name = %v, want [Bosch drill]", names)
		}

		// The description is searched as well, and the match spans every
		// location, containers included.
		byDescription, err := repos.Items.Search(ctx, "cordless")
		requiresNoError(t, err, "Search by description")
		if len(byDescription) != 1 || byDescription[0].Location != drill.Location {
			t.Fatalf("Search by description = %+v, want the drill in its container", byDescription)
		}
	})

	t.Run("SearchIgnoresCase", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		createItem(t, repos, "Cordless Drill", inventory.RoomLocation(room.ID))

		for _, query := range []string{"drill", "DRILL", "DrIlL", "  drill  "} {
			matches, err := repos.Items.Search(ctx, query)
			requiresNoError(t, err, "Search "+query)
			if names := itemNames(matches); !slices.Equal(names, []string{"Cordless Drill"}) {
				t.Fatalf("Search(%q) = %v, want [Cordless Drill]", query, names)
			}
		}
	})

	t.Run("SearchIsOrderedByName", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		location := inventory.RoomLocation(room.ID)
		createItem(t, repos, "drill battery", location)
		createItem(t, repos, "Drill", location)
		createItem(t, repos, "Hammer drill", location)

		matches, err := repos.Items.Search(ctx, "drill")
		requiresNoError(t, err, "Search")
		want := []string{"Drill", "drill battery", "Hammer drill"}
		if names := itemNames(matches); !slices.Equal(names, want) {
			t.Fatalf("Search = %v, want %v", names, want)
		}
	})

	t.Run("SearchEscapesWildcards", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		location := inventory.RoomLocation(room.ID)
		createItem(t, repos, "Cable 50% copper", location)
		createItem(t, repos, "Screws", location)

		// The query is data, not a LIKE pattern: "%" matches a literal one.
		matches, err := repos.Items.Search(ctx, "50%")
		requiresNoError(t, err, "Search 50%")
		if names := itemNames(matches); !slices.Equal(names, []string{"Cable 50% copper"}) {
			t.Fatalf("Search(50%%) = %v, want [Cable 50%% copper]", names)
		}

		matches, err = repos.Items.Search(ctx, "_")
		requiresNoError(t, err, "Search _")
		if len(matches) != 0 {
			t.Fatalf("Search(_) = %v, want no matches", itemNames(matches))
		}
	})

	t.Run("SearchReturnsTags", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := inventory.Item{
			Name:     "Drill",
			Quantity: 1,
			Tags:     []string{"Strumenti", "bagno"},
			Location: inventory.RoomLocation(room.ID),
		}
		requiresNoError(t, repos.Items.Create(ctx, &item), "creating item")

		matches, err := repos.Items.Search(ctx, "drill")
		requiresNoError(t, err, "Search")
		// Tags come back in case-insensitive order, like every listing.
		if len(matches) != 1 || !slices.Equal(matches[0].Tags, []string{"bagno", "Strumenti"}) {
			t.Fatalf("Search = %+v, want the item carrying its tags", matches)
		}
	})

	t.Run("SearchMatchesAliasesAndTags", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		location := inventory.RoomLocation(room.ID)
		screwdriver := inventory.Item{
			Name:     "Cacciavite",
			Quantity: 1,
			Aliases:  []string{"giravite"},
			Tags:     []string{"strumenti"},
			Location: location,
		}
		requiresNoError(t, repos.Items.Create(ctx, &screwdriver), "creating item")
		createItem(t, repos, "Hammer", location)

		byAlias, err := repos.Items.Search(ctx, "giravite")
		requiresNoError(t, err, "Search by alias")
		if names := itemNames(byAlias); !slices.Equal(names, []string{"Cacciavite"}) {
			t.Fatalf("Search by alias = %v, want [Cacciavite]", names)
		}

		byTag, err := repos.Items.Search(ctx, "strumenti")
		requiresNoError(t, err, "Search by tag")
		if names := itemNames(byTag); !slices.Equal(names, []string{"Cacciavite"}) {
			t.Fatalf("Search by tag = %v, want [Cacciavite]", names)
		}

		// The match carries the labels that matched, like every other read.
		if !slices.Equal(byAlias[0].Aliases, []string{"giravite"}) || !slices.Equal(byAlias[0].Tags, []string{"strumenti"}) {
			t.Fatalf("Search result = %+v, want its aliases and tags", byAlias[0])
		}
	})

	t.Run("SearchIgnoresNotes", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		item := inventory.Item{
			Name:     "Drill",
			Quantity: 1,
			Notes:    "battery in the charger",
			Location: inventory.RoomLocation(room.ID),
		}
		requiresNoError(t, repos.Items.Create(ctx, &item), "creating item")

		matches, err := repos.Items.Search(ctx, "battery")
		requiresNoError(t, err, "Search notes")
		if len(matches) != 0 {
			t.Fatalf("Search = %v, want no matches: notes are not searched", itemNames(matches))
		}
	})

	t.Run("SearchWithBlankQueryMatchesNothing", func(t *testing.T) {
		repos := newRepos(t)
		ctx := t.Context()

		room := createRoom(t, repos, "Garage")
		createItem(t, repos, "Drill", inventory.RoomLocation(room.ID))

		for _, query := range []string{"", "   ", "\t\n"} {
			matches, err := repos.Items.Search(ctx, query)
			requiresNoError(t, err, "Search blank query")
			if matches == nil {
				t.Fatalf("Search(%q) returned nil, want an empty slice", query)
			}
			if len(matches) != 0 {
				t.Fatalf("Search(%q) = %v, want no matches", query, itemNames(matches))
			}
		}
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
