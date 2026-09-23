package search

import (
	"context"
	"errors"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// fakeItems answers Search with a fixed list of candidates, the way a
// repository returns the items that matched a query.
type fakeItems struct {
	inventory.ItemRepo
	candidates []inventory.Item
	err        error
	calls      int
}

func (f *fakeItems) Search(context.Context, string) ([]inventory.Item, error) {
	f.calls++
	return f.candidates, f.err
}

// fakeRooms and fakeContainers serve the inventory a path is resolved from.
type fakeRooms struct {
	inventory.RoomRepo
	rooms []inventory.Room
	err   error
}

func (f fakeRooms) List(context.Context) ([]inventory.Room, error) { return f.rooms, f.err }

type fakeContainers struct {
	inventory.ContainerRepo
	containers []inventory.Container
	err        error
}

func (f fakeContainers) List(context.Context) ([]inventory.Container, error) {
	return f.containers, f.err
}

// TestEngineRanksCandidatesAndResolvesPaths covers a whole search: the
// candidates come back name-ordered, leave ranked by how well they match, and
// each one carries the path that tells it apart from the others.
func TestEngineRanksCandidatesAndResolvesPaths(t *testing.T) {
	toolbox := inventory.ContainerID(10)
	drawer := inventory.ContainerID(11)
	engine := Engine{
		Items: &fakeItems{candidates: []inventory.Item{
			{ID: 1, Name: "Valigetta", Description: "per trapano", Location: inventory.RoomLocation(1)},
			{ID: 2, Name: "Punte", Tags: []string{"trapano"}, Location: inventory.RoomLocation(1)},
			{ID: 3, Name: "Punte per trapano", Location: inventory.ContainerLocation(drawer)},
			{ID: 4, Name: "Trapano", Location: inventory.ContainerLocation(toolbox)},
			// A dangling container: the candidate is still useful, so the
			// search never fails on a broken reference.
			{ID: 5, Name: "Trapano a batteria", Location: inventory.ContainerLocation(99)},
		}},
		Rooms: fakeRooms{rooms: []inventory.Room{{ID: 1, Name: "Garage"}}},
		Containers: fakeContainers{containers: []inventory.Container{
			{ID: 10, RoomID: 1, Name: "Toolbox"},
			{ID: 11, RoomID: 1, ParentID: &toolbox, Name: "Drawer 1"},
		}},
	}

	results, err := engine.Search(context.Background(), "trapano")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := []struct {
		id    inventory.ItemID
		path  string
		match Match
	}{
		{4, "Garage > Toolbox", Match{FieldName, KindExact}},
		{5, "", Match{FieldName, KindPrefix}},
		{3, "Garage > Toolbox > Drawer 1", Match{FieldName, KindPartial}},
		{2, "Garage", Match{FieldTag, KindExact}},
		{1, "Garage", Match{FieldDescription, KindPartial}},
	}
	if len(results) != len(want) {
		t.Fatalf("Search = %+v, want %d candidates", results, len(want))
	}
	for index, expected := range want {
		if results[index].Item.ID != expected.id {
			t.Fatalf("results[%d] = %+v, want item %d", index, results[index], expected.id)
		}
		if results[index].Path != expected.path {
			t.Fatalf("results[%d].Path = %q, want %q", index, results[index].Path, expected.path)
		}
		if results[index].Match != expected.match {
			t.Fatalf("results[%d].Match = %+v, want %+v", index, results[index].Match, expected.match)
		}
	}
}

func TestEngineAnswersABlankQueryWithoutSearching(t *testing.T) {
	items := &fakeItems{candidates: []inventory.Item{{ID: 1, Name: "Trapano"}}}
	engine := Engine{Items: items, Rooms: fakeRooms{}, Containers: fakeContainers{}}

	for _, query := range []string{"", "   ", "\t\n"} {
		results, err := engine.Search(context.Background(), query)
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}
		if results == nil || len(results) != 0 {
			t.Fatalf("Search(%q) = %+v, want an empty, non-nil list", query, results)
		}
	}
	if items.calls != 0 {
		t.Fatalf("the repository was searched %d times for a blank query, want 0", items.calls)
	}
}

func TestEngineAnswersNoCandidatesWithoutLoadingLocations(t *testing.T) {
	// Nothing matched: the rooms and containers are not read, so a repository
	// that would fail there is never consulted.
	engine := Engine{
		Items:      &fakeItems{},
		Rooms:      fakeRooms{err: errors.New("rooms unavailable")},
		Containers: fakeContainers{err: errors.New("containers unavailable")},
	}

	results, err := engine.Search(context.Background(), "trapano")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results == nil || len(results) != 0 {
		t.Fatalf("Search = %+v, want an empty, non-nil list", results)
	}
}

func TestEngineReportsRepositoryFailures(t *testing.T) {
	itemsErr := errors.New("items unavailable")
	roomsErr := errors.New("rooms unavailable")
	containersErr := errors.New("containers unavailable")
	candidate := inventory.Item{ID: 1, Name: "Trapano", Location: inventory.RoomLocation(1)}

	cases := []struct {
		name   string
		engine Engine
		want   error
	}{
		{
			"items",
			Engine{Items: &fakeItems{err: itemsErr}, Rooms: fakeRooms{}, Containers: fakeContainers{}},
			itemsErr,
		},
		{
			"rooms",
			Engine{Items: &fakeItems{candidates: []inventory.Item{candidate}}, Rooms: fakeRooms{err: roomsErr}, Containers: fakeContainers{}},
			roomsErr,
		},
		{
			"containers",
			Engine{Items: &fakeItems{candidates: []inventory.Item{candidate}}, Rooms: fakeRooms{}, Containers: fakeContainers{err: containersErr}},
			containersErr,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.engine.Search(context.Background(), "trapano"); !errors.Is(err, tc.want) {
				t.Fatalf("Search error = %v, want %v", err, tc.want)
			}
		})
	}
}
