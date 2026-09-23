package search

import (
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

func TestClassifyRanksFieldsBeforeKinds(t *testing.T) {
	item := inventory.Item{
		Name:        "Bosch trapano",
		Description: "trapano a percussione con valigetta",
		Aliases:     []string{"drill", "batteria"},
		Tags:        []string{"trapano", "officina"},
	}

	cases := []struct {
		name string
		term string
		want Match
	}{
		{"name prefix", "bosch", Match{FieldName, KindPrefix}},
		{"name partial", "apano", Match{FieldName, KindPartial}},
		// The name comes first: a partial match there beats an exact tag.
		{"tag exact loses to the name", "trapano", Match{FieldName, KindPartial}},
		{"alias exact", "drill", Match{FieldAlias, KindExact}},
		{"alias prefix", "batte", Match{FieldAlias, KindPrefix}},
		{"alias partial", "teria", Match{FieldAlias, KindPartial}},
		{"tag exact", "officina", Match{FieldTag, KindExact}},
		{"tag partial", "ffici", Match{FieldTag, KindPartial}},
		{"description partial", "valigetta", Match{FieldDescription, KindPartial}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Classify(item, []string{tc.term})
			if !ok {
				t.Fatalf("Classify(%q) found nothing", tc.term)
			}
			if got != tc.want {
				t.Fatalf("Classify(%q) = %+v, want %+v", tc.term, got, tc.want)
			}
		})
	}

	if _, ok := Classify(item, []string{"cemento"}); ok {
		t.Fatal("Classify found a match for a term that is nowhere in the item")
	}
}

func TestClassifyPrefersTheCloserKindWithinAField(t *testing.T) {
	item := inventory.Item{Name: "Drill", Tags: []string{"trapano", "trapanino"}}

	cases := []struct {
		term string
		want Match
	}{
		// "trapanino" is a tag exactly and a prefix of the other one.
		{"trapanino", Match{FieldTag, KindExact}},
		{"trapani", Match{FieldTag, KindPrefix}},
		{"panin", Match{FieldTag, KindPartial}},
	}
	for _, tc := range cases {
		got, ok := Classify(item, []string{tc.term})
		if !ok || got != tc.want {
			t.Fatalf("Classify(%q) = %+v/%t, want %+v", tc.term, got, ok, tc.want)
		}
	}
}

func TestClassifyIsCaseInsensitive(t *testing.T) {
	item := inventory.Item{Name: "Trapano", Aliases: []string{"Giravite"}}

	for _, term := range []string{"trapano", "TRAPANO", "TraPaNo"} {
		got, ok := Classify(item, []string{term})
		if !ok || got != (Match{FieldName, KindExact}) {
			t.Fatalf("Classify(%q) = %+v/%t, want an exact name match", term, got, ok)
		}
	}
	if got, ok := Classify(item, []string{"giravite"}); !ok || got.Field != FieldAlias {
		t.Fatalf("Classify(alias) = %+v/%t, want an alias match", got, ok)
	}
}

func TestClassifyTakesTheBestTerm(t *testing.T) {
	item := inventory.Item{
		Name:        "Bosch drill",
		Description: "cordless",
	}

	// The second term matches the name exactly: that is the best of the two.
	got, ok := Classify(item, []string{"cordless", "bosch drill"})
	if !ok || got != (Match{FieldName, KindExact}) {
		t.Fatalf("Classify = %+v/%t, want the exact name match", got, ok)
	}
}

func TestClassifyIgnoresAccents(t *testing.T) {
	item := inventory.Item{Name: "Caffè", Tags: []string{"caffetteria"}}

	cases := []struct {
		term string
		want Match
	}{
		// The accent and its direction do not decide the kind: what a user
		// types as "caffe" is an exact match of "Caffè".
		{"caffe", Match{FieldName, KindExact}},
		{"caffé", Match{FieldName, KindExact}},
		{"CAFFÈ", Match{FieldName, KindExact}},
		{"caffett", Match{FieldTag, KindPrefix}},
		{"affetteria", Match{FieldTag, KindPartial}},
	}
	for _, tc := range cases {
		got, ok := Classify(item, []string{tc.term})
		if !ok || got != tc.want {
			t.Fatalf("Classify(%q) = %+v/%t, want %+v", tc.term, got, ok, tc.want)
		}
	}
}

func TestLocationsPath(t *testing.T) {
	toolbox := inventory.ContainerID(10)
	index := locations{
		rooms: map[inventory.RoomID]inventory.Room{
			1: {ID: 1, Name: "Garage"},
			2: {ID: 2, Name: "Kitchen"},
		},
		containers: map[inventory.ContainerID]inventory.Container{
			10: {ID: 10, RoomID: 1, Name: "Toolbox"},
			11: {ID: 11, RoomID: 1, ParentID: &toolbox, Name: "Drawer 1"},
		},
	}

	cases := []struct {
		name     string
		location inventory.Location
		want     string
	}{
		{"room", inventory.RoomLocation(1), "Garage"},
		{"root container", inventory.ContainerLocation(10), "Garage > Toolbox"},
		{"nested container", inventory.ContainerLocation(11), "Garage > Toolbox > Drawer 1"},
		{"unknown room", inventory.RoomLocation(99), ""},
		{"unknown container", inventory.ContainerLocation(99), ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := index.path(tc.location); got != tc.want {
				t.Fatalf("path = %q, want %q", got, tc.want)
			}
		})
	}
}
