package tools

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// ambiguousPlaces are the two toolboxes of the fixture: the names "Toolbox"
// fits both, so a tool that takes a place has to ask which one.
var ambiguousPlaces = []string{"Garage > Toolbox", "Cucina > Toolbox"}

// TestAddItemClarifiesAnAmbiguousPlace: adding to "Toolbox" with two toolboxes
// must ask, not pick one.
func TestAddItemClarifiesAnAmbiguousPlace(t *testing.T) {
	f := newFixture(t)

	var answer AddItemOutput
	f.decode(t, "add_item", `{"name":"Chiave inglese","location":"Toolbox"}`, &answer)
	assertClarification(t, answer.Clarification, "location", "Toolbox")
	if answer.Item != nil {
		t.Fatalf("item = %+v, want none while the place is ambiguous", answer.Item)
	}

	// Nothing was added: both toolboxes hold exactly what they held before.
	var listed ListLocationOutput
	f.decode(t, "list_location", `{"location":"Garage > Toolbox"}`, &listed)
	if len(listed.Items) != 1 || listed.Items[0].Name != "Punte" {
		t.Fatalf("items = %+v, want only the bits", listed.Items)
	}
}

// TestMoveItemClarifiesAnAmbiguousDestination is the exit criterion of the
// step: an ambiguous destination comes back as candidates, and the item does
// not move.
func TestMoveItemClarifiesAnAmbiguousDestination(t *testing.T) {
	f := newFixture(t)
	bits := itoa(int64(f.ids["bits"]))

	var answer MoveItemOutput
	f.decode(t, "move_item", `{"id":`+bits+`,"destination":"toolbox"}`, &answer)
	assertClarification(t, answer.Clarification, "destination", "toolbox")
	if answer.Item != nil {
		t.Fatalf("item = %+v, want none while the destination is ambiguous", answer.Item)
	}

	// The bits stayed where they were.
	var stored ItemView
	f.decode(t, "get_item", `{"id":`+bits+`}`, &stored)
	if stored.Location != "Garage > Toolbox" {
		t.Fatalf("location = %q, want the bits unmoved", stored.Location)
	}
}

// TestReadToolsClarifyAnAmbiguousPlace checks the same question reaches the
// tools that only look around.
func TestReadToolsClarifyAnAmbiguousPlace(t *testing.T) {
	f := newFixture(t)

	var listed ListLocationOutput
	f.decode(t, "list_location", `{"location":"toolbox"}`, &listed)
	assertClarification(t, listed.Clarification, "location", "toolbox")
	if len(listed.Items) != 0 || len(listed.Containers) != 0 {
		t.Fatalf("listed something while the place is ambiguous: %+v", listed)
	}

	var counted CountItemsOutput
	f.decode(t, "count_items", `{"location":"Toolbox"}`, &counted)
	assertClarification(t, counted.Clarification, "location", "Toolbox")
}

// TestClarificationOnlyReplacesAmbiguity checks the other failures are still
// failures, and that a name only one place carries resolves without asking.
func TestClarificationOnlyReplacesAmbiguity(t *testing.T) {
	f := newFixture(t)

	if err := f.fail(t, "add_item", `{"name":"Trapano","location":"Giardino"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown place = %v, want ErrNotFound", err)
	}
	if err := f.fail(t, "move_item", `{"id":`+itoa(int64(f.ids["bits"]))+`,"destination":""}`); !errors.Is(err, inventory.ErrValidation) {
		t.Fatalf("an empty destination = %v, want ErrValidation", err)
	}

	// A full path is not ambiguous: the call acts.
	var answer AddItemOutput
	f.decode(t, "add_item", `{"name":"Chiave inglese","location":"Garage > Toolbox"}`, &answer)
	if answer.Clarification != nil {
		t.Fatalf("a full path was called ambiguous: %+v", answer.Clarification)
	}
	if answer.Item == nil || answer.Item.Location != "Garage > Toolbox" {
		t.Fatalf("item = %+v, want the new item in the toolbox", answer.Item)
	}
}

// assertClarification checks a clarification names the argument and the name
// the caller gave, asks a question and lists every candidate.
func assertClarification(t *testing.T, got *Clarification, argument, name string) {
	t.Helper()
	if got == nil {
		t.Fatal("no clarification came back")
	}
	if got.Argument != argument {
		t.Fatalf("argument = %q, want %q", got.Argument, argument)
	}
	if got.Name != name {
		t.Fatalf("name = %q, want %q", got.Name, name)
	}
	if !strings.Contains(got.Question, name) {
		t.Fatalf("question = %q, want it to name %q", got.Question, name)
	}
	if !slices.Equal(got.Candidates, ambiguousPlaces) {
		t.Fatalf("candidates = %v, want %v", got.Candidates, ambiguousPlaces)
	}
}
