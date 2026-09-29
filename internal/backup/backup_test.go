package backup

// The backup document: an export that carries the whole inventory in a
// portable, name-based document, and an import that validates it and applies
// it idempotently. The tests run against real repositories, so the round trip
// goes through the same SQL a fresh instance would.

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nicolasalberti00/homey/internal/inventory"
	"github.com/nicolasalberti00/homey/internal/storage"
)

// openFixture builds an empty instance and returns the repositories an export
// and an import work through.
func openFixture(t *testing.T) Repos {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "homey.db")
	if err := storage.MigrateUp(dbPath); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repos := storage.NewRepos(db)
	return Repos{Rooms: repos.Rooms, Containers: repos.Containers, Items: repos.Items}
}

// seed builds a small home whose shape exercises every reference the document
// has: a nested container, a container name that repeats in another room (so a
// name alone is not enough to point at one), and items with tags and aliases.
func seed(t *testing.T, r Repos) {
	t.Helper()
	ctx := t.Context()

	garage := inventory.Room{Name: "Garage", Description: "dove parcheggio"}
	cucina := inventory.Room{Name: "Cucina"}
	for _, room := range []*inventory.Room{&garage, &cucina} {
		if err := r.Rooms.Create(ctx, room); err != nil {
			t.Fatalf("creating room %q: %v", room.Name, err)
		}
	}

	toolbox := inventory.Container{RoomID: garage.ID, Name: "Toolbox", Description: "cassetta"}
	if err := r.Containers.Create(ctx, &toolbox); err != nil {
		t.Fatalf("creating toolbox: %v", err)
	}
	drawer := inventory.Container{RoomID: garage.ID, ParentID: &toolbox.ID, Name: "Drawer"}
	if err := r.Containers.Create(ctx, &drawer); err != nil {
		t.Fatalf("creating drawer: %v", err)
	}
	// The same container name in the other room: a reference must carry the
	// room, or two toolboxes would be one.
	secondToolbox := inventory.Container{RoomID: cucina.ID, Name: "Toolbox"}
	if err := r.Containers.Create(ctx, &secondToolbox); err != nil {
		t.Fatalf("creating the kitchen toolbox: %v", err)
	}

	items := []inventory.Item{
		{
			Name: "Trapano", Description: "Bosch blu", Quantity: 1, Notes: "regalo di papà",
			Tags: []string{"officina"}, Aliases: []string{"avvitatore"},
			Location: inventory.RoomLocation(garage.ID),
		},
		{
			Name: "Cavo rotto", Quantity: 3,
			Location: inventory.ContainerLocation(drawer.ID),
		},
		{
			Name: "Moka", Quantity: 2, Tags: []string{"caffè"}, Aliases: []string{"caffettiera"},
			Location: inventory.RoomLocation(cucina.ID),
		},
	}
	for index := range items {
		if err := r.Items.Create(ctx, &items[index]); err != nil {
			t.Fatalf("creating item %q: %v", items[index].Name, err)
		}
	}
}

