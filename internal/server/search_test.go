package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
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

// runSearch runs a query and returns the candidates.
func runSearch(t *testing.T, api humatest.TestAPI, bearer, query string) []SearchResult {
	t.Helper()
	rec := api.Get("/search?q="+url.QueryEscape(query), bearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("searching %q: status = %d, body: %s", query, rec.Code, rec.Body.String())
	}
	var results []SearchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decoding search results: %v", err)
	}
	return results
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

	// Name match: the candidate carries the item, where it lives and why.
	results := runSearch(t, api, bearer.Read, "drill")
	if len(results) != 1 || results[0].Item.ID != drill.ID {
		t.Fatalf("results = %+v, want just the drill", results)
	}
	if results[0].Path != "Garage" {
		t.Fatalf("path = %q, want Garage", results[0].Path)
	}
	if results[0].Match != (SearchMatch{Field: "name", Kind: "partial"}) {
		t.Fatalf("match = %+v, want a partial name match", results[0].Match)
	}
	if results[0].Item.Location.Kind != "room" || results[0].Item.Location.ID != room.ID {
		t.Fatalf("location = %+v, want room %d", results[0].Item.Location, room.ID)
	}

	// Upper case matches the same item: the match ignores case.
	results = runSearch(t, api, bearer.Read, "BOSCH")
	if len(results) != 1 || results[0].Item.ID != drill.ID {
		t.Fatalf("upper-case results = %+v, want the drill", results)
	}
	if results[0].Match != (SearchMatch{Field: "name", Kind: "prefix"}) {
		t.Fatalf("match = %+v, want a name prefix", results[0].Match)
	}

	// The description is searched too.
	results = runSearch(t, api, bearer.Read, "cordless")
	if len(results) != 1 || results[0].Match.Field != "description" {
		t.Fatalf("description results = %+v, want the drill", results)
	}

	// No match is an empty list, never null.
	rec := api.Get("/search?q=cement", bearer.Read)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("body = %q, want an empty array", body)
	}
}

func TestSearchMatchesAliasesAndTags(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	location := LocationRef{Kind: "room", ID: room.ID}
	screwdriver := postItem(t, api, bearer.Write, ItemInput{
		Name:     "Cacciavite",
		Quantity: 1,
		Aliases:  []string{"giravite"},
		Tags:     []string{"strumenti"},
		Location: location,
	})
	postItem(t, api, bearer.Write, ItemInput{Name: "Hammer", Quantity: 1, Location: location})

	cases := []struct {
		query string
		field string
	}{
		{"giravite", "alias"},
		{"strumenti", "tag"},
	}
	for _, tc := range cases {
		results := runSearch(t, api, bearer.Read, tc.query)
		if len(results) != 1 || results[0].Item.ID != screwdriver.ID {
			t.Fatalf("Search(%q) = %+v, want the screwdriver", tc.query, results)
		}
		if results[0].Match != (SearchMatch{Field: tc.field, Kind: "exact"}) {
			t.Fatalf("Search(%q) match = %+v, want an exact %s match", tc.query, results[0].Match, tc.field)
		}
	}
}

// An ambiguous query answers with every candidate, ranked: an exact name
// first, then a partial name, an exact tag and a description, each with the
// path that tells them apart.
func TestSearchRanksCandidates(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	toolbox := createContainerViaAPI(t, api, bearer.Write, room.ID, nil, "Toolbox")
	roomLocation := LocationRef{Kind: "room", ID: room.ID}
	containerLocation := LocationRef{Kind: "container", ID: toolbox.ID}

	byDescription := postItem(t, api, bearer.Write, ItemInput{
		Name: "Valigetta", Description: "per trapano", Quantity: 1, Location: roomLocation,
	})
	byTag := postItem(t, api, bearer.Write, ItemInput{
		Name: "Punte", Tags: []string{"trapano"}, Quantity: 1, Location: roomLocation,
	})
	byNamePartial := postItem(t, api, bearer.Write, ItemInput{
		Name: "Punte per trapano", Quantity: 1, Location: containerLocation,
	})
	byNameExact := postItem(t, api, bearer.Write, ItemInput{
		Name: "Trapano", Quantity: 1, Location: roomLocation,
	})

	results := runSearch(t, api, bearer.Read, "trapano")
	want := []struct {
		id    int64
		path  string
		match SearchMatch
	}{
		{byNameExact.ID, "Garage", SearchMatch{Field: "name", Kind: "exact"}},
		{byNamePartial.ID, "Garage > Toolbox", SearchMatch{Field: "name", Kind: "partial"}},
		{byTag.ID, "Garage", SearchMatch{Field: "tag", Kind: "exact"}},
		{byDescription.ID, "Garage", SearchMatch{Field: "description", Kind: "partial"}},
	}
	if len(results) != len(want) {
		t.Fatalf("results = %+v, want %d candidates", results, len(want))
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

func TestSearchTiesKeepNameOrder(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	location := LocationRef{Kind: "room", ID: room.ID}
	postItem(t, api, bearer.Write, ItemInput{Name: "drill bits", Quantity: 1, Location: location})
	postItem(t, api, bearer.Write, ItemInput{Name: "Drill battery", Quantity: 1, Location: location})
	postItem(t, api, bearer.Write, ItemInput{Name: "Awl", Quantity: 1, Location: location})

	results := runSearch(t, api, bearer.Read, "drill")
	names := make([]string, len(results))
	for index, result := range results {
		names[index] = result.Item.Name
	}
	if want := []string{"Drill battery", "drill bits"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestSearchTreatsWildcardsLiterally(t *testing.T) {
	api, bearer := newTestAPI(t)

	room := createRoomViaAPI(t, api, bearer.Write, "Garage")
	location := LocationRef{Kind: "room", ID: room.ID}
	postItem(t, api, bearer.Write, ItemInput{Name: "Cable 50% copper", Quantity: 1, Location: location})
	postItem(t, api, bearer.Write, ItemInput{Name: "Screws", Quantity: 1, Location: location})

	results := runSearch(t, api, bearer.Read, "50%")
	if len(results) != 1 || results[0].Item.Name != "Cable 50% copper" {
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
