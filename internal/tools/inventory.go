package tools

import (
	"context"
	"slices"
	"strings"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Inventory holds the repositories the inventory tools work through, so a tool
// stays what it should be: a definition plus a method that calls the core.
type Inventory struct {
	Rooms      inventory.RoomRepo
	Containers inventory.ContainerRepo
	Items      inventory.ItemRepo
}

// CountItemsInput is what count_items takes.
type CountItemsInput struct {
	Tag string `json:"tag,omitempty" jsonschema:"count only the items carrying this tag"`
}

// CountItemsOutput is what count_items answers.
type CountItemsOutput struct {
	Count int `json:"count" jsonschema:"how many items matched"`
}

// CountItems counts the items of the inventory, or only those carrying a tag.
func (inv Inventory) CountItems(ctx context.Context, input CountItemsInput) (CountItemsOutput, error) {
	items, err := inv.Items.List(ctx)
	if err != nil {
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

// Tools returns the tools of the inventory, each one a definition and a
// handler. Adding one is a definition here and a method above.
func (inv Inventory) Tools() ([]Tool, error) {
	countItems, err := New(Definition[CountItemsInput, CountItemsOutput]{
		Name: "count_items",
		Description: "Count the items in the home inventory, optionally only those carrying a tag. " +
			"Use it for questions like \"how many drills do I have?\" when the number is the answer and the items are not.",
		Permission: PermissionRead,
		Handler:    inv.CountItems,
	})
	if err != nil {
		return nil, err
	}
	return []Tool{countItems}, nil
}
