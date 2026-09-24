package server

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPAmbiguousDestinationAsksForAChoice is Step 6.7 over the wire: a
// destination that fits two places comes back as a structured clarification,
// the item does not move, and naming the full path carries the call through.
func TestMCPAmbiguousDestinationAsksForAChoice(t *testing.T) {
	url, authorization := mcpTestServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Cucina"}`)
	postJSON(t, ctx, url+"/api/v1/containers", authorization, `{"name":"Toolbox","room_id":1}`)
	postJSON(t, ctx, url+"/api/v1/containers", authorization, `{"name":"Toolbox","room_id":2}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization,
		`{"name":"Trapano","quantity":1,"location":{"kind":"room","id":1}}`)

	session := connectMCP(t, ctx, url, authorization)

	answer := callMCP(t, ctx, session, "move_item", map[string]any{"id": 1, "destination": "Toolbox"})
	if _, moved := answer["item"]; moved {
		t.Fatalf("move_item acted on an ambiguous destination: %#v", answer)
	}
	clarification, ok := answer["clarification"].(map[string]any)
	if !ok {
		t.Fatalf("clarification = %#v, want the candidates", answer["clarification"])
	}
	if clarification["argument"] != "destination" {
		t.Fatalf("argument = %#v, want destination", clarification["argument"])
	}
	raw, _ := clarification["candidates"].([]any)
	candidates := make([]string, len(raw))
	for index, candidate := range raw {
		candidates[index], _ = candidate.(string)
	}
	want := []string{"Garage > Toolbox", "Cucina > Toolbox"}
	if !slices.Equal(candidates, want) {
		t.Fatalf("candidates = %v, want %v", candidates, want)
	}

	// The drill did not move.
	var moved map[string]any
	for _, result := range searchResults(t, callMCPResult(t, ctx, session, "search_inventory", map[string]any{"query": "trapano"})) {
		if result["name"] == "Trapano" {
			moved = result
		}
	}
	if moved == nil || moved["location"] != "Garage" {
		t.Fatalf("the drill moved on a clarification: %#v", moved)
	}

	// Choosing a candidate carries the move through.
	done := callMCP(t, ctx, session, "move_item", map[string]any{"id": 1, "destination": "Garage > Toolbox"})
	item, ok := done["item"].(map[string]any)
	if !ok || item["location"] != "Garage > Toolbox" {
		t.Fatalf("move with the full path = %#v, want the toolbox", done)
	}
}

// callMCPResult is callMCP for the tests that need the whole tool result, not
// only its structured content.
func callMCPResult(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %s", name, textOf(result))
	}
	return result
}
