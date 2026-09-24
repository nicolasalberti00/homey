package tools

import (
	"context"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// AddItemInput is what add_item takes.
type AddItemInput struct {
	Name        string   `json:"name" jsonschema:"the name of the item, as the person calls it: \"Trapano\", \"Cacciavite a stella\""`
	Location    string   `json:"location" jsonschema:"where it goes: a room (\"Garage\") or a path (\"Garage > Toolbox\")"`
	Quantity    int      `json:"quantity,omitempty" jsonschema:"how many there are (default 1)"`
	Description string   `json:"description,omitempty" jsonschema:"what it is, in a few words"`
	Tags        []string `json:"tags,omitempty" jsonschema:"labels to find it by, such as \"officina\" or \"elettronica\""`
	Aliases     []string `json:"aliases,omitempty" jsonschema:"the other names it answers to, such as \"giravite\" for a cacciavite"`
	Notes       string   `json:"notes,omitempty" jsonschema:"free notes about the item"`
}

// UpdateItemInput is what update_item takes: only the fields to change, so a
// caller that does not mention a field leaves it as it is. Tags and aliases
// are replaced as a set.
type UpdateItemInput struct {
	ID          inventory.ItemID `json:"id" jsonschema:"the id of the item to change, as returned by search_inventory"`
	Name        *string          `json:"name,omitempty" jsonschema:"the new name"`
	Description *string          `json:"description,omitempty" jsonschema:"the new description"`
	Quantity    *int             `json:"quantity,omitempty" jsonschema:"the new quantity"`
	Tags        *[]string        `json:"tags,omitempty" jsonschema:"the new tags, replacing the ones stored"`
	Aliases     *[]string        `json:"aliases,omitempty" jsonschema:"the new aliases, replacing the ones stored"`
	Notes       *string          `json:"notes,omitempty" jsonschema:"the new notes"`
}

// MoveItemInput is what move_item takes.
type MoveItemInput struct {
	ID          inventory.ItemID `json:"id" jsonschema:"the id of the item to move, as returned by search_inventory"`
	Destination string           `json:"destination" jsonschema:"where it goes: a room (\"Garage\") or a path (\"Garage > Toolbox\")"`
}

// AddItemOutput is what add_item answers: the new item, or a clarification when
// the place named several and nothing was added.
type AddItemOutput struct {
	Item          *ItemView      `json:"item,omitempty" jsonschema:"the item as stored, id included; absent when a clarification is asked for"`
	Clarification *Clarification `json:"clarification,omitempty" jsonschema:"set when the place was ambiguous: ask which one, then call again with a full path"`
}

// MoveItemOutput is what move_item answers: the moved item, or a clarification
// when the destination named several and the item did not move.
type MoveItemOutput struct {
	Item          *ItemView      `json:"item,omitempty" jsonschema:"the item as stored after the move; absent when a clarification is asked for"`
	Clarification *Clarification `json:"clarification,omitempty" jsonschema:"set when the destination was ambiguous: ask which one, then call again with a full path"`
}

// AddItem puts a new item in a place that already exists, and answers with the
// item as stored, id included, so a caller can move or change it next. When the
// place named several it asks which one instead of guessing.
func (inv Inventory) AddItem(ctx context.Context, input AddItemInput) (AddItemOutput, error) {
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return AddItemOutput{}, err
	}
	location, err := index.Resolve(input.Location)
	if err != nil {
		if clarification, ok := clarify("location", input.Location, err); ok {
			return AddItemOutput{Clarification: clarification}, nil
		}
		return AddItemOutput{}, err
	}

	quantity := input.Quantity
	if quantity == 0 {
		quantity = 1
	}
	item := inventory.Item{
		Name:        input.Name,
		Description: input.Description,
		Quantity:    quantity,
		Notes:       input.Notes,
		Tags:        input.Tags,
		Aliases:     input.Aliases,
		Location:    location,
	}
	if err := inv.Items.Create(ctx, &item); err != nil {
		return AddItemOutput{}, err
	}
	view := itemView(item, index.Path(item.Location))
	return AddItemOutput{Item: &view}, nil
}

// UpdateItem changes the fields that were given and leaves the others alone.
// Where the item is stays put: moving is move_item.
func (inv Inventory) UpdateItem(ctx context.Context, input UpdateItemInput) (ItemView, error) {
	item, err := inv.Items.Get(ctx, input.ID)
	if err != nil {
		return ItemView{}, err
	}
	if input.Name != nil {
		item.Name = *input.Name
	}
	if input.Description != nil {
		item.Description = *input.Description
	}
	if input.Quantity != nil {
		item.Quantity = *input.Quantity
	}
	if input.Tags != nil {
		item.Tags = *input.Tags
	}
	if input.Aliases != nil {
		item.Aliases = *input.Aliases
	}
	if input.Notes != nil {
		item.Notes = *input.Notes
	}
	if err := inv.Items.Update(ctx, &item); err != nil {
		return ItemView{}, err
	}

	index, err := inv.locationIndex(ctx)
	if err != nil {
		return ItemView{}, err
	}
	return itemView(item, index.Path(item.Location)), nil
}

// MoveItem puts an item in another place without touching anything else. The
// destination is a room or a path such as "Garage > Toolbox", resolved the way
// a person names it. When the destination named several places it asks which
// one instead of guessing.
func (inv Inventory) MoveItem(ctx context.Context, input MoveItemInput) (MoveItemOutput, error) {
	index, err := inv.locationIndex(ctx)
	if err != nil {
		return MoveItemOutput{}, err
	}
	destination, err := index.Resolve(input.Destination)
	if err != nil {
		if clarification, ok := clarify("destination", input.Destination, err); ok {
			return MoveItemOutput{Clarification: clarification}, nil
		}
		return MoveItemOutput{}, err
	}
	if err := inv.Items.Move(ctx, input.ID, destination); err != nil {
		return MoveItemOutput{}, err
	}

	// Read the item back, so the answer is what storage holds rather than
	// what the move was asked to do.
	item, err := inv.Items.Get(ctx, input.ID)
	if err != nil {
		return MoveItemOutput{}, err
	}
	view := itemView(item, index.Path(item.Location))
	return MoveItemOutput{Item: &view}, nil
}
