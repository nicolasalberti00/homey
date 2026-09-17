package server

// The search endpoint is part of the application contract from day one, but
// its implementation arrives with Phase 5 (aliases, tags, ranking). Until
// then it answers with a stable 501 problem so clients can build against
// the shape.

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// RegisterSearch adds the search operation to the API.
func RegisterSearch(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "search",
		Method:      http.MethodGet,
		Path:        "/search",
		Summary:     "Search the inventory",
		Description: "Not implemented yet: the query is accepted and the endpoint answers with 501 until Phase 5 delivers it.",
		Tags:        []string{"Search"},
	}, func(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
		return nil, &huma.ErrorModel{
			Status: http.StatusNotImplemented,
			Title:  "Not Implemented",
			Detail: "search is planned for Phase 5 of the roadmap",
		}
	})
}

type SearchInput struct {
	Query string `query:"q" required:"true" minLength:"1" example:"trapano" doc:"Free-text query."`
}

type SearchOutput struct {
	Body struct{} `json:"body"`
}
