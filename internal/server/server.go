// Package server wires homey's HTTP surface: health endpoints today, the
// /api/v1 REST API in Phase 3 and the MCP endpoint in Phase 6.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/nicolasalberti00/homey/internal/config"
	"github.com/nicolasalberti00/homey/internal/storage"
	"github.com/nicolasalberti00/homey/internal/webui"
)

const (
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
	readyTimeout      = 2 * time.Second
)

// New returns the HTTP server for cfg. The caller owns its lifecycle
// (ListenAndServe / Shutdown).
func New(cfg *config.Config, logger *slog.Logger, db *sql.DB) *http.Server {
	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           NewHandler(cfg, logger, db),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// NewHandler builds the root HTTP handler: security headers, CORS, rate
// limits and failed-auth blocking wrap the mux that carries health checks,
// the Huma API and the embedded UI. Kept separate from New so tests can
// exercise it with httptest.
func NewHandler(cfg *config.Config, logger *slog.Logger, db *sql.DB) http.Handler {
	return newHandler(cfg, logger, db, webui.Assets())
}

// newHandler is NewHandler with the UI files injected: production passes the
// embedded build, tests pass a fixture and stay independent of it.
func newHandler(cfg *config.Config, logger *slog.Logger, db *sql.DB, ui fs.FS) http.Handler {
	mux := http.NewServeMux()
	tokens := storage.NewTokenStore(db)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			logger.Warn("database not ready", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	NewAPI(mux, Deps{
		Repos:  storage.NewRepos(db),
		Tokens: tokens,
		Logger: logger,
	})

	// Everything else is the embedded single-page app (Step 4.9). The pattern
	// is method-scoped so unknown methods keep the mux's 405 behaviour.
	mux.Handle("GET /", newSPAHandler(ui))

	var handler http.Handler = mux
	handler = authFailureLimit(cfg.RateLimitAuthFailures, handler)
	handler = rateLimit(cfg.RateLimitWrites, handler)
	handler = cors(cfg.CORSOrigins, handler)
	handler = securityHeaders(handler)
	return requestLogger(logger, handler)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The connection may already be gone; there is nothing sensible left to
	// do with an encoding error.
	_ = json.NewEncoder(w).Encode(payload)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// requestLogger logs one line per request with method, path, status and
// duration.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
