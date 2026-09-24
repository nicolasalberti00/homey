package tools

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
	"github.com/nicolasalberti00/homey/internal/storage"
)

// fixture is a small home in a real database, with the registry of its tools
// and the ids of the items the tests reach for.
type fixture struct {
	registry *Registry
	ids      map[string]inventory.ItemID
}

// newFixture builds a home with two rooms, a nested toolbox and two drills
// with the same name in different places: the shape that makes a search answer
// with candidates instead of one item.
func newFixture(t *testing.T) fixture {
	t.Helper()
	dbPath := t.TempDir() + "/homey.db"
	if err := storage.MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repos := storage.NewRepos(db)
	ctx := t.Context()

	garage := inventory.Room{Name: "Garage"}
	kitchen := inventory.Room{Name: "Cucina"}
	for _, room := range []*inventory.Room{&garage, &kitchen} {
		if err := repos.Rooms.Create(ctx, room); err != nil {
			t.Fatalf("creating room: %v", err)
		}
	}
	toolbox := inventory.Container{RoomID: garage.ID, Name: "Toolbox"}
	if err := repos.Containers.Create(ctx, &toolbox); err != nil {
		t.Fatalf("creating container: %v", err)
	}
	drawer := inventory.Container{RoomID: garage.ID, ParentID: &toolbox.ID, Name: "Cassetto 1"}
	if err := repos.Containers.Create(ctx, &drawer); err != nil {
		t.Fatalf("creating container: %v", err)
	}
	// A second toolbox in another room, so a bare "Toolbox" names two places.
	kitchenToolbox := inventory.Container{RoomID: kitchen.ID, Name: "Toolbox"}
	if err := repos.Containers.Create(ctx, &kitchenToolbox); err != nil {
		t.Fatalf("creating container: %v", err)
	}

	items := []struct {
		slug string
		item inventory.Item
	}{
		{"drill in the drawer", inventory.Item{
			Name: "Trapano", Description: "Bosch blu", Quantity: 1, Notes: "regalo di papà",
			Tags: []string{"officina"}, Aliases: []string{"avvitatore"},
			Location: inventory.ContainerLocation(drawer.ID),
		}},
		{"drill in the kitchen", inventory.Item{
			Name: "Trapano", Quantity: 1, Location: inventory.RoomLocation(kitchen.ID),
		}},
		{"bits", inventory.Item{
			Name: "Punte", Quantity: 12, Tags: []string{"officina", "trapano"},
			Location: inventory.ContainerLocation(toolbox.ID),
		}},
		{"screwdriver", inventory.Item{
			Name: "Cacciavite", Quantity: 3, Tags: []string{"officina"}, Aliases: []string{"giravite"},
			Location: inventory.RoomLocation(garage.ID),
		}},
		{"moka", inventory.Item{
			Name: "Caffettiera", Quantity: 1, Location: inventory.RoomLocation(kitchen.ID),
		}},
	}
	ids := make(map[string]inventory.ItemID, len(items))
	for _, entry := range items {
		item := entry.item
		if err := repos.Items.Create(ctx, &item); err != nil {
			t.Fatalf("creating %s: %v", entry.slug, err)
		}
		ids[entry.slug] = item.ID
	}

	list, err := Inventory{Rooms: repos.Rooms, Containers: repos.Containers, Items: repos.Items}.Tools()
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	registry, err := NewRegistry(list...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return fixture{registry: registry, ids: ids}
}

// call runs one tool of the registry and returns what it answered.
func (f fixture) call(t *testing.T, name, input string) string {
	t.Helper()
	tool, found := f.registry.Lookup(name)
	if !found {
		t.Fatalf("%s is not registered", name)
	}
	output, err := tool.Call(t.Context(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s(%s): %v", name, input, err)
	}
	return string(output)
}

// TestSearchInventoryAnswersWhereThingsAre is the question the tools exist
// for: "dov'è il trapano?" has to come back with the place, and with both
// candidates when there are two.
func TestSearchInventoryAnswersWhereThingsAre(t *testing.T) {
	f := newFixture(t)

	var found SearchInventoryOutput
	if err := json.Unmarshal([]byte(f.call(t, "search_inventory", `{"query":"trapano"}`)), &found); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if found.Count != 3 {
		t.Fatalf("count = %d, want the two drills and the bits", found.Count)
	}
	want := []struct {
		name     string
		location string
		field    string
		kind     string
	}{
		{"Trapano", "Garage > Toolbox > Cassetto 1", "name", "exact"},
		{"Trapano", "Cucina", "name", "exact"},
		{"Punte", "Garage > Toolbox", "tag", "exact"},
	}
	if len(found.Results) != len(want) {
		t.Fatalf("results = %d, want %d: %+v", len(found.Results), len(want), found.Results)
	}
	for index, expected := range want {
		got := found.Results[index]
		if got.Name != expected.name || got.Location != expected.location {
			t.Fatalf("results[%d] = %s in %s, want %s in %s", index, got.Name, got.Location, expected.name, expected.location)
		}
		if got.Match.Field != expected.field || got.Match.Kind != expected.kind {
			t.Fatalf("results[%d] match = %s/%s, want %s/%s", index, got.Match.Field, got.Match.Kind, expected.field, expected.kind)
		}
	}
	// The item comes whole: a model should not have to ask again for the
	// quantity or the tags.
	drill := found.Results[0]
	if drill.Quantity != 1 || !slices.Contains(drill.Tags, "officina") || !slices.Contains(drill.Aliases, "avvitatore") {
		t.Fatalf("the drill came back thin: %+v", drill.ItemView)
	}
	if drill.LocationKind != inventory.LocationContainer || drill.LocationID == 0 {
		t.Fatalf("the drill lost its location: %+v", drill.ItemView)
	}
	if found.Truncated {
		t.Fatal("three results were reported as truncated")
	}
}

func TestSearchInventoryMatchesAliasesAndDescriptions(t *testing.T) {
	f := newFixture(t)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"by alias", `{"query":"giravite"}`, "Cacciavite"},
		{"by description", `{"query":"bosch"}`, "Trapano"},
		{"by another word of the description", `{"query":"blu"}`, "Trapano"},
		{"ignoring accents and case", `{"query":"CAFFETTIERA"}`, "Caffettiera"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var found SearchInventoryOutput
			if err := json.Unmarshal([]byte(f.call(t, "search_inventory", tc.input)), &found); err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if found.Count != 1 || len(found.Results) != 1 || found.Results[0].Name != tc.want {
				t.Fatalf("%s = %+v, want %s", tc.input, found, tc.want)
			}
		})
	}
}

