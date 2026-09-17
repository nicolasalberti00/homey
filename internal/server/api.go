package server

// The REST API surface is built code-first with Huma v2: the Go input/output
// types of the operations define both request validation and the OpenAPI 3.1
// document, which is generated — never hand-maintained. Operations mount
// under /api/v1 through the humago prefix adapter, so the document stays
// relative and lists servers: [{url: "/api/v1"}].
//
// The document is served at /api/v1/openapi.json|.yaml, rendered as a docs UI
// at /api/v1/docs, and exported to the committed artifact api/openapi.yaml by
// `go run ./cmd/openapi`; CI regenerates it and fails the build on any drift.
//
// Operations register on the returned API with huma.Register; business logic
// stays in internal/inventory.

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/nicolasalberti00/homey/internal/storage"
)

// apiVersion is the version reported by the generated OpenAPI document. It
// is fixed here rather than taken from the build-time version so the
// committed artifact stays deterministic.
const apiVersion = "0.1.0"

// Deps carries what the API operations need. Handlers receive it at
// registration time and run it per request; the zero value is a valid,
// spec-only API, which is what the openapi generator uses.
type Deps struct {
	Repos  storage.Repos
	Logger *slog.Logger
}

// NewAPI mounts the /api/v1 REST API on mux and returns the Huma API.
func NewAPI(mux *http.ServeMux, deps Deps) huma.API {
	cfg := huma.DefaultConfig("homey API", apiVersion)
	cfg.Info.Description = "REST API for homey, the self-hosted home inventory: rooms, nested containers, items and moves."
	cfg.OpenAPIPath = "/openapi"
	cfg.DocsPath = "/docs"
	cfg.Servers = []*huma.Server{{URL: "/api/v1", Description: "Versioned API root"}}

	return humago.NewWithPrefix(mux, "/api/v1", cfg)
}
