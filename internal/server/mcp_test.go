package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
