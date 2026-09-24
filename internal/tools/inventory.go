package tools

import (
	"github.com/nicolasalberti00/homey/internal/inventory"
)

// Inventory holds the repositories the inventory tools work through, so a tool
// stays what it should be: a definition plus a method that calls the core.
type Inventory struct {
	Rooms      inventory.RoomRepo
	Containers inventory.ContainerRepo
	Items      inventory.ItemRepo
}

const (
	// defaultSearchLimit is how many candidates search_inventory returns when
	// the caller does not say.
	defaultSearchLimit = 10
	// maxSearchLimit caps the candidates, so one call cannot flood a caller
	// with the whole inventory.
	maxSearchLimit = 50
)

// Tools returns the tools of the inventory, each one a definition and a
// handler. Adding a tool is a definition here and a method in this package.
func (inv Inventory) Tools() ([]Tool, error) {
	searchInventory, err := New(Definition[SearchInventoryInput, SearchInventoryOutput]{
		Name: "search_inventory",
		Description: "Search the home inventory by name, alias, tag or description and get where each match is. " +
			"Use it whenever someone asks where something is (\"dov'è il trapano?\", \"dove tengo le batterie?\") " +
			"or whether they own it: every result carries the location path, the quantity and the tags, so one call is usually enough. " +
			"Several candidates can come back; when they are different items with the same name, ask which one is meant instead of picking one.",
		Permission: PermissionRead,
		Handler:    inv.SearchInventory,
	})
	if err != nil {
		return nil, err
	}

	getItem, err := New(Definition[GetItemInput, ItemView]{
		Name: "get_item",
		Description: "Read one item by id, with everything stored about it: description, quantity, tags, aliases, notes and its location path. " +
			"Use it when the id is already known, from search_inventory or from the person asking.",
		Permission: PermissionRead,
		Handler:    inv.GetItem,
	})
	if err != nil {
		return nil, err
	}

	listLocation, err := New(Definition[ListLocationInput, ListLocationOutput]{
		Name: "list_location",
		Description: "List what sits directly in a room or in a container: the containers inside it and the items in it, one level deep. " +
			"Without a location it lists the rooms of the home, which is how to find out what the home is made of. " +
			"Use it for \"cosa c'è in garage?\" and \"cosa c'è nel cassetto 1?\"; to find a specific item, search_inventory is quicker.",
		Permission: PermissionRead,
		Handler:    inv.ListLocation,
	})
	if err != nil {
		return nil, err
	}

	countItems, err := New(Definition[CountItemsInput, CountItemsOutput]{
		Name: "count_items",
		Description: "Count the items of the home inventory, optionally only those carrying a tag or only those a room or container holds, " +
			"including everything inside its containers. " +
			"Use it when the number is the answer (\"quanti trapani ho?\", \"quante cose ci sono in garage?\"); " +
			"when the items themselves are wanted, use search_inventory or list_location.",
		Permission: PermissionRead,
		Handler:    inv.CountItems,
	})
	if err != nil {
		return nil, err
	}

	addItem, err := New(Definition[AddItemInput, ItemView]{
		Name: "add_item",
		Description: "Add an item to the home inventory, in a room or in a container that already exists. " +
			"Use it when someone says they put something away (\"ho messo il trapano nel toolbox\") or that it should be recorded. " +
			"The answer carries the new id, so the item can be changed or moved next. If a place with that name does not exist, " +
			"list_location shows what there is; if an item with the same name is already there, the call fails instead of overwriting it.",
		Permission: PermissionWrite,
		Handler:    inv.AddItem,
	})
	if err != nil {
		return nil, err
	}

	updateItem, err := New(Definition[UpdateItemInput, ItemView]{
		Name: "update_item",
		Description: "Change what is stored about an item: its name, description, quantity, tags, aliases or notes. " +
			"Only the fields given are changed, the others stay as they are, and tags and aliases replace the whole set. " +
			"Where the item is does not change here: use move_item for that.",
		Permission:  PermissionWrite,
		Destructive: true,
		Handler:     inv.UpdateItem,
	})
	if err != nil {
		return nil, err
	}

	moveItem, err := New(Definition[MoveItemInput, ItemView]{
		Name: "move_item",
		Description: "Move an item to another room or container, leaving everything else about it alone. " +
			"The destination is named the way a person names it: \"Garage\" or \"Garage > Toolbox\". " +
			"Use it for \"metti il trapano nel toolbox in garage\" once the item id is known from search_inventory.",
		Permission:  PermissionWrite,
		Destructive: true,
		Handler:     inv.MoveItem,
	})
	if err != nil {
		return nil, err
	}

	return []Tool{searchInventory, getItem, listLocation, countItems, addItem, updateItem, moveItem}, nil
}
