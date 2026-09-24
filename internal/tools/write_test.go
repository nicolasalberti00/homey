package tools

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// decode runs a tool and decodes what it answered into out.
func (f fixture) decode(t *testing.T, name, input string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(f.call(t, name, input)), out); err != nil {
		t.Fatalf("decoding the answer of %s: %v", name, err)
	}
}

// fail runs a tool that is expected to refuse, and returns the error.
func (f fixture) fail(t *testing.T, name, input string) error {
	t.Helper()
	tool, found := f.registry.Lookup(name)
	if !found {
		t.Fatalf("%s is not registered", name)
	}
	if _, err := tool.Call(writerContext(t), json.RawMessage(input)); err == nil {
		t.Fatalf("%s(%s) was accepted, want a refusal", name, input)
	} else {
		return err
	}
	return nil
}

func TestAddItem(t *testing.T) {
	f := newFixture(t)

	var added ItemView
	f.decode(t, "add_item", `{"name":"Martello","location":"Garage > Toolbox"}`, &added)
	if added.ID == 0 {
		t.Fatal("the new item came back without an id")
	}
	if added.Location != "Garage > Toolbox" {
		t.Fatalf("location = %q, want the toolbox", added.Location)
	}
	// A quantity nobody asked for is one of the thing.
	if added.Quantity != 1 {
		t.Fatalf("quantity = %d, want 1", added.Quantity)
	}

	// Everything else is stored as given, and reads back the same way.
	var whole ItemView
	f.decode(t, "add_item", `{
		"name":"Livella","location":"Garage","quantity":2,
		"description":"a bolla","tags":["officina","misure"],
		"aliases":["livella a bolla"],"notes":"presa in prestito"}`, &whole)
	var stored ItemView
	f.decode(t, "get_item", `{"id":`+itoa(int64(whole.ID))+`}`, &stored)
	if stored.Name != "Livella" || stored.Quantity != 2 || stored.Description != "a bolla" || stored.Notes != "presa in prestito" {
		t.Fatalf("stored = %+v, want what was added", stored)
	}
	if len(stored.Tags) != 2 || len(stored.Aliases) != 1 || stored.Location != "Garage" {
		t.Fatalf("stored = %+v, want the tags, the aliases and the room", stored)
	}
}

func TestAddItemRefusesWhatItCannotPlace(t *testing.T) {
	f := newFixture(t)

	if err := f.fail(t, "add_item", `{"name":"Trapano","location":"Giardino"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown place = %v, want ErrNotFound", err)
	}
	if err := f.fail(t, "add_item", `{"name":"Trapano","location":"Toolbox"}`); !errors.Is(err, inventory.ErrAmbiguous) {
		t.Fatalf("a shared place name = %v, want ErrAmbiguous", err)
	}
	// Punte already lives in that toolbox: adding another would overwrite it.
	err := f.fail(t, "add_item", `{"name":"Punte","location":"Garage > Toolbox"}`)
	if !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate name = %v, want ErrConflict", err)
	}
	if !CallerError(err) {
		t.Fatal("a duplicate name is not reported as something the caller can act on")
	}
	// The same name somewhere else is a different item, not a clash.
	f.call(t, "add_item", `{"name":"Punte","location":"Garage"}`)
}

func TestUpdateItemChangesOnlyWhatItIsGiven(t *testing.T) {
	f := newFixture(t)
	id := itoa(int64(f.ids["drill in the drawer"]))

	var updated ItemView
	f.decode(t, "update_item", `{"id":`+id+`,"quantity":3}`, &updated)
	if updated.Quantity != 3 {
		t.Fatalf("quantity = %d, want 3", updated.Quantity)
	}
	// Nothing else moved.
	if updated.Name != "Trapano" || updated.Description != "Bosch blu" || updated.Notes != "regalo di papà" {
		t.Fatalf("updated = %+v, want the other fields untouched", updated)
	}
	if updated.Location != "Garage > Toolbox > Cassetto 1" || len(updated.Tags) != 1 || len(updated.Aliases) != 1 {
		t.Fatalf("updated = %+v, want the tags, the aliases and the place untouched", updated)
	}

	// Tags are replaced as a set, and an empty set empties them.
	var retagged ItemView
	f.decode(t, "update_item", `{"id":`+id+`,"tags":["officina","nuovo"],"aliases":[]}`, &retagged)
	if len(retagged.Tags) != 2 || len(retagged.Aliases) != 0 {
		t.Fatalf("retagged = %+v, want the new tags and no aliases", retagged)
	}

	// What is stored is what the answer says.
	var stored ItemView
	f.decode(t, "get_item", `{"id":`+id+`}`, &stored)
	if stored.Quantity != 3 || len(stored.Tags) != 2 || len(stored.Aliases) != 0 {
		t.Fatalf("stored = %+v, want the updates", stored)
	}
}

func TestUpdateItemRefusesWhatItCannotChange(t *testing.T) {
	f := newFixture(t)

	if err := f.fail(t, "update_item", `{"id":4242,"quantity":1}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown item = %v, want ErrNotFound", err)
	}
	// An empty name is refused by the domain rules, not silently stored.
	if err := f.fail(t, "update_item", `{"id":`+itoa(int64(f.ids["bits"]))+`,"name":"  "}`); !errors.Is(err, inventory.ErrValidation) {
		t.Fatalf("an empty name = %v, want ErrValidation", err)
	}
	// Two items in one place cannot share a name: the bits would become the
	// drill's twin in the toolbox.
	f.call(t, "add_item", `{"name":"Chiave inglese","location":"Garage > Toolbox"}`)
	if err := f.fail(t, "update_item", `{"id":`+itoa(int64(f.ids["bits"]))+`,"name":"Chiave inglese"}`); !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate name = %v, want ErrConflict", err)
	}
}