// TestExportCarriesTheWholeInventory checks the document says everything a
// fresh instance needs, in an order that does not depend on internal ids.
func TestExportCarriesTheWholeInventory(t *testing.T) {
	r := openFixture(t)
	seed(t, r)

	doc, err := Export(t.Context(), r)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if doc.Format != Format || doc.Version != Version {
		t.Fatalf("format/version = %q/%d, want %q/%d", doc.Format, doc.Version, Format, Version)
	}
	if doc.ExportedAt.IsZero() {
		t.Fatal("the document says no export time")
	}

	if got, want := roomNames(doc), []string{"Cucina", "Garage"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rooms = %v, want %v", got, want)
	}
	if doc.Rooms[1].Description != "dove parcheggio" {
		t.Errorf("Garage = %+v, want its description", doc.Rooms[1])
	}

	// Parents before children, and the two toolboxes told apart by room.
	wantContainers := []string{"Cucina/Toolbox", "Garage/Toolbox", "Garage/Drawer"}
	if got := containerPaths(doc); !reflect.DeepEqual(got, wantContainers) {
		t.Fatalf("containers = %v, want %v", got, wantContainers)
	}
	if doc.Containers[1].Parent != "" || doc.Containers[2].Parent != "Toolbox" {
		t.Errorf("parents = %q/%q, want the drawer nested under its toolbox",
			doc.Containers[1].Parent, doc.Containers[2].Parent)
	}
	if doc.Containers[1].Description != "cassetta" {
		t.Errorf("Toolbox = %+v, want its description", doc.Containers[1])
	}

	// Items with tags, aliases, notes and the place they live in.
	wantItems := []string{"Cavo rotto", "Moka", "Trapano"}
	if got := itemNames(doc); !reflect.DeepEqual(got, wantItems) {
		t.Fatalf("items = %v, want %v", got, wantItems)
	}
	trapano := doc.Items[2]
	if trapano.Description != "Bosch blu" || trapano.Quantity != 1 || trapano.Notes != "regalo di papà" {
		t.Errorf("Trapano = %+v, want its fields", trapano)
	}
	if !reflect.DeepEqual(trapano.Tags, []string{"officina"}) || !reflect.DeepEqual(trapano.Aliases, []string{"avvitatore"}) {
		t.Errorf("Trapano = %+v, want the tags and the aliases", trapano)
	}
	if trapano.Location != (Location{Kind: "room", Room: "Garage"}) {
		t.Errorf("Trapano location = %+v, want the garage", trapano.Location)
	}

	cavo := doc.Items[0]
	if cavo.Location != (Location{Kind: "container", Room: "Garage", Container: "Drawer"}) {
		t.Errorf("Cavo location = %+v, want Garage > Drawer", cavo.Location)
	}
}

// TestExportIsRepeatable: two exports of the same inventory differ only in the
// timestamp, so a backup diff shows what changed.
func TestExportIsRepeatable(t *testing.T) {
	r := openFixture(t)
	seed(t, r)

	first, err := Export(t.Context(), r)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	second, err := Export(t.Context(), r)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	first.ExportedAt, second.ExportedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("two exports differ:\nfirst  %+v\nsecond %+v", first, second)
	}
}

// TestImportOnAFreshInstanceReproducesTheInventory is the exit criterion of
// the step: what you export here, you can import there, without losses.
func TestImportOnAFreshInstanceReproducesTheInventory(t *testing.T) {
	source := openFixture(t)
	seed(t, source)
	exported, err := Export(t.Context(), source)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	target := openFixture(t)
	report, err := Import(t.Context(), target, exported)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Rooms.Created != 2 || report.Containers.Created != 3 || report.Items.Created != 3 {
		t.Fatalf("report = %+v, want everything created", report)
	}

	restored, err := Export(t.Context(), target)
	if err != nil {
		t.Fatalf("Export after import: %v", err)
	}
	exported.ExportedAt, restored.ExportedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(exported, restored) {
		t.Fatalf("the restored inventory differs:\nexported %+v\nrestored %+v", exported, restored)
	}
}

// TestImportIsIdempotent: running the same document again changes nothing and
// says so — no duplicate rows, no writes for what already stands.
func TestImportIsIdempotent(t *testing.T) {
	source := openFixture(t)
	seed(t, source)
	doc, err := Export(t.Context(), source)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	target := openFixture(t)
	if _, err := Import(t.Context(), target, doc); err != nil {
		t.Fatalf("first Import: %v", err)
	}
	again, err := Import(t.Context(), target, doc)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	want := Report{
		Rooms:      Counts{Unchanged: 2},
		Containers: Counts{Unchanged: 3},
		Items:      Counts{Unchanged: 3},
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("second import report = %+v, want %+v", again, want)
	}

	// And the inventory still holds exactly one of everything.
	restored, err := Export(t.Context(), target)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(restored.Rooms) != 2 || len(restored.Containers) != 3 || len(restored.Items) != 3 {
		t.Fatalf("after a second import: %d rooms, %d containers, %d items, want 2/3/3",
			len(restored.Rooms), len(restored.Containers), len(restored.Items))
	}
}

