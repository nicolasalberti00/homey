package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicolasalberti00/homey/internal/auth"
)

// TestMCPReadOnlyTokenCannotWrite is the exit criterion of Step 6.6: a token
// without the write scope may explore the inventory but can change nothing,
// whichever write tool it reaches for.
func TestMCPReadOnlyTokenCannotWrite(t *testing.T) {
	url, tokens, writeAuth := mcpTestServerWith(t, auth.ConfirmationRequired)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// The inventory is filled through the REST API with a write token.
	postJSON(t, ctx, url+"/api/v1/rooms", writeAuth, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/items", writeAuth,
		`{"name":"Trapano","quantity":1,"location":{"kind":"room","id":1}}`)

	readAuth := strings.TrimPrefix(
		makeTokenHeaderPolicy(t, tokens, "explorer", "read", auth.ConfirmationRequired),
		"Authorization: ")
	session := connectMCP(t, ctx, url, readAuth)

	// Exploring works: the token is valid and reads are the baseline.
	if got := countField(t, callMCP(t, ctx, session, "count_items", nil)); got != 1 {
		t.Fatalf("count = %d, want the item the write token created", got)
	}

	cases := []struct {
		name string
		args map[string]any
	}{
		{"add_item", map[string]any{"name": "Martello", "location": "Garage"}},
		{"update_item", map[string]any{"id": 1, "quantity": 5}},
		{"move_item", map[string]any{"id": 1, "destination": "Garage"}},
		{"delete_item", map[string]any{"id": 1, "confirm": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			message := callMCPError(t, ctx, session, tc.name, tc.args)
			if !strings.Contains(message, "permission") {
				t.Fatalf("%s = %q, want a permission error", tc.name, message)
			}
		})
	}

	// Nothing changed: one item, still the drill.
	if got := countField(t, callMCP(t, ctx, session, "count_items", nil)); got != 1 {
		t.Fatalf("count = %d, want the inventory untouched", got)
	}
	result := callMCP(t, ctx, session, "search_inventory", map[string]any{"query": "trapano"})
	results, _ := result["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results = %#v, want the drill still there", result["results"])
	}
}

// TestMCPWriteTokenStillWrites checks the permission check did not lock out the
// caller it is meant to let through.
func TestMCPWriteTokenStillWrites(t *testing.T) {
	url, authorization := mcpTestServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)

	session := connectMCP(t, ctx, url, authorization)
	added := callMCP(t, ctx, session, "add_item", map[string]any{"name": "Martello", "location": "Garage"})
	if added["name"] != "Martello" {
		t.Fatalf("added = %#v, want the new item", added)
	}
}

// callMCPError calls a tool that is expected to be refused and returns the
// message the model would read.
func callMCPError(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) string {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	if !result.IsError {
		t.Fatalf("%s was accepted, want a refusal", name)
	}
	return textOf(result)
}
