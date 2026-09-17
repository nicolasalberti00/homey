package server

// Bearer-token authentication for every registered operation, as a Huma
// middleware: read methods accept any valid token, mutating methods require
// the write scope. The OpenAPI document, the docs UI and the schemas are
// served by raw adapter routes and never reach the middleware stack, so they
// stay public.

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/nicolasalberti00/homey/internal/auth"
)

// bearerAuth returns a Huma middleware that authenticates the request.
// Authentication failures answer with the same RFC 9457 problem shape the
// handlers use.
func bearerAuth(tokens auth.Store) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		plaintext, ok := bearerToken(ctx.Header("Authorization"))
		if !ok {
			writeAuthProblem(ctx, http.StatusUnauthorized, "missing or malformed bearer token")
			return
		}
		token, found := tokens.Authenticate(ctx.Context(), auth.Hash(plaintext))
		if !found {
			writeAuthProblem(ctx, http.StatusUnauthorized, "invalid or revoked token")
			return
		}
		if isWriteMethod(ctx.Method()) && !token.CanWrite() {
			writeAuthProblem(ctx, http.StatusForbidden,
				"this token only grants read access; use a token with the write scope")
			return
		}
		next(ctx)
	}
}

// bearerToken extracts the plaintext token from the Authorization header.
func bearerToken(header string) (string, bool) {
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

// isWriteMethod reports whether the method mutates state.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// writeAuthProblem writes an RFC 9457 problem for authentication failures.
func writeAuthProblem(ctx huma.Context, status int, detail string) {
	ctx.SetStatus(status)
	ctx.SetHeader("Content-Type", "application/problem+json")
	if status == http.StatusUnauthorized {
		ctx.SetHeader("WWW-Authenticate", `Bearer realm="homey"`)
	}
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(map[string]any{
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}