// TestImportUpdatesWhatChanged: a document is a statement of what should be
// there, so a value that moved gets written and reported.
func TestImportUpdatesWhatChanged(t *testing.T) {
	source := openFixture(t)
	seed(t, source)
	doc, err := Export(t.Context(), source)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// The trpano broke: one more, a note, one alias fewer.
	doc.Items[2].Quantity = 2
	doc.Items[2].Notes = "va cambiato"
	doc.Items[2].Aliases = []string{}
	doc.Rooms[1].Description = "rifatto"

	report, err := Import(t.Context(), source, doc)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	want := Report{
		Rooms:      Counts{Updated: 1, Unchanged: 1},
		Containers: Counts{Unchanged: 3},
		Items:      Counts{Updated: 1, Unchanged: 2},
	}
	if !reflect.DeepEqual(report, want) {
		t.Fatalf("report = %+v, want %+v", report, want)
	}

	updated, err := Export(t.Context(), source)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if updated.Items[2].Quantity != 2 || updated.Items[2].Notes != "va cambiato" {
		t.Errorf("Trapano = %+v, want the change applied", updated.Items[2])
	}
	if len(updated.Items[2].Aliases) != 0 {
		t.Errorf("aliases = %v, want them cleared", updated.Items[2].Aliases)
	}
	if updated.Rooms[1].Description != "rifatto" {
		t.Errorf("Garage = %+v, want the new description", updated.Rooms[1])
	}
}

// TestImportNeverDeletes: an import adds and updates; an entity the document
// does not mention is somebody's data, not an import's to remove.
func TestImportNeverDeletes(t *testing.T) {
	r := openFixture(t)
	seed(t, r)

	partial := Document{
		Format: Format, Version: Version,
		Rooms: []Room{{Name: "Garage"}},
	}

	report, err := Import(t.Context(), r, partial)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Rooms != (Counts{Updated: 1}) {
		t.Fatalf("rooms = %+v, want the garage updated", report.Rooms)
	}

	doc, err := Export(t.Context(), r)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(doc.Rooms) != 2 || len(doc.Containers) != 3 || len(doc.Items) != 3 {
		t.Fatalf("the inventory lost something: %d/%d/%d, want 2/3/3",
			len(doc.Rooms), len(doc.Containers), len(doc.Items))
	}
	if doc.Rooms[1].Description != "" {
		t.Errorf("Garage = %+v, want it cleared by the document", doc.Rooms[1])
	}
}

