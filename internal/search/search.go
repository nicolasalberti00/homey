// Package search implements inventory search: it turns the candidates a
// repository returns into ranked results, each carrying the match that
// selected it and the full path of its location (rendered by the inventory's
// LocationIndex, so every caller names a place the same way).
//
// The rules of matching live in the inventory core (SearchTerms) and in the
// storage adapter (one query per search); this package owns what the results
// mean, so the REST API, the MCP server and the web UI present candidates the
// same way.
package search

import (
	"context"
	"slices"
	"strings"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Field is the item field a query term matched.
type Field string

const (
	FieldName        Field = "name"
	FieldAlias       Field = "alias"
	FieldTag         Field = "tag"
	FieldDescription Field = "description"
)

// Kind says how closely a field matched a term.
type Kind string

const (
	// KindExact means the field is the term.
	KindExact Kind = "exact"
	// KindPrefix means the field starts with the term.
	KindPrefix Kind = "prefix"
	// KindPartial means the field contains the term somewhere else.
	KindPartial Kind = "partial"
)

// Match is the best match between an item and the terms of a query: which
// field matched, and how closely.
type Match struct {
	Field Field
	Kind  Kind
}

// Result is one candidate: the item, the path of its location and the match
// that selected it.
type Result struct {
	Item  inventory.Item
	Path  string
	Match Match
}

// Engine answers search queries over the inventory. The repositories must be
// filled in; the zero value searches nothing.
type Engine struct {
	Items      inventory.ItemRepo
	Rooms      inventory.RoomRepo
	Containers inventory.ContainerRepo
}

// Search returns the candidates for query, best match first. Every candidate
// carries the full path of its location ("Garage > Toolbox > Drawer 1"), so a
// caller can tell two same-named items apart and a model can name where the
// item lives without another lookup.
func (e Engine) Search(ctx context.Context, query string) ([]Result, error) {
	terms := inventory.SearchTerms(query)
	if len(terms) == 0 {
		return []Result{}, nil
	}
	items, err := e.Items.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []Result{}, nil
	}
	index, err := e.locationIndex(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(items))
	for _, item := range items {
		match, _ := Classify(item, terms)
		results = append(results, Result{
			Item:  item,
			Path:  index.Path(item.Location),
			Match: match,
		})
	}
	// The candidates arrive name-ordered; the stable sort keeps that order
	// inside each rank, so equally good matches stay predictable.
	slices.SortStableFunc(results, func(a, b Result) int {
		return rankOf(a.Match) - rankOf(b.Match)
	})
	return results, nil
}

// Classify returns the best match between item and the terms of a query: the
// highest-ranked field, and the closest match inside it. Terms are lowercased
// here, so a caller can pass them as typed. The second result is false when no
// term matches, which cannot happen for candidates coming from a repository,
// but keeps the function total.
func Classify(item inventory.Item, terms []string) (Match, bool) {
	best, found := Match{}, false
	for _, term := range terms {
		term = inventory.FoldText(term)
		for _, field := range fieldOrder {
			for _, value := range fieldValues(item, field) {
				kind, ok := classify(value, term)
				if !ok {
					continue
				}
				if candidate := (Match{Field: field, Kind: kind}); !found || better(candidate, best) {
					best, found = candidate, true
				}
			}
		}
	}
	return best, found
}

// fieldOrder ranks the fields by how much a match there means: a name beats an
// alias, an alias a tag, a tag a description.
var fieldOrder = []Field{FieldName, FieldAlias, FieldTag, FieldDescription}

// kindOrder ranks the kinds inside a field: exact beats prefix, prefix beats
// partial.
var kindOrder = []Kind{KindExact, KindPrefix, KindPartial}

// better reports whether a ranks above b: the field first, then the kind.
func better(a, b Match) bool {
	if a.Field != b.Field {
		return fieldIndex(a.Field) < fieldIndex(b.Field)
	}
	return kindIndex(a.Kind) < kindIndex(b.Kind)
}

// rankOf turns a match into a number that sorts best first.
func rankOf(m Match) int {
	return fieldIndex(m.Field)*len(kindOrder) + kindIndex(m.Kind)
}

func fieldIndex(field Field) int {
	if index := slices.Index(fieldOrder, field); index >= 0 {
		return index
	}
	return len(fieldOrder)
}

func kindIndex(kind Kind) int {
	if index := slices.Index(kindOrder, kind); index >= 0 {
		return index
	}
	return len(kindOrder)
}

// fieldValues lists the strings of one field, in the order a match is looked
// for: the name, the aliases, the tags, the description.
func fieldValues(item inventory.Item, field Field) []string {
	switch field {
	case FieldName:
		return []string{item.Name}
	case FieldAlias:
		return item.Aliases
	case FieldTag:
		return item.Tags
	case FieldDescription:
		return []string{item.Description}
	}
	return nil
}

// classify compares one field value with one term. Both sides are folded, so
// an accent or a capital letter never decides how closely a field matched.
func classify(value, term string) (Kind, bool) {
	lower := inventory.FoldText(value)
	switch {
	case lower == term:
		return KindExact, true
	case strings.HasPrefix(lower, term):
		return KindPrefix, true
	case strings.Contains(lower, term):
		return KindPartial, true
	}
	return "", false
}

// locationIndex loads the rooms and containers once per search, so a result
// list does not pay one query per item.
func (e Engine) locationIndex(ctx context.Context) (inventory.LocationIndex, error) {
	rooms, err := e.Rooms.List(ctx)
	if err != nil {
		return inventory.LocationIndex{}, err
	}
	containers, err := e.Containers.List(ctx)
	if err != nil {
		return inventory.LocationIndex{}, err
	}
	return inventory.NewLocationIndex(rooms, containers), nil
}
