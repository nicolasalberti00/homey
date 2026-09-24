package inventory

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// testIndex builds the index of a small home: two rooms, a toolbox in each of
// them (so a bare "Toolbox" is ambiguous), a drawer inside one of them, and a
// few broken references a real inventory can end up with.
func testIndex() LocationIndex {
	toolbox := ContainerID(10)
	missing := ContainerID(99)
	itself := ContainerID(16)
	return NewLocationIndex(
		[]Room{{ID: 1, Name: "Garage"}, {ID: 2, Name: "Cantina"}},
		[]Container{
			{ID: 10, RoomID: 1, Name: "Toolbox"},
			{ID: 11, RoomID: 1, ParentID: &toolbox, Name: "Cassetto 1"},
			{ID: 12, RoomID: 2, Name: "Toolbox"},
			{ID: 13, RoomID: 2, Name: "Scaffale"},
			{ID: 14, RoomID: 99, Name: "Senza stanza"},
			{ID: 15, RoomID: 1, ParentID: &missing, Name: "Senza padre"},
			{ID: 16, RoomID: 1, ParentID: &itself, Name: "In cerchio"},
			{ID: 17, RoomID: 2, Name: "Garage"},
		},
	)
}

func TestLocationIndexRendersPaths(t *testing.T) {
	index := testIndex()

	cases := []struct {
		name     string
		location Location
		want     string
	}{
		{"room", RoomLocation(1), "Garage"},
		{"root container", ContainerLocation(10), "Garage > Toolbox"},
		{"nested container", ContainerLocation(11), "Garage > Toolbox > Cassetto 1"},
		{"unknown room", RoomLocation(99), ""},
		{"unknown container", ContainerLocation(99), ""},
		{"container without a room", ContainerLocation(14), ""},
		{"container whose parent is missing", ContainerLocation(15), "Garage > Senza padre"},
		{"container that is its own parent", ContainerLocation(16), "Garage > In cerchio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := index.Path(tc.location); got != tc.want {
				t.Fatalf("Path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLocationIndexListsChildren(t *testing.T) {
	index := testIndex()

	cases := []struct {
		name     string
		location Location
		want     []ContainerID
	}{
		// 15 names a parent that is not in the inventory, so it hangs
		// directly in the room, the way Path renders it. 16 names itself,
		// which keeps it out of reach of the room.
		{"room", RoomLocation(1), []ContainerID{10, 15}},
		{"another room", RoomLocation(2), []ContainerID{12, 13, 17}},
		{"container", ContainerLocation(10), []ContainerID{11}},
		{"leaf container", ContainerLocation(11), nil},
		{"container that is its own parent", ContainerLocation(16), []ContainerID{16}},
		{"unknown room", RoomLocation(99), nil},
		{"unknown container", ContainerLocation(99), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			children := index.Children(tc.location)
			got := make([]ContainerID, len(children))
			for i, child := range children {
				got[i] = child.ID
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("Children = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLocationIndexDoesNotDependOnListingOrder guards a bug the tools found:
// containers arrive ordered by name, so "Cassetto 1" comes before the
// "Toolbox" that holds it, and the path of a container must not depend on the
// order it was indexed in.
func TestLocationIndexDoesNotDependOnListingOrder(t *testing.T) {
	toolbox := ContainerID(10)
	index := NewLocationIndex(
		[]Room{{ID: 1, Name: "Garage"}},
		[]Container{
			{ID: 11, RoomID: 1, ParentID: &toolbox, Name: "Cassetto 1"},
			{ID: 10, RoomID: 1, Name: "Toolbox"},
		},
	)

	if got := index.Path(ContainerLocation(11)); got != "Garage > Toolbox > Cassetto 1" {
		t.Fatalf("Path = %q, want the drawer inside the toolbox", got)
	}
	if got, err := index.Resolve("Garage > Toolbox > Cassetto 1"); err != nil || got != ContainerLocation(11) {
		t.Fatalf("Resolve = %v/%v, want the drawer", got, err)
	}
	if got, err := index.Resolve("Cassetto 1"); err != nil || got != ContainerLocation(11) {
		t.Fatalf("Resolve by name = %v/%v, want the drawer", got, err)
	}
}

func TestLocationIndexListsAncestors(t *testing.T) {
	index := testIndex()

	cases := []struct {
		name     string
		location Location
		want     []Location
	}{
		{"a room is its own ancestor", RoomLocation(1), []Location{RoomLocation(1)}},
		{"a root container", ContainerLocation(10), []Location{ContainerLocation(10), RoomLocation(1)}},
		{"a nested container", ContainerLocation(11), []Location{ContainerLocation(11), ContainerLocation(10), RoomLocation(1)}},
		{"a container without a room", ContainerLocation(14), []Location{ContainerLocation(14)}},
		{"a container that is its own parent", ContainerLocation(16), []Location{ContainerLocation(16), RoomLocation(1)}},
		{"an unknown room", RoomLocation(99), nil},
		{"an unknown container", ContainerLocation(99), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := index.Ancestors(tc.location); !slices.Equal(got, tc.want) {
				t.Fatalf("Ancestors = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLocationIndexResolvesNamesAndPaths(t *testing.T) {
	index := testIndex()

	cases := []struct {
		name  string
		input string
		want  Location
	}{
		{"a room", "Garage", RoomLocation(1)},
		{"a room, ignoring case", "garage", RoomLocation(1)},
		{"a container on its own", "Scaffale", ContainerLocation(13)},
		{"a container, ignoring case and accents", "cassettò 1", ContainerLocation(11)},
		{"a full path", "Garage > Toolbox", ContainerLocation(10)},
		{"a full path, loosely written", "garage>toolbox", ContainerLocation(10)},
		{"a nested path", "Garage > Toolbox > Cassetto 1", ContainerLocation(11)},
		{"a room wins over a container named like it", "Garage", RoomLocation(1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := index.Resolve(tc.input)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("Resolve(%q) = %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestLocationIndexResolveFailures(t *testing.T) {
	index := testIndex()

	t.Run("empty", func(t *testing.T) {
		if _, err := index.Resolve("   "); !errors.Is(err, ErrValidation) {
			t.Fatalf("Resolve(\"\") = %v, want ErrValidation", err)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		if _, err := index.Resolve("Giardino"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Resolve(\"Giardino\") = %v, want ErrNotFound", err)
		}
	})

	t.Run("a path that does not exist", func(t *testing.T) {
		if _, err := index.Resolve("Garage > Scaffale"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Resolve = %v, want ErrNotFound", err)
		}
	})

	t.Run("ambiguous", func(t *testing.T) {
		_, err := index.Resolve("Toolbox")
		if !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("Resolve(\"Toolbox\") = %v, want ErrAmbiguous", err)
		}
		var ambiguous *AmbiguousError
		if !errors.As(err, &ambiguous) {
			t.Fatalf("Resolve(\"Toolbox\") = %v, want an *AmbiguousError", err)
		}
		want := []string{"Garage > Toolbox", "Cantina > Toolbox"}
		if !slices.Equal(ambiguous.Candidates, want) {
			t.Fatalf("candidates = %v, want %v", ambiguous.Candidates, want)
		}
		// The message has to name the candidates: it is what a caller reads
		// back to the person who asked.
		for _, candidate := range want {
			if !strings.Contains(err.Error(), candidate) {
				t.Fatalf("error %q does not mention %q", err, candidate)
			}
		}
	})
}

// TestLocationIndexZeroValueResolvesNothing documents that an index nobody
// built is harmless: a caller that forgot to load the inventory gets empty
// paths rather than a panic.
func TestLocationIndexZeroValueResolvesNothing(t *testing.T) {
	var index LocationIndex
	if got := index.Path(RoomLocation(1)); got != "" {
		t.Fatalf("Path = %q, want an empty path", got)
	}
	if children := index.Children(RoomLocation(1)); len(children) != 0 {
		t.Fatalf("Children = %v, want none", children)
	}
	if _, err := index.Resolve("Garage"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve = %v, want ErrNotFound", err)
	}
}