// TestImportRefusesWhatItCannotApply collects the documents an import must
// turn down, and the way each one is pointed at.
func TestImportRefusesWhatItCannotApply(t *testing.T) {
	cases := []struct {
		name     string
		document func() Document
		field    string
	}{
		{
			name:     "a foreign format",
			document: func() Document { return Document{Format: "someone.else", Version: Version} },
			field:    "format",
		},
		{
			name:     "a version from the future",
			document: func() Document { return Document{Format: Format, Version: 99} },
			field:    "version",
		},
		{
			name: "an empty room name",
			document: func() Document {
				return Document{Format: Format, Version: Version, Rooms: []Room{{Name: "   "}}}
			},
			field: "rooms[0].name",
		},
		{
			name: "the same room twice",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Rooms: []Room{{Name: "Garage"}, {Name: "garage"}}}
			},
			field: "rooms[1].name",
		},
		{
			name: "a container in a room that is not there",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Containers: []Container{{Room: "Attic", Name: "Box"}}}
			},
			field: "containers[0].room",
		},
		{
			name: "a container under a parent from another room",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Rooms: []Room{{Name: "Garage"}, {Name: "Cucina"}},
					Containers: []Container{
						{Room: "Garage", Name: "Toolbox"},
						{Room: "Cucina", Name: "Drawer", Parent: "Toolbox"},
					}}
			},
			field: "containers[1].parent",
		},
		{
			name: "containers in a circle",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Rooms: []Room{{Name: "Garage"}},
					Containers: []Container{
						{Room: "Garage", Name: "A", Parent: "B"},
						{Room: "Garage", Name: "B", Parent: "A"},
					}}
			},
			field: "containers",
		},
		{
			name: "an item pointing at a room that is not there",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Items: []Item{{Name: "Cavo", Location: Location{Kind: "room", Room: "Attic"}}}}
			},
			field: "items[0].location.room",
		},
		{
			name: "an item pointing at a container of another room",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Rooms:      []Room{{Name: "Garage"}, {Name: "Cucina"}},
					Containers: []Container{{Room: "Garage", Name: "Toolbox"}},
					Items: []Item{{Name: "Cavo",
						Location: Location{Kind: "container", Room: "Cucina", Container: "Toolbox"}}}}
			},
			field: "items[0].location.container",
		},
		{
			name: "a location kind that is neither",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Items: []Item{{Name: "Cavo", Location: Location{Kind: "trunk", Room: "Garage"}}}}
			},
			field: "items[0].location.kind",
		},
		{
			name: "a negative quantity",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Rooms: []Room{{Name: "Garage"}},
					Items: []Item{{Name: "Cavo", Quantity: -1,
						Location: Location{Kind: "room", Room: "Garage"}}}}
			},
			field: "items[0].quantity",
		},
		{
			name: "the same item twice at the same place",
			document: func() Document {
				return Document{Format: Format, Version: Version,
					Rooms: []Room{{Name: "Garage"}},
					Items: []Item{
						{Name: "Cavo", Location: Location{Kind: "room", Room: "Garage"}},
						{Name: "cavo", Location: Location{Kind: "room", Room: "Garage"}},
					}}
			},
			field: "items[1].name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := openFixture(t)
			_, err := Import(t.Context(), r, tc.document())
			var validation *inventory.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("Import = %v, want a validation error", err)
			}
			var found bool
			for _, problem := range validation.Problems {
				if problem.Field == tc.field {
					found = true
					break
				}
			}
			if !found {
				var fields []string
				for _, problem := range validation.Problems {
					fields = append(fields, problem.Field)
				}
				t.Fatalf("problem fields = %v, want %q to be one of them", fields, tc.field)
			}
		})
	}
}

// TestImportValidatesBeforeWriting: a document is accepted or refused as a
// whole — a bad entry at the end must not leave the good ones behind.
func TestImportValidatesBeforeWriting(t *testing.T) {
	r := openFixture(t)
	doc := Document{
		Format: Format, Version: Version,
		Rooms: []Room{{Name: "Garage"}, {Name: "Cucina"}},
		Items: []Item{{Name: "", Location: Location{Kind: "room", Room: "Garage"}}},
	}

	if _, err := Import(t.Context(), r, doc); err == nil {
		t.Fatal("Import accepted a document with an invalid item")
	}

	restored, err := Export(t.Context(), r)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(restored.Rooms) != 0 || len(restored.Items) != 0 {
		t.Fatalf("the failed import wrote %d rooms and %d items, want none",
			len(restored.Rooms), len(restored.Items))
	}
}

// TestImportAcceptsAHandWrittenDocument: the format is for people too, so a
// document with nothing but names and one item still works.
func TestImportAcceptsAHandWrittenDocument(t *testing.T) {
	r := openFixture(t)
	doc := Document{
		Format: Format, Version: Version,
		Rooms: []Room{{Name: "Garage"}},
		Items: []Item{{Name: "Trapano", Quantity: 1, Location: Location{Kind: "room", Room: "Garage"}}},
	}

	report, err := Import(t.Context(), r, doc)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Rooms.Created != 1 || report.Items.Created != 1 || report.Containers.Created != 0 {
		t.Fatalf("report = %+v, want one room and one item created", report)
	}
}

// --- helpers ---

func roomNames(doc Document) []string {
	names := make([]string, len(doc.Rooms))
	for index, room := range doc.Rooms {
		names[index] = room.Name
	}
	return names
}

func containerPaths(doc Document) []string {
	paths := make([]string, len(doc.Containers))
	for index, container := range doc.Containers {
		paths[index] = container.Room + "/" + container.Name
	}
	return paths
}

func itemNames(doc Document) []string {
	names := make([]string, len(doc.Items))
	for index, item := range doc.Items {
		names[index] = item.Name
	}
	return names
}