func TestSearchInventoryLimitsAndEmptyResults(t *testing.T) {
	f := newFixture(t)

	var limited SearchInventoryOutput
	if err := json.Unmarshal([]byte(f.call(t, "search_inventory", `{"query":"trapano","limit":1}`)), &limited); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if limited.Count != 3 || len(limited.Results) != 1 || !limited.Truncated {
		t.Fatalf("limited = %+v, want one of three and truncated", limited)
	}

	var none SearchInventoryOutput
	if err := json.Unmarshal([]byte(f.call(t, "search_inventory", `{"query":"elicottero"}`)), &none); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if none.Count != 0 || len(none.Results) != 0 {
		t.Fatalf("none = %+v, want no results", none)
	}
	if got := f.call(t, "search_inventory", `{"query":"elicottero"}`); got != `{"query":"elicottero","count":0,"results":[]}` {
		t.Fatalf("empty answer = %s, want an empty result list rather than null", got)
	}
}

func TestGetItemReturnsEverything(t *testing.T) {
	f := newFixture(t)

	var item ItemView
	input := `{"id":` + itoa(int64(f.ids["drill in the drawer"])) + `}`
	if err := json.Unmarshal([]byte(f.call(t, "get_item", input)), &item); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if item.Name != "Trapano" || item.Quantity != 1 || item.Notes != "regalo di papà" {
		t.Fatalf("item = %+v, want the whole drill", item)
	}
	if item.Location != "Garage > Toolbox > Cassetto 1" {
		t.Fatalf("location = %q, want the path", item.Location)
	}
}

func TestGetItemReportsAMissingItem(t *testing.T) {
	f := newFixture(t)

	tool, _ := f.registry.Lookup("get_item")
	_, err := tool.Call(t.Context(), json.RawMessage(`{"id":4242}`))
	if !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("Call = %v, want ErrNotFound", err)
	}
	if !CallerError(err) {
		t.Fatal("a missing item is not reported as something the caller can act on")
	}
}

