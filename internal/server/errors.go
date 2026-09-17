package server

// One mapping from inventory core errors to the API's RFC 9457 problem
// responses keeps a single error shape across every operation. Handlers
// never write status codes for domain errors themselves.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// mapError turns a core error into the response error. Unknown errors become
// an opaque 500 whose internals never leak: the real error is logged instead.
func mapError(deps Deps, err error) error {
	if err == nil {
		return nil
	}

	var validation *inventory.ValidationError
	switch {
	case errors.Is(err, inventory.ErrNotFound):
		return huma.Error404NotFound(notFoundDetail(err))
	case errors.Is(err, inventory.ErrConflict):
		return huma.Error409Conflict(detail(err, inventory.ErrConflict, "the operation conflicts with the current state"))
	case errors.Is(err, inventory.ErrCycle):
		return huma.Error409Conflict(detail(err, inventory.ErrCycle, "the operation would create a cycle"))
	case errors.As(err, &validation):
		problem := &huma.ErrorModel{
			Status: http.StatusUnprocessableEntity,
			Detail: "the request does not satisfy the inventory rules",
		}
		for _, p := range validation.Problems {
			problem.Add(&huma.ErrorDetail{
				Location: "body." + p.Field,
				Message:  p.Problem,
			})
		}
		return problem
	default:
		if deps.Logger != nil {
			deps.Logger.Error("internal error", "error", err)
		}
		return huma.Error500InternalServerError("internal error")
	}
}

// notFoundDetail composes the problem detail for a missing entity: the core
// wraps the lookup context, e.g. "room 3: inventory: not found" becomes
// "room 3 not found".
func notFoundDetail(err error) string {
	context := strings.TrimSuffix(err.Error(), ": "+inventory.ErrNotFound.Error())
	if context == err.Error() {
		return "the requested entity does not exist"
	}
	return context + " not found"
}

// detail strips the ": inventory: <sentinel>" tail from err's message, so
// the wrapping context becomes the problem detail; fallback is used when
// nothing meaningful is wrapped.
func detail(err error, sentinel error, fallback string) string {
	msg := strings.TrimSuffix(err.Error(), ": "+sentinel.Error())
	if msg == err.Error() || msg == "" {
		return fallback
	}
	return msg
}