// TestMoveItemPutsTheDrillInTheToolbox is the exit criterion of the write
// tools: "metti il trapano Bosch nel toolbox in garage" ends with the drill in
// the toolbox, and with storage saying so.
func TestMoveItemPutsTheDrillInTheToolbox(t *testing.T) {
	f := newFixture(t)

	// What a model does first: find the drill, by the words of the question.
	var found SearchInventoryOutput
	f.decode(t, "search_inventory", `{"query":"trapano bosch"}`, &found)
	if found.Count != 1 {
		t.Fatalf("search found %d items, want the Bosch drill", found.Count)
	}

	var moved ItemView
	f.decode(t, "move_item", `{"id":`+itoa(int64(found.Results[0].ID))+`,"destination":"Garage > Toolbox"}`, &moved)
	if moved.Location != "Garage > Toolbox" {
		t.Fatalf("moved to %q, want the toolbox", moved.Location)
	}
	if moved.Name != "Trapano" || moved.Quantity != 1 || len(moved.Tags) != 1 {
		t.Fatalf("moved = %+v, want everything else untouched", moved)
	}

	var stored ItemView
	f.decode(t, "get_item", `{"id":`+itoa(int64(found.Results[0].ID))+`}`, &stored)
	if stored.Location != "Garage > Toolbox" {
		t.Fatalf("storage says %q, want the toolbox", stored.Location)
	}
}

func TestMoveItemResolvesDestinations(t *testing.T) {
	f := newFixture(t)
	bits := itoa(int64(f.ids["bits"]))

	// A nested place, named the loose way a person types it.
	var moved ItemView
	f.decode(t, "move_item", `{"id":`+bits+`,"destination":"garage > toolbox > cassetto 1"}`, &moved)
	if moved.Location != "Garage > Toolbox > Cassetto 1" {
		t.Fatalf("location = %q, want the drawer", moved.Location)
	}
	// A bare container name, when only one place carries it.
	f.decode(t, "move_item", `{"id":`+bits+`,"destination":"Cucina"}`, &moved)
	if moved.Location != "Cucina" {
		t.Fatalf("location = %q, want the kitchen", moved.Location)
	}
}

func TestMoveItemRefusesWhatItCannotDo(t *testing.T) {
	f := newFixture(t)

	if err := f.fail(t, "move_item", `{"id":4242,"destination":"Garage"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown item = %v, want ErrNotFound", err)
	}
	if err := f.fail(t, "move_item", `{"id":`+itoa(int64(f.ids["bits"]))+`,"destination":"Giardino"}`); !errors.Is(err, inventory.ErrNotFound) {
		t.Fatalf("an unknown place = %v, want ErrNotFound", err)
	}
	if err := f.fail(t, "move_item", `{"id":`+itoa(int64(f.ids["bits"]))+`,"destination":"Toolbox"}`); !errors.Is(err, inventory.ErrAmbiguous) {
		t.Fatalf("a shared place name = %v, want ErrAmbiguous", err)
	}
	// The kitchen drill would land where the other drill already lives.
	err := f.fail(t, "move_item", `{"id":`+itoa(int64(f.ids["drill in the kitchen"]))+`,"destination":"Garage > Toolbox > Cassetto 1"}`)
	if !errors.Is(err, inventory.ErrConflict) {
		t.Fatalf("a duplicate name in the destination = %v, want ErrConflict", err)
	}
}

// TestWriteToolsAreWrites checks the annotations a host uses to decide whether
// a tool needs care: none of these is a read, adding is additive, changing and
// moving are not.
func TestWriteToolsAreWrites(t *testing.T) {
	f := newFixture(t)

	cases := []struct {
		name        string
		destructive bool
	}{
		{"add_item", false},
		{"update_item", true},
		{"move_item", true},
	}
	for _, tc := range cases {
		tool, found := f.registry.Lookup(tc.name)
		if !found {
			t.Fatalf("%s is not registered", tc.name)
		}
		if tool.Permission() != PermissionWrite {
			t.Fatalf("%s needs %s, want write", tc.name, tool.Permission())
		}
		if tool.Destructive() != tc.destructive {
			t.Fatalf("%s is destructive=%t, want %t", tc.name, tool.Destructive(), tc.destructive)
		}
		// Confirmation is a separate decision, and it comes later.
		if tool.RequiresConfirmation() {
			t.Fatalf("%s asks for confirmation, which is not built yet", tc.name)
		}
	}
}
