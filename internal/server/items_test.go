package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

func itemPathID(id int64) string {
	return "/items/" + strconv.FormatInt(id, 10)
}

func TestItemLifecycle(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")

	rec := api.Post("/items", bearer.Write, ItemInput{
		Name:        "  Drill  ",
		Description: "  cordless  ",
		Quantity:    2,
		Notes:       "battery in the charger",
		Tags:        []string{"Strumenti", "bagno"},
		Location:    LocationRef{Kind: "room", ID: room.ID},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var created ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if created.Name != "Drill" || created.Quantity != 2 || len(created.Tags) != 2 {
		t.Fatalf("created = %+v", created)
	}
	if created.Location.Kind != "room" || created.Location.ID != room.ID {
		t.Fatalf("location = %+v, want room %d", created.Location, room.ID)
	}
	// Regression: timestamps used to be dropped in the response DTO, leaving
	// Go's zero time (0001-01-01) on every item.
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("created = %+v, want non-zero timestamps", created)
	}

	// Get.
	rec = api.Get(itemPathID(created.ID), bearer.Write)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// List with no filter returns every item.
	rec = api.Get("/items", bearer.Write)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var all []ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &all); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("list = %+v", all)
	}

	// List filtered by room.
	rec = api.Get("/items?room_id="+strconv.FormatInt(room.ID, 10), bearer.Write)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &all); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("filtered list = %+v", all)
	}

	// Patch: change quantity and replace tags.
	quantity := 3
	rec = api.Patch(itemPathID(created.ID), bearer.Write, ItemPatch{Quantity: &quantity, Tags: &[]string{"Officina"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var updated ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if updated.Quantity != 3 || len(updated.Tags) != 1 || updated.Tags[0] != "Officina" {
		t.Fatalf("updated = %+v", updated)
	}

	// Delete.
	rec = api.Delete(itemPathID(created.ID), bearer.Write)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	rec = api.Get(itemPathID(created.ID), bearer.Write)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 after delete", rec.Code)
	}
}

func TestItemMoveViaAPI(t *testing.T) {
	api, bearer := newTestAPI(t)

	garage := createRoomViaAPI(t, api, bearer.Write, "Garage")
	kitchen := createRoomViaAPI(t, api, bearer.Write, "Kitchen")
	toolbox := createContainerViaAPI(t, api, bearer.Write, garage.ID, nil, "Toolbox")
	item := createItemViaAPI(t, api, bearer.Write, LocationRef{Kind: "room", ID: garage.ID}, "Drill")

	// Move into a container.
	rec := api.Post(itemPathID(item.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "container", ID: toolbox.ID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var moved ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &moved); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if moved.Location.Kind != "container" || moved.Location.ID != toolbox.ID {
		t.Fatalf("location after move = %+v", moved.Location)
	}

	// Move into another room.
	rec = api.Post(itemPathID(item.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: kitchen.ID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Unknown destination: 404.
	rec = api.Post(itemPathID(item.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: 4242}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// Name collision at the destination: 409.
	createItemViaAPI(t, api, bearer.Write, LocationRef{Kind: "room", ID: garage.ID}, "Drill")
	rec = api.Post(itemPathID(item.ID)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: garage.ID}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}

	// Unknown item: 404.
	rec = api.Post(itemPathID(4242)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: garage.ID}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestItemErrors(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")

	// Unknown location: 404.
	rec := api.Post("/items", bearer.Write, ItemInput{Name: "Drill", Quantity: 1, Location: LocationRef{Kind: "room", ID: 4242}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// Invalid location kind: 422 from the core.
	rec = api.Post("/items", bearer.Write, ItemInput{Name: "Drill", Quantity: 1, Location: LocationRef{Kind: "shelf", ID: 1}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	_, _, _, locations := decodeProblem(t, rec)
	if len(locations) == 0 || locations[0] != "body.location.kind" {
		t.Fatalf("locations = %v, want body.location.kind", locations)
	}

	// Negative quantity: 422 from the core (the schema allows it, the domain
	// rules live in the inventory package).
	rec = api.Post("/items", bearer.Write, ItemInput{Name: "Drill", Quantity: -1, Location: LocationRef{Kind: "room", ID: room.ID}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	_, _, _, locations = decodeProblem(t, rec)
	if len(locations) == 0 || locations[0] != "body.quantity" {
		t.Fatalf("locations = %v, want body.quantity", locations)
	}

	// Duplicate name in the same location: 409.
	createItemViaAPI(t, api, bearer.Write, LocationRef{Kind: "room", ID: room.ID}, "Cable")
	rec = api.Post("/items", bearer.Write, ItemInput{Name: "cable", Quantity: 1, Location: LocationRef{Kind: "room", ID: room.ID}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}

	// Unknown item: 404 on get/patch/delete/move.
	rec = api.Get(itemPathID(4242), bearer.Write)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	name := "Box"
	rec = api.Patch(itemPathID(4242), bearer.Write, ItemPatch{Name: &name})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	rec = api.Delete(itemPathID(4242), bearer.Write)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	rec = api.Post(itemPathID(4242)+"/move", bearer.Write, MoveBody{Destination: LocationRef{Kind: "room", ID: room.ID}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// Both location filters at once: 422.
	rec = api.Get("/items?room_id=1&container_id=1", bearer.Write)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

// createItemViaAPI is a test helper creating an item through the API.
func createItemViaAPI(t *testing.T, api humatest.TestAPI, bearer string, location LocationRef, name string) ItemResponse {
	t.Helper()
	rec := api.Post("/items", bearer, ItemInput{Name: name, Quantity: 1, Location: location})
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating item %q: status = %d, body: %s", name, rec.Code, rec.Body.String())
	}
	var item ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("decoding item: %v", err)
	}
	return item
}

func TestItemAliases(t *testing.T) {
	api, bearer := newTestAPI(t)
	room := createRoomViaAPI(t, api, bearer.Write, "Garage")

	rec := api.Post("/items", bearer.Write, ItemInput{
		Name:     "Cacciavite",
		Quantity: 1,
		Aliases:  []string{"  Giravite  ", "cacciavite a stella"},
		Location: LocationRef{Kind: "room", ID: room.ID},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var created ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	// Trimmed and ordered case-insensitively, like tags.
	if want := []string{"cacciavite a stella", "Giravite"}; !slices.Equal(created.Aliases, want) {
		t.Fatalf("aliases = %v, want %v", created.Aliases, want)
	}

	// A patch replaces the whole set.
	rec = api.Patch(itemPathID(created.ID), bearer.Write, ItemPatch{Aliases: &[]string{"Giravite"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var updated ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if want := []string{"Giravite"}; !slices.Equal(updated.Aliases, want) {
		t.Fatalf("aliases after patch = %v, want %v", updated.Aliases, want)
	}

	// The rules are reported as field problems, like the other item fields.
	tooMany := make([]string, 21)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("alias-%d", index)
	}
	rec = api.Post("/items", bearer.Write, ItemInput{
		Name:     "Saw",
		Quantity: 1,
		Aliases:  tooMany,
		Location: LocationRef{Kind: "room", ID: room.ID},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	if _, _, _, locations := decodeProblem(t, rec); !slices.Contains(locations, "body.aliases") {
		t.Fatalf("locations = %v, want body.aliases", locations)
	}
}
