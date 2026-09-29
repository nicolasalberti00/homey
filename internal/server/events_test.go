package server

// From the HTTP surface: a mutation that goes through the REST API or
// the MCP endpoint leaves its event behind, with the caller and the tool that
// carried it out.

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/nicolasalberti00/homey/internal/events"
	"github.com/nicolasalberti00/homey/internal/storage"
)

// eventsAPI builds the API like newTestAPI does, and also hands back the reader
// of the event log the mutations write.
func eventsAPI(t *testing.T) (humatest.TestAPI, testTokens, events.Store) {
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	store := storage.NewTokenStore(db)
	tokens := testTokens{
		Write: makeTokenHeader(t, store, "test write", "read,write"),
		Read:  makeTokenHeader(t, store, "test read", "read"),
	}

	_, api := humatest.New(t, apiConfig())
	api.UseMiddleware(bearerAuth(store, logger))
	registerOperations(api, Deps{Repos: storage.NewRepos(db), Tokens: store, Logger: logger})
	return api, tokens, storage.NewEventStore(db)
}

// TestRESTMutationsAreRecorded checks every mutation of a lifecycle lands in
// the event log with the token that made it.
func TestRESTMutationsAreRecorded(t *testing.T) {
	api, bearer, eventStore := eventsAPI(t)

	rec := api.Post("/rooms", bearer.Write, RoomInput{Name: "Garage"})
	if rec.Code != 201 {
		t.Fatalf("creating the room = %d; body: %s", rec.Code, rec.Body.String())
	}
	rec = api.Post("/items", bearer.Write, ItemInput{
		Name:     "Trapano",
		Quantity: 1,
		Location: LocationRef{Kind: "room", ID: 1},
	})
	if rec.Code != 201 {
		t.Fatalf("creating the item = %d; body: %s", rec.Code, rec.Body.String())
	}

	// The caller the middleware authenticated is the actor of the event.
	story, err := eventStore.ForEntity(t.Context(), events.EntityRoom, 1)
	if err != nil {
		t.Fatalf("reading the room's events: %v", err)
	}
	if got := eventTypesOf(story); !slices.Equal(got, []string{events.RoomCreated}) {
		t.Fatalf("room events = %v, want the creation", got)
	}
	if story[0].Actor != "test write" || story[0].Tool != "" || story[0].Confirmation != "" {
		t.Fatalf("event = %+v, want the token as actor without a tool", story[0])
	}

	itemStory, err := eventStore.ForEntity(t.Context(), events.EntityItem, 1)
	if err != nil {
		t.Fatalf("reading the item's events: %v", err)
	}
	if got := eventTypesOf(itemStory); !slices.Equal(got, []string{events.ItemCreated}) {
		t.Fatalf("item events = %v, want the creation", got)
	}

	// A read-only token cannot write, and nothing is recorded for the attempt.
	rec = api.Post("/rooms", bearer.Read, RoomInput{Name: "Cucina"})
	if rec.Code != 403 {
		t.Fatalf("a read-only creation = %d, want 403", rec.Code)
	}
	history, err := eventStore.Recent(t.Context(), 10)
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	if count := countOf(eventTypesOf(history), events.RoomCreated); count != 1 {
		t.Fatalf("log = %v, want exactly one room created", eventTypesOf(history))
	}
}

// countOf counts the entries equal to want.
func countOf(list []string, want string) int {
	count := 0
	for _, value := range list {
		if value == want {
			count++
		}
	}
	return count
}

// TestMCPDeleteIsAudited checks the audit of the LLM actions over the wire: the
// event carries which tool ran, who held the token, and how the destructive
// step was authorised.
func TestMCPDeleteIsAudited(t *testing.T) {
	url, eventStore, authorization := mcpEventsServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization,
		`{"name":"Trapano","quantity":1,"location":{"kind":"room","id":1}}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization,
		`{"name":"Cavo rotto","quantity":1,"location":{"kind":"room","id":1}}`)

	session := connectMCP(t, ctx, url, authorization)

	// The confirmed path.
	proposal := callMCP(t, ctx, session, "delete_item", map[string]any{"id": 1})
	token := stringField(t, proposal, "confirmation_token")
	callMCP(t, ctx, session, "delete_item", map[string]any{"id": 1, "confirmation_token": token})
	assertLastEvent(t, eventStore, events.EntityItem, 1, "mcp test", "delete_item", "confirmed")

	// The per-call bypass, on the second item.
	callMCP(t, ctx, session, "delete_item", map[string]any{"id": 2, "confirm": true})
	assertLastEvent(t, eventStore, events.EntityItem, 2, "mcp test", "delete_item", "bypassed")
}

// assertLastEvent reads one entity's story and checks how the newest event was
// authorised.
func assertLastEvent(t *testing.T, store events.Store, kind events.EntityKind, id int64, actor, tool, confirmation string) {
	t.Helper()
	story, err := store.ForEntity(t.Context(), kind, id)
	if err != nil {
		t.Fatalf("reading the events of %s %d: %v", kind, id, err)
	}
	if len(story) == 0 {
		t.Fatalf("no events for %s %d", kind, id)
	}
	last := story[len(story)-1]
	if last.Actor != actor || last.Tool != tool || last.Confirmation != confirmation {
		t.Fatalf("last event = %+v, want actor %q, tool %q, confirmation %q", last, actor, tool, confirmation)
	}
	if !slices.Contains([]string{
		events.ItemCreated, events.ItemUpdated, events.ItemDeleted, events.ItemMoved,
	}, last.Type) {
		t.Fatalf("last event = %+v, want an item event", last)
	}
}

// mcpEventsServer starts the production handler chain with the MCP endpoint on
// and hands back the reader of the event log it writes.
func mcpEventsServer(t *testing.T) (url string, store events.Store, authorization string) {
	t.Helper()
	cfg := defaultTestConfig(t)
	cfg.MCPEnabled = true
	handler, db := newTestHandlerCfg(t, cfg)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	line := makeTokenHeader(t, storage.NewTokenStore(db), "mcp test", "read,write")
	return srv.URL, storage.NewEventStore(db), strings.TrimPrefix(line, "Authorization: ")
}

// eventTypesOf collects the types of an event history.
func eventTypesOf(history []events.Event) []string {
	got := make([]string, len(history))
	for index, event := range history {
		got[index] = event.Type
	}
	return got
}
