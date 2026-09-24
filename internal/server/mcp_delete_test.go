package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicolasalberti00/homey/internal/auth"
)

// TestMCPDeleteAsksThenDeletes walks the confirmation flow over the wire: the
// first call proposes and leaves the item alone, the token from that answer
// carries the deletion out.
func TestMCPDeleteAsksThenDeletes(t *testing.T) {
	url, authorization := mcpTestServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization,
		`{"name":"Trapano","description":"Bosch","quantity":1,"location":{"kind":"room","id":1}}`)

	session := connectMCP(t, ctx, url, authorization)

	proposal := callMCP(t, ctx, session, "delete_item", map[string]any{"id": 1})
	if boolField(t, proposal, "deleted") {
		t.Fatal("delete_item removed the item before it was confirmed")
	}
	if got := stringField(t, proposal, "confirmation"); got != "pending" {
		t.Fatalf("confirmation = %q, want pending", got)
	}
	token := stringField(t, proposal, "confirmation_token")
	if token == "" {
		t.Fatal("the proposal came without a confirmation token")
	}
	if got := countField(t, callMCP(t, ctx, session, "count_items", nil)); got != 1 {
		t.Fatalf("count = %d, want the item still there", got)
	}

	done := callMCP(t, ctx, session, "delete_item", map[string]any{"id": 1, "confirmation_token": token})
	if !boolField(t, done, "deleted") {
		t.Fatal("the confirmed deletion did not happen")
	}
	if got := stringField(t, done, "confirmation"); got != "confirmed" {
		t.Fatalf("confirmation = %q, want confirmed", got)
	}
	if got := countField(t, callMCP(t, ctx, session, "count_items", nil)); got != 0 {
		t.Fatalf("count = %d, want the item gone", got)
	}
}

// TestMCPDeleteStaysProtectedForADefaultToken is the guard the step promises:
// a token whose policy is the default never deletes without confirmation.
func TestMCPDeleteStaysProtectedForADefaultToken(t *testing.T) {
	url, authorization := mcpTestServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization,
		`{"name":"Trapano","quantity":1,"location":{"kind":"room","id":1}}`)

	session := connectMCP(t, ctx, url, authorization)
	answer := callMCP(t, ctx, session, "delete_item", map[string]any{"id": 1})
	if boolField(t, answer, "deleted") {
		t.Fatal("a default token deleted without confirming")
	}
	if got := countField(t, callMCP(t, ctx, session, "count_items", nil)); got != 1 {
		t.Fatalf("count = %d, want the item still there", got)
	}
}

// TestMCPDeleteBypassesWithConfirm is the per-call bypass a model uses after
// the person has already said yes.
func TestMCPDeleteBypassesWithConfirm(t *testing.T) {
	url, authorization := mcpTestServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization,
		`{"name":"Trapano","quantity":1,"location":{"kind":"room","id":1}}`)

	session := connectMCP(t, ctx, url, authorization)
	done := callMCP(t, ctx, session, "delete_item", map[string]any{"id": 1, "confirm": true})
	if !boolField(t, done, "deleted") {
		t.Fatal("confirm=true did not delete")
	}
	if got := stringField(t, done, "confirmation"); got != "bypassed" {
		t.Fatalf("confirmation = %q, want bypassed", got)
	}
	if got := countField(t, callMCP(t, ctx, session, "count_items", nil)); got != 0 {
		t.Fatalf("count = %d, want the item gone", got)
	}
}

// TestMCPDeleteBypassesForATrustedToken is the per-client bypass: a token
// created with --destructive-confirmation=bypass deletes without the extra step.
func TestMCPDeleteBypassesForATrustedToken(t *testing.T) {
	url, tokens, authorization := mcpTestServerWith(t, auth.ConfirmationRequired)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// The inventory is filled through the REST API with the default token.
	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization,
		`{"name":"Trapano","quantity":1,"location":{"kind":"room","id":1}}`)

	// A second token, trusted to skip the confirmation step, drives the delete.
	trusted := strings.TrimPrefix(
		makeTokenHeaderPolicy(t, tokens, "automation", "read,write", auth.ConfirmationBypass),
		"Authorization: ")
	session := connectMCP(t, ctx, url, trusted)

	done := callMCP(t, ctx, session, "delete_item", map[string]any{"id": 1})
	if !boolField(t, done, "deleted") {
		t.Fatal("a trusted token still had to confirm")
	}
	if got := stringField(t, done, "confirmation"); got != "bypassed" {
		t.Fatalf("confirmation = %q, want bypassed", got)
	}
	if got := countField(t, callMCP(t, ctx, session, "count_items", nil)); got != 0 {
		t.Fatalf("count = %d, want the item gone", got)
	}
}

// connectMCP opens a session with the production transport, authenticating as
// the holder of authorization.
func connectMCP(t *testing.T, ctx context.Context, url, authorization string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "homey-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   url + "/mcp",
		HTTPClient: &http.Client{Transport: &headerTransport{authorization: authorization}},
	}, nil)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// callMCP calls one tool and returns its structured answer, failing the test if
// the call itself failed.
func callMCP(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("calling %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %s", name, textOf(result))
	}
	content, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("%s structured content = %#v, want an object", name, result.StructuredContent)
	}
	return content
}

// boolField reads a boolean out of a structured answer.
func boolField(t *testing.T, content map[string]any, key string) bool {
	t.Helper()
	value, ok := content[key].(bool)
	if !ok {
		t.Fatalf("%s = %#v, want a boolean", key, content[key])
	}
	return value
}

// stringField reads a string out of a structured answer.
func stringField(t *testing.T, content map[string]any, key string) string {
	t.Helper()
	value, ok := content[key].(string)
	if !ok {
		t.Fatalf("%s = %#v, want a string", key, content[key])
	}
	return value
}

// countField reads the count out of a count_items answer.
func countField(t *testing.T, content map[string]any) int {
	t.Helper()
	value, ok := content["count"].(float64)
	if !ok {
		t.Fatalf("count = %#v, want a number", content["count"])
	}
	return int(value)
}
