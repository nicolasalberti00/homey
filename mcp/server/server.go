// Package server hosts the MCP adapter: the server an MCP client connects to
// and, in the steps that follow, the tool registry and the handlers mapping
// MCP tool calls onto inventory operations.
//
// The transport is Streamable HTTP, served at /mcp by the HTTP layer.
package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicolasalberti00/homey/internal/tools"
)

// Name is the server name a client sees during initialization.
const Name = "homey"

// sessionTimeout closes a session no client has touched for a while, so a
// forgotten connection does not hold memory for the lifetime of the process.
const sessionTimeout = 30 * time.Minute

// instructions say what this server is for. They stay about the inventory and
// not about individual tools: each tool describes itself.
const instructions = "homey is a self-hosted home inventory: rooms hold containers, containers hold items. " +
	"Search before answering, and answer with the location path a result carries instead of guessing one."

// New builds the MCP server with the tools of registry, which may be nil for a
// server that offers none. It is cheap and holds no connection state: the
// transport owns the sessions.
func New(logger *slog.Logger, registry *tools.Registry) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: Name, Version: version()},
		&mcp.ServerOptions{Instructions: instructions, Logger: logger},
	)
	if registry != nil {
		registerTools(server, registry)
	}
	return server
}

// Handler serves server over the Streamable HTTP transport.
func Handler(server *mcp.Server, logger *slog.Logger) http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Logger: logger, SessionTimeout: sessionTimeout},
	)
}

// version reports the module version the binary was built from, so a client
// sees something meaningful without threading the build flag down here.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "devel"
	}
	return info.Main.Version
}
