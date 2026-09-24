package tools

import (
	"context"
	"slices"
	"strings"

	"github.com/nicolasalberti00/homey/internal/inventory"
	"github.com/nicolasalberti00/homey/internal/search"
)

// ItemView is how an item is presented to a model: the item and where it is,
// in one answer, so a model can say "il trapano è in Garage > Toolbox" without
// another lookup.
type ItemView struct {
	ID           inventory.ItemID       `json:"id" jsonschema:"the item id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	Quantity     int                    `json:"quantity" jsonschema:"how many of this item there are"`
	Tags         []string               `json:"tags,omitempty" jsonschema:"the labels of the item"`
	Aliases      []string               `json:"aliases,omitempty" jsonschema:"the other names this item answers to"`
	Notes        string                 `json:"notes,omitempty"`
	Location     string                 `json:"location" jsonschema:"where the item is, as a path: \"Garage > Toolbox\""`
	LocationKind inventory.LocationKind `json:"location_kind" jsonschema:"whether the item sits in a room or in a container"`
	LocationID   int64                  `json:"location_id" jsonschema:"the id of the room or container the item sits in"`
}

// LocationView is how a container is presented to a model.
type LocationView struct {
	ID   inventory.ContainerID `json:"id"`
	Name string                `json:"name"`
	Path string                `json:"path" jsonschema:"the container as a path: \"Garage > Toolbox\""`
}

// RoomView is how a room is presented to a model.
type RoomView struct {
	ID          inventory.RoomID `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
}

// SearchInventoryInput is what search_inventory takes.
type SearchInventoryInput struct {
	Query string `json:"query" jsonschema:"what to look for, in the words of the person asking: \"trapano\", \"Bosch\", \"attrezzi\""`
	Limit int    `json:"limit,omitempty" jsonschema:"how many candidates to return at most (default 10, maximum 50)"`
}

// SearchInventoryOutput is what search_inventory answers.
type SearchInventoryOutput struct {
	Query     string         `json:"query"`
	Count     int            `json:"count" jsonschema:"how many items matched in total"`
	Truncated bool           `json:"truncated,omitempty" jsonschema:"true when the limit cut the results short"`
	Results   []SearchResult `json:"results" jsonschema:"the candidates, best match first"`
}

// SearchResult is one candidate: the item and why it matched.
type SearchResult struct {
	ItemView
	Match MatchView `json:"match" jsonschema:"why this item matched, so an answer can explain itself"`
}

// MatchView says which field matched and how closely.
type MatchView struct {
	Field string `json:"field" jsonschema:"the field that matched: name, alias, tag or description"`
	Kind  string `json:"kind" jsonschema:"how closely it matched: exact, prefix or partial"`
}

// CountItemsInput is what count_items takes.
type CountItemsInput struct {
	Tag      string `json:"tag,omitempty" jsonschema:"count only the items carrying this tag"`
	Location string `json:"location,omitempty" jsonschema:"count only the items in this room or container and in everything inside it (\"Garage\" or \"Garage > Toolbox\")"`
}

// CountItemsOutput is what count_items answers.
type CountItemsOutput struct {
	Count int `json:"count" jsonschema:"how many items matched"`
	// Clarification is set when the place named several and the count cannot
	// be taken; Count is then zero and means nothing.
	Clarification *Clarification `json:"clarification,omitempty" jsonschema:"set when the place was ambiguous: ask which one, then call again"`
}

// GetItemInput is what get_item takes.
type GetItemInput struct {
	ID inventory.ItemID `json:"id" jsonschema:"the id of the item, as returned by search_inventory"`
}

// ListLocationInput is what list_location takes.
type ListLocationInput struct {
	Location string `json:"location,omitempty" jsonschema:"a room (\"Garage\") or a path (\"Garage > Toolbox\"); leave it empty to list the rooms of the home"`
}

// ListLocationOutput is what list_location answers: what sits directly in one
// place.
type ListLocationOutput struct {
	Location   string         `json:"location,omitempty" jsonschema:"the place that was listed, as a path; \"Home\" when none was given"`
	Rooms      []RoomView     `json:"rooms,omitempty" jsonschema:"the rooms of the home, when no place was given"`
	Containers []LocationView `json:"containers,omitempty" jsonschema:"the containers directly inside the place"`
	Items      []ItemView     `json:"items,omitempty" jsonschema:"the items directly inside the place"`
	// Clarification is set when the place named several and nothing can be
	// listed yet.
	Clarification *Clarification `json:"clarification,omitempty" jsonschema:"set when the place was ambiguous: ask which one, then call again"`
}

// SearchInventory looks for items by name, alias, tag or description and
// answers with where each one is. It is the tool for "dov'è il trapano?" and
// for "do I have …?", because the candidates carry their location: answer from
// them instead of asking for one item after another.
func (inv Inventory) SearchInventory(ctx context.Context, input SearchInventoryInput) (SearchInventoryOutput, error) {
	limit := input.Limit
	switch {
	case limit <= 0:
		limit = defaultSearchLimit
	case limit > maxSearchLimit:
		limit = maxSearchLimit
	}

	engine := search.Engine{Items: inv.Items, Rooms: inv.Rooms, Containers: inv.Containers}
	found, err := engine.Search(ctx, input.Query)
	if err != nil {
		return SearchInventoryOutput{}, err
	}

	output := SearchInventoryOutput{
		Query:   input.Query,
		Count:   len(found),
		Results: make([]SearchResult, 0, min(limit, len(found))),
	}
	for _, result := range found[:min(limit, len(found))] {
		output.Results = append(output.Results, SearchResult{
			ItemView: itemView(result.Item, result.Path),
			Match:    MatchView{Field: string(result.Match.Field), Kind: string(result.Match.Kind)},
		})
	}
	output.Truncated = output.Count > len(output.Results)
	return output, nil
}

