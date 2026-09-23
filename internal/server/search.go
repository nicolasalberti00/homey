package server

// The search operation: it answers with the candidates for a query, best
// match first, each carrying the path of its location and the match that
// selected it, so a client (or a model) can tell two same-named items apart.

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/nicolasalberti00/homey/internal/search"
)

// RegisterSearch adds the search operation to the API.
func RegisterSearch(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "search",
		Method:      http.MethodGet,
		Path:        "/search",
		Summary:     "Search the inventory",
		Description: "Matches every term of the query against item names, descriptions, aliases and tags. Candidates come back best match first: an exact match before a prefix, a prefix before a partial one, and a name before an alias, a tag and a description.",
		Tags:        []string{"Search"},
	}, func(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
		engine := search.Engine{
			Items:      deps.Repos.Items,
			Rooms:      deps.Repos.Rooms,
			Containers: deps.Repos.Containers,
		}
		results, err := engine.Search(ctx, input.Query)
		if err != nil {
			return nil, mapError(deps, err)
		}
		out := make([]SearchResult, len(results))
		for index, result := range results {
			out[index] = SearchResult{
				Item: newItemResponse(result.Item),
				Path: result.Path,
				Match: SearchMatch{
					Field: string(result.Match.Field),
					Kind:  string(result.Match.Kind),
				},
			}
		}
		return &SearchOutput{Body: out}, nil
	})
}

type SearchInput struct {
	Query string `query:"q" required:"true" minLength:"1" maxLength:"200" example:"trapano" doc:"Free-text query; every term must match."`
}

// SearchMatch says why a candidate matched.
type SearchMatch struct {
	Field string `json:"field" enum:"name,alias,tag,description" doc:"Item field the query matched."`
	Kind  string `json:"kind" enum:"exact,prefix,partial" doc:"How closely the field matched: the field is the term, starts with it, or contains it somewhere else."`
}

// SearchResult is one candidate: the item, the path of its location and the
// match that selected it.
type SearchResult struct {
	Item  ItemResponse `json:"item"`
	Path  string       `json:"path" example:"Garage > Toolbox" doc:"Full path of the location, so same-named items can be told apart."`
	Match SearchMatch  `json:"match"`
}

type SearchOutput struct {
	Body []SearchResult `json:"body"`
}
