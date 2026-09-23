package server

// The search operation: it answers with the items whose name, description,
// aliases or tags contain the query. Ranking is not part of the query yet, so
// the response stays a plain list of items.

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
		Description: "Matches the query against item names, descriptions, aliases and tags, case-insensitively, and returns the matches ordered by name.",
		Tags:        []string{"Search"},
	}, func(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
		items, err := deps.Repos.Items.Search(ctx, input.Query)
		if err != nil {
			return nil, mapError(deps, err)
		}
		out := make([]ItemResponse, len(items))
		for index, item := range items {
			out[index] = newItemResponse(item)
		}
		return &SearchOutput{Body: out}, nil
	})
}

type SearchInput struct {
	Query string `query:"q" required:"true" minLength:"1" maxLength:"200" example:"trapano" doc:"Free-text query."`
}

type SearchOutput struct {
	Body []ItemResponse `json:"body"`
}