// GetItem answers with one item and where it is, for when its id is already
// known.
func (inv Inventory) GetItem(ctx context.Context, input GetItemInput) (ItemView, error) {
	item, err := inv.Items.Get(ctx, input.ID)
	if err != nil {
		return ItemView{}, err
	}
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return ItemView{}, err
	}
	return itemView(item, index.Path(item.Location)), nil
}

// ListLocation answers what sits directly in a room or a container, one level
// deep: a caller that needs to go deeper asks again for what it found. Without
// a location it lists the rooms, which is how a model learns what the home is
// made of.
func (inv Inventory) ListLocation(ctx context.Context, input ListLocationInput) (ListLocationOutput, error) {
	if strings.TrimSpace(input.Location) == "" {
		rooms, err := inv.Rooms.List(ctx)
		if err != nil {
			return ListLocationOutput{}, err
		}
		output := ListLocationOutput{Location: "Home", Rooms: make([]RoomView, 0, len(rooms))}
		for _, room := range rooms {
			output.Rooms = append(output.Rooms, RoomView{ID: room.ID, Name: room.Name, Description: room.Description})
		}
		return output, nil
	}

	index, err := inv.locationIndex(ctx)
	if err != nil {
		return ListLocationOutput{}, err
	}
	location, err := index.Resolve(input.Location)
	if err != nil {
		if clarification, ok := clarify("location", input.Location, err); ok {
			return ListLocationOutput{Clarification: clarification}, nil
		}
		return ListLocationOutput{}, err
	}
	items, err := inv.Items.ListByLocation(ctx, location)
	if err != nil {
		return ListLocationOutput{}, err
	}
	children := index.Children(location)

	output := ListLocationOutput{
		Location:   index.Path(location),
		Containers: make([]LocationView, 0, len(children)),
		Items:      make([]ItemView, 0, len(items)),
	}
	for _, container := range children {
		output.Containers = append(output.Containers, LocationView{
			ID:   container.ID,
			Name: container.Name,
			Path: index.Path(inventory.ContainerLocation(container.ID)),
		})
	}
	for _, item := range items {
		output.Items = append(output.Items, itemView(item, index.Path(item.Location)))
	}
	return output, nil
}

// CountItems counts the items of the inventory, or only those carrying a tag,
// or only those a place holds, for when the number is the answer and the items
// are not.
func (inv Inventory) CountItems(ctx context.Context, input CountItemsInput) (CountItemsOutput, error) {
	items, err := inv.candidates(ctx, input.Location)
	if err != nil {
		if clarification, ok := clarify("location", input.Location, err); ok {
			return CountItemsOutput{Clarification: clarification}, nil
		}
		return CountItemsOutput{}, err
	}
	if input.Tag == "" {
		return CountItemsOutput{Count: len(items)}, nil
	}
	matches := 0
	for _, item := range items {
		if slices.ContainsFunc(item.Tags, func(tag string) bool { return strings.EqualFold(tag, input.Tag) }) {
			matches++
		}
	}
	return CountItemsOutput{Count: matches}, nil
}

// candidates returns every item, or only those a place holds: the place
// itself and everything inside it, at any depth, because "quante cose ci sono
// in garage?" counts the toolbox too.
func (inv Inventory) candidates(ctx context.Context, location string) ([]inventory.Item, error) {
	items, err := inv.Items.List(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(location) == "" {
		return items, nil
	}
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return nil, err
	}
	place, err := index.Resolve(location)
	if err != nil {
		return nil, err
	}

	held := make([]inventory.Item, 0, len(items))
	for _, item := range items {
		if slices.Contains(index.Ancestors(item.Location), place) {
			held = append(held, item)
		}
	}
	return held, nil
}

// locationIndex loads the rooms and containers once and indexes them, so a
// tool resolves a name and renders a path from the same view of the inventory.
func (inv Inventory) locationIndex(ctx context.Context) (inventory.LocationIndex, error) {
	rooms, err := inv.Rooms.List(ctx)
	if err != nil {
		return inventory.LocationIndex{}, err
	}
	containers, err := inv.Containers.List(ctx)
	if err != nil {
		return inventory.LocationIndex{}, err
	}
	return inventory.NewLocationIndex(rooms, containers), nil
}

// itemView presents an item with the path of its location, which the caller
// renders from the index it resolved the location with.
func itemView(item inventory.Item, path string) ItemView {
	return ItemView{
		ID:           item.ID,
		Name:         item.Name,
		Description:  item.Description,
		Quantity:     item.Quantity,
		Tags:         item.Tags,
		Aliases:      item.Aliases,
		Notes:        item.Notes,
		Location:     path,
		LocationKind: item.Location.Kind,
		LocationID:   item.Location.ID,
	}
}
