package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

// postItem creates one item through the API and returns the stored
// representation.
func postItem(t *testing.T, api humatest.TestAPI, bearer string, input ItemInput) ItemResponse {
	t.Helper()
	rec := api.Post("/items", bearer, input)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating item %q: status = %d, body: %s", input.Name, rec.Code, rec.Body.String())
	}
	var item ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("decoding item: %v", err)
	}
	return item
}

func TestSearchMatchesNameAndDescription(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	location := LocationRef{Kind: "room", ID: room.ID}
	drill := postItem(t, api, bearer.Write, ItemInput{
		Name:        "Bosch drill",
		Description: "cordless",
		Quantity:    1,
		Location:    location,
	})
	postItem(t, api, bearer.Write, ItemInput{Name: "Hammer", Quantity: 1, Location: location})

	// Name match.
	rec := api.Get("/search?q=drill", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var results []ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(results) != 1 || results[0].ID != drill.ID {
		t.Fatalf("results = %+v, want just the drill", results)
	}
	// The response carries the whole item, location included.
	if results[0].Location.Kind != "room" || results[0].Location.ID != room.ID {
		t.Fatalf("location = %+v, want room %d", results[0].Location, room.ID)
	}

	// Upper case matches the same item: the match ignores case.
	rec = api.Get("/search?q=BOSCH", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(results) != 1 || results[0].ID != drill.ID {
		t.Fatalf("upper-case results = %+v, want the drill", results)
	}

	// The description is searched too.
	rec = api.Get("/search?q=cordless", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(results) != 1 || results[0].ID != drill.ID {
		t.Fatalf("description results = %+v, want the drill", results)
	}

	// No match is an empty list, never null.
	rec = api.Get("/search?q=cement", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("body = %q, want an empty array", body)
	}
}

func TestSearchOrdersByName(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	location := LocationRef{Kind: "room", ID: room.ID}
	createItemViaAPI(t, api, bearer.Write, location, "drill battery")
	createItemViaAPI(t, api, bearer.Write, location, "Awl")
	createItemViaAPI(t, api, bearer.Write, location, "Drill")

	rec := api.Get("/search?q=drill", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var results []ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	// Case-insensitive by name, like every other listing.
	want := []string{"Drill", "drill battery"}
	if len(results) != len(want) {
		t.Fatalf("results = %+v, want %v", results, want)
	}
	for i, name := range want {
		if results[i].Name != name {
			t.Fatalf("results[%d].Name = %q, want %q", i, results[i].Name, name)
		}
	}
}

func TestSearchTreatsWildcardsLiterally(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	location := LocationRef{Kind: "room", ID: room.ID}
	createItemViaAPI(t, api, bearer.Write, location, "Cable 50% copper")
	createItemViaAPI(t, api, bearer.Write, location, "Screws")

	rec := api.Get("/search?q=50%25", bearer.Read) // %25 is an encoded %
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var results []ItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(results) != 1 || results[0].Name != "Cable 50% copper" {
		t.Fatalf("results = %+v, want the cable only", results)
	}
}

func TestSearchRequiresAQuery(t *testing.T) {
	api, bearer := newTestAPI(t)

	rec := api.Get("/search", bearer.Read)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 without q; body: %s", rec.Code, rec.Body.String())
	}
	status, _, _, _ := decodeProblem(t, rec)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("problem status = %d, want 422", status)
	}
}
