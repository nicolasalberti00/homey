package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicolasalberti00/homey/internal/storage"
	mcpserver "github.com/nicolasalberti00/homey/mcp/server"
)

// initializeRequest is what a client sends first, hand-written so the test
// exercises the transport instead of the SDK's own client.
const initializeRequest = `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
	`"params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

// mcpTestServer starts the production handler chain with the MCP endpoint on
// and returns its URL with the Authorization header value of a write token.
func mcpTestServer(t *testing.T) (url, authorization string) {
	t.Helper()
	cfg := defaultTestConfig(t)
	cfg.MCPEnabled = true
	handler, db := newTestHandlerCfg(t, cfg)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// makeTokenHeader returns the whole "Authorization: Bearer …" line, which
	// is what humatest wants; a header value is what the transport sets.
	line := makeTokenHeader(t, storage.NewTokenStore(db), "mcp test", "read,write")
	return srv.URL, strings.TrimPrefix(line, "Authorization: ")
}

// TestMCPClientConnects is the smoke test of the endpoint: a real MCP client
// speaks the Streamable HTTP transport through the production middleware chain
// and initializes.
func TestMCPClientConnects(t *testing.T) {
	url, authorization := mcpTestServer(t)
	// A client that cannot bring its stream up would otherwise hang here: the
	// deadline turns that into a failure with a name.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "homey-test", Version: "0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint: url + "/mcp",
		HTTPClient: &http.Client{
			Transport: &headerTransport{authorization: authorization},
		},
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer session.Close()

	result := session.InitializeResult()
	if result == nil {
		t.Fatal("initialization returned no result")
	}
	if result.ServerInfo.Name != mcpserver.Name {
		t.Fatalf("server name = %q, want %q", result.ServerInfo.Name, mcpserver.Name)
	}
}

// TestMCPRejectsBadTokens checks the endpoint is never open: without a token,
// with a malformed one and with an unknown one it answers a problem document
// and the challenge a client needs.
func TestMCPRejectsBadTokens(t *testing.T) {
	url, _ := mcpTestServer(t)

	cases := []struct {
		name          string
		authorization string
		want          string
	}{
		{"no token", "", "missing or malformed bearer token"},
		{"malformed", "Bearer", "missing or malformed bearer token"},
		{"unknown", "Bearer not-a-token", "invalid or revoked token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/mcp", strings.NewReader(initializeRequest))
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("requesting: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if challenge := resp.Header.Get("WWW-Authenticate"); challenge == "" {
				t.Fatal("no WWW-Authenticate challenge")
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading the body: %v", err)
			}
			if !strings.Contains(string(body), tc.want) {
				t.Fatalf("body = %s, want %q", body, tc.want)
			}
		})
	}
}

// TestMCPDisabledAnswersNotFound: /mcp is not a client route, so a disabled
// endpoint says so instead of answering with the single-page app's shell.
func TestMCPDisabledAnswersNotFound(t *testing.T) {
	cfg := defaultTestConfig(t)
	cfg.MCPEnabled = false
	handler, _ := newTestHandlerCfg(t, cfg)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+"/mcp", strings.NewReader(""))
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("requesting: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("%s /mcp = %d, want 404", method, resp.StatusCode)
			}
		})
	}
}

// headerTransport adds the bearer header to every request, the way an MCP
// client configured with a token does.
type headerTransport struct {
	authorization string
}

func (h *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", h.authorization)
	return http.DefaultTransport.RoundTrip(req)
}

// TestMCPClientListsAndCallsTools walks the whole path: the registry describes
// the tool, the transport carries it, and the call reaches the same database
// the REST API writes to.
func TestMCPClientListsAndCallsTools(t *testing.T) {
	url, authorization := mcpTestServer(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// One item, created through the REST API with the same token.
	postJSON(t, ctx, url+"/api/v1/rooms", authorization, `{"name":"Garage"}`)
	postJSON(t, ctx, url+"/api/v1/items", authorization, `{"name":"Trapano","quantity":1,"location":{"kind":"room","id":1}}`)

	client := mcp.NewClient(&mcp.Implementation{Name: "homey-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   url + "/mcp",
		HTTPClient: &http.Client{Transport: &headerTransport{authorization: authorization}},
	}, nil)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer session.Close()

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("listing tools: %v", err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	// The tool surface is a contract with the clients.
	want := []string{"count_items", "get_item", "list_location", "search_inventory"}
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
	for _, tool := range list.Tools {
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("%s came without a schema", tool.Name)
		}
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "count_items"})
	if err != nil {
		t.Fatalf("calling count_items: %v", err)
	}
	if result.IsError {
		t.Fatalf("count_items failed: %s", textOf(result))
	}
	if got := structuredCount(t, result); got != 1 {
		t.Fatalf("count_items = %d, want the item the API created", got)
	}

	// The question the read tools exist for, over the wire.
	found, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_inventory",
		Arguments: map[string]any{"query": "trapano"},
	})
	if err != nil {
		t.Fatalf("calling search_inventory: %v", err)
	}
	if found.IsError {
		t.Fatalf("search_inventory failed: %s", textOf(found))
	}
	if got := structuredSearch(t, found); got != "Garage" {
		t.Fatalf("search_inventory put the drill in %q, want the room the API created", got)
	}

	// Arguments that do not fit the schema come back as a tool error, which is
	// what the protocol asks a server to hand a model.
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "count_items",
		Arguments: map[string]any{"tag": 7},
	})
	if err != nil {
		t.Fatalf("calling count_items with a bad argument: %v", err)
	}
	if !bad.IsError {
		t.Fatal("a bad argument was not reported as a tool error")
	}
	if text := textOf(bad); !strings.Contains(text, "invalid tool input") {
		t.Fatalf("tool error = %q, want the validation message", text)
	}
}

// structuredSearch reads the location of the first search result out of a tool
// result.
func structuredSearch(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	content, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %#v, want an object", result.StructuredContent)
	}
	results, ok := content["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("results = %#v, want at least one", content["results"])
	}
	first, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] = %#v, want an object", results[0])
	}
	location, ok := first["location"].(string)
	if !ok {
		t.Fatalf("location = %#v, want a path", first["location"])
	}
	return location
}

// postJSON sends a request to the REST API and fails the test unless it is
// accepted.
func postJSON(t *testing.T, ctx context.Context, url, authorization, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("posting to %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s = %d: %s", url, resp.StatusCode, payload)
	}
}

// structuredCount reads the count out of a tool result.
func structuredCount(t *testing.T, result *mcp.CallToolResult) int {
	t.Helper()
	content, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %#v, want an object", result.StructuredContent)
	}
	count, ok := content["count"].(float64)
	if !ok {
		t.Fatalf("count = %#v, want a number", content["count"])
	}
	return int(count)
}

// textOf returns the text a result carries, which is what a model reads.
func textOf(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if typed, ok := content.(*mcp.TextContent); ok {
			text.WriteString(typed.Text)
		}
	}
	return text.String()
}
