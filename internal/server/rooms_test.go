package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func pathID(id int64) string {
	return "/rooms/" + strconv.FormatInt(id, 10)
}

func TestRoomLifecycle(t *testing.T) {
	api := newTestAPI(t)

	rec := api.Post("/rooms", RoomInput{Name: "  Garage  ", Description: "  cars and tools  "})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var created RoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if created.ID <= 0 || created.Name != "Garage" || created.Description != "cars and tools" {
		t.Fatalf("created = %+v", created)
	}

	rec = api.Get("/rooms")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var list []RoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decoding list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Garage" {
		t.Fatalf("list = %+v", list)
	}

	rec = api.Get(pathID(created.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	name := "Workshop"
	rec = api.Patch(pathID(created.ID), RoomPatch{Name: &name})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var updated RoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if updated.Name != "Workshop" || updated.Description != "cars and tools" {
		t.Fatalf("updated = %+v", updated)
	}

	rec = api.Delete(pathID(created.ID))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	rec = api.Get(pathID(created.ID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 after delete", rec.Code)
	}
}

func TestRoomErrors(t *testing.T) {
	api := newTestAPI(t)

	rec := api.Post("/rooms", RoomInput{Name: "Garage"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	// Duplicate name: 409 with an RFC 9457 problem.
	rec = api.Post("/rooms", RoomInput{Name: "garage"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
		t.Errorf("content type = %q, want application/problem+json", ct)
	}
	status, title, detail, _ := decodeProblem(t, rec)
	if status != http.StatusConflict || title != "Conflict" {
		t.Fatalf("problem = %d/%q", status, title)
	}
	if detail == "" || strings.Contains(detail, "inventory:") {
		t.Errorf("detail %q should be human-readable without sentinel leaks", detail)
	}

	// Unknown room: 404 with the lookup context as detail.
	rec = api.Get("/rooms/4242")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	status, title, detail, _ = decodeProblem(t, rec)
	if status != http.StatusNotFound || title != "Not Found" || detail != "room 4242 not found" {
		t.Fatalf("problem = %d/%q/%q", status, title, detail)
	}

	// Schema-level validation: 422 from Huma itself.
	rec = api.Post("/rooms", RoomInput{Name: ""})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	_, _, _, locations := decodeProblem(t, rec)
	if len(locations) == 0 || locations[0] != "body.name" {
		t.Fatalf("locations = %v, want body.name", locations)
	}

	// Domain-level validation: a whitespace-only name passes the schema, the
	// core rejects it, and the mapper turns it into the same 422 shape.
	rec = api.Post("/rooms", RoomInput{Name: "   "})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	_, _, _, locations = decodeProblem(t, rec)
	if len(locations) == 0 || locations[0] != "body.name" {
		t.Fatalf("locations = %v, want body.name", locations)
	}

	// Patching an unknown room: 404.
	name := "Workshop"
	rec = api.Patch("/rooms/4242", RoomPatch{Name: &name})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