func TestListLocationWalksTheHome(t *testing.T) {
	f := newFixture(t)

	t.Run("without a location it lists the rooms", func(t *testing.T) {
		var listed ListLocationOutput
		if err := json.Unmarshal([]byte(f.call(t, "list_location", `{}`)), &listed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if listed.Location != "Home" {
			t.Fatalf("location = %q, want Home", listed.Location)
		}
		if names := roomNames(listed.Rooms); !slices.Equal(names, []string{"Cucina", "Garage"}) {
			t.Fatalf("rooms = %v, want the two rooms", names)
		}
	})

	t.Run("a room lists its containers and its own items", func(t *testing.T) {
		var listed ListLocationOutput
		if err := json.Unmarshal([]byte(f.call(t, "list_location", `{"location":"Garage"}`)), &listed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if listed.Location != "Garage" {
			t.Fatalf("location = %q, want Garage", listed.Location)
		}
		if len(listed.Containers) != 1 || listed.Containers[0].Path != "Garage > Toolbox" {
			t.Fatalf("containers = %+v, want the toolbox", listed.Containers)
		}
		// Only what sits directly in the room: the drill in the drawer is
		// one level deeper.
		if len(listed.Items) != 1 || listed.Items[0].Name != "Cacciavite" {
			t.Fatalf("items = %+v, want only the screwdriver", listed.Items)
		}
	})

	t.Run("a nested container, named loosely", func(t *testing.T) {
		var listed ListLocationOutput
		if err := json.Unmarshal([]byte(f.call(t, "list_location", `{"location":"garage>toolbox"}`)), &listed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if listed.Location != "Garage > Toolbox" {
			t.Fatalf("location = %q, want the toolbox", listed.Location)
		}
		if len(listed.Containers) != 1 || listed.Containers[0].Name != "Cassetto 1" {
			t.Fatalf("containers = %+v, want the drawer", listed.Containers)
		}
		if len(listed.Items) != 1 || listed.Items[0].Location != "Garage > Toolbox" {
			t.Fatalf("items = %+v, want the bits", listed.Items)
		}
	})

	t.Run("a container by its own name", func(t *testing.T) {
		var listed ListLocationOutput
		if err := json.Unmarshal([]byte(f.call(t, "list_location", `{"location":"Cassetto 1"}`)), &listed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(listed.Items) != 1 || listed.Items[0].Name != "Trapano" {
			t.Fatalf("items = %+v, want the drill", listed.Items)
		}
	})
}

func TestListLocationReportsWhatItCannotResolve(t *testing.T) {
	f := newFixture(t)
	tool, _ := f.registry.Lookup("list_location")

	_, err := tool.Call(t.Context(), json.RawMessage(`{"location":"Giardino"}`))
	if !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown place = %v, want ErrNotFound", err)
	}

	_, err = tool.Call(t.Context(), json.RawMessage(`{"location":"Toolbox"}`))
	if !errors.Is(err, inventory.ErrAmbiguous) {
		t.Fatalf("a shared name = %v, want ErrAmbiguous", err)
	}
	// The message names the candidates, so a model can ask which one.
	var ambiguous *inventory.AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("ambiguous = %+v, want the candidates", ambiguous)
	}
	want := []string{"Garage > Toolbox", "Cucina > Toolbox"}
	if !slices.Equal(ambiguous.Candidates, want) {
		t.Fatalf("candidates = %v, want %v", ambiguous.Candidates, want)
	}
}

func TestCountItems(t *testing.T) {
	f := newFixture(t)
	tool, found := f.registry.Lookup("count_items")
	if !found {
		t.Fatal("count_items is not registered")
	}
	if tool.Permission() != PermissionRead || tool.Destructive() || tool.RequiresConfirmation() {
		t.Fatalf("count_items is described as %s/destructive=%t/confirm=%t, want a plain read",
			tool.Permission(), tool.Destructive(), tool.RequiresConfirmation())
	}

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"everything", `{}`, `{"count":5}`},
		{"by tag", `{"tag":"officina"}`, `{"count":3}`},
		{"by tag, ignoring case", `{"tag":"OFFICINA"}`, `{"count":3}`},
		{"a tag nobody has", `{"tag":"cantina"}`, `{"count":0}`},
		// A place counts what it holds and what its containers hold: the
		// garage counts the screwdriver, the bits in the toolbox and the
		// drill in the drawer.
		{"by room", `{"location":"Garage"}`, `{"count":3}`},
		{"by room without containers", `{"location":"Cucina"}`, `{"count":2}`},
		{"by container", `{"location":"Garage > Toolbox"}`, `{"count":2}`},
		{"by nested container", `{"location":"Cassetto 1"}`, `{"count":1}`},
		{"by tag and place", `{"tag":"officina","location":"Garage"}`, `{"count":3}`},
		{"a tag the place does not hold", `{"tag":"officina","location":"Cucina"}`, `{"count":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.call(t, "count_items", tc.input); got != tc.want {
				t.Fatalf("count_items(%s) = %s, want %s", tc.input, got, tc.want)
			}
		})
	}
}

// TestReadToolsArePlainReads checks the annotations a host uses to decide
// whether a tool may run without asking.
func TestReadToolsArePlainReads(t *testing.T) {
	f := newFixture(t)

	for _, tool := range f.registry.List() {
		if tool.Permission() != PermissionRead {
			t.Fatalf("%s needs %s, want read", tool.Name(), tool.Permission())
		}
		if tool.Destructive() || tool.RequiresConfirmation() {
			t.Fatalf("%s is destructive=%t/confirm=%t, want a plain read", tool.Name(), tool.Destructive(), tool.RequiresConfirmation())
		}
		if tool.InputSchema() == nil || tool.OutputSchema() == nil {
			t.Fatalf("%s came without a schema", tool.Name())
		}
	}
}

// roomNames collects the names of a room listing.
func roomNames(rooms []RoomView) []string {
	names := make([]string, len(rooms))
	for index, room := range rooms {
		names[index] = room.Name
	}
	return names
}

// itoa renders an id for a tool call.
func itoa(id int64) string {
	encoded, _ := json.Marshal(id)
	return string(encoded)
}
