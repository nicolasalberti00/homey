package server

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nicolasalberti00/homey/internal/auth"
	"github.com/nicolasalberti00/homey/internal/storage"
	"github.com/nicolasalberti00/homey/internal/tools"
	mcpserver "github.com/nicolasalberti00/homey/mcp/server"
)

// mcpMethods are the methods of the Streamable HTTP transport: POST to send a
// message, GET to open the server-sent event stream, DELETE to end a session.
// They are registered one by one because a bare "/mcp" would be ambiguous next
// to the single-page app's "GET /": Go's mux refuses patterns where neither is
// more specific than the other.
var mcpMethods = []string{http.MethodGet, http.MethodPost, http.MethodDelete}

// mountMCP wires the MCP endpoint. When it is disabled the path answers 404
// rather than the single-page app's shell: /mcp is not a client route.
func mountMCP(mux *http.ServeMux, tokens auth.Store, logger *slog.Logger, enabled bool, repos storage.Repos) {
	var handler http.Handler = http.NotFoundHandler()
	if enabled {
		server := mcpserver.New(logger, toolRegistry(logger, repos))
		handler = mcpAuth(tokens, logger, mcpserver.Handler(server, logger))
	}
	for _, method := range mcpMethods {
		mux.Handle(method+" /mcp", handler)
	}
}

// toolRegistry builds the registry the endpoint serves. The definitions are
// static, so a failure here is a programming mistake: stopping the process is
// better than serving a server with a tool silently missing.
func toolRegistry(logger *slog.Logger, repos storage.Repos) *tools.Registry {
	inventory := tools.Inventory{Rooms: repos.Rooms, Containers: repos.Containers, Items: repos.Items}
	list, err := inventory.Tools()
	if err != nil {
		panic(fmt.Sprintf("building the inventory tools: %v", err))
	}
	registry, err := tools.NewRegistry(list...)
	if err != nil {
		panic(fmt.Sprintf("building the tool registry: %v", err))
	}
	logger.Debug("tools registered", "tools", len(registry.List()))
	return registry
}

// mcpAuth authenticates an MCP request with a bearer token. The MCP transport
// is a plain http.Handler rather than a Huma operation, so it needs its own
// check; the rejection is reported as a problem document, like the API's.
//
// Any valid token may connect for now: the tools that write arrive with the
// registry, and the per-tool permissions with it.
func mcpAuth(tokens auth.Store, logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plaintext, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeMCPAuthFailure(w, logger, r, "missing or malformed bearer token")
			return
		}
		if _, found := tokens.Authenticate(r.Context(), auth.Hash(plaintext)); !found {
			writeMCPAuthFailure(w, logger, r, "invalid or revoked token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeMCPAuthFailure answers with RFC 9457 fields and the WWW-Authenticate
// challenge a client needs to know that a token is what it is missing.
func writeMCPAuthFailure(w http.ResponseWriter, logger *slog.Logger, r *http.Request, reason string) {
	if logger != nil {
		logger.Warn("authentication rejected",
			"reason", reason,
			"remote", r.RemoteAddr,
			"method", r.Method,
			"path", r.URL.Path,
		)
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="homey"`)
	writeJSON(w, http.StatusUnauthorized, map[string]any{
		"title":  http.StatusText(http.StatusUnauthorized),
		"status": http.StatusUnauthorized,
		"detail": reason,
	})
}
