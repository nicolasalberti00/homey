package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// itemRepo is the SQLite implementation of inventory.ItemRepo.
type itemRepo struct {
	db *sql.DB
}

const (
	insertItemSQL = `
INSERT INTO items (room_id, container_id, name, description, quantity, notes)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, room_id, container_id, name, description, quantity, notes, created_at, updated_at`

	getItemSQL = `
SELECT id, room_id, container_id, name, description, quantity, notes, created_at, updated_at
FROM items
WHERE id = ?`

	listItemsSQL = `
SELECT id, room_id, container_id, name, description, quantity, notes, created_at, updated_at
FROM items
ORDER BY name COLLATE NOCASE, id`

	listItemsByRoomSQL = `
SELECT id, room_id, container_id, name, description, quantity, notes, created_at, updated_at
FROM items
WHERE room_id = ?
ORDER BY name COLLATE NOCASE, id`

	listItemsByContainerSQL = `
SELECT id, room_id, container_id, name, description, quantity, notes, created_at, updated_at
FROM items
WHERE container_id = ?
ORDER BY name COLLATE NOCASE, id`

	// LIKE is case-insensitive for ASCII by default, which is the same
	// folding the COLLATE NOCASE ordering uses. The query is passed as a
	// pattern with its wildcards escaped (see likePattern).
	searchItemsSQL = `
SELECT id, room_id, container_id, name, description, quantity, notes, created_at, updated_at
FROM items
WHERE name LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\'
ORDER BY name COLLATE NOCASE, id`

	updateItemSQL = `
UPDATE items
SET room_id = ?,
    container_id = ?,
    name = ?,
    description = ?,
    quantity = ?,
    notes = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING id, room_id, container_id, name, description, quantity, notes, created_at, updated_at`

	deleteItemSQL = `DELETE FROM items WHERE id = ? RETURNING id`

	moveItemSQL = `
UPDATE items
SET room_id = ?,
    container_id = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING id`

	insertItemTagSQL = `INSERT INTO item_tags (item_id, tag) VALUES (?, ?)`

	deleteItemTagsSQL = `DELETE FROM item_tags WHERE item_id = ?`

	getItemTagsSQL = `
SELECT tag
FROM item_tags
WHERE item_id = ?
ORDER BY tag COLLATE NOCASE, tag`

	listItemTagsSQL = `
SELECT it.item_id, it.tag
FROM item_tags it
WHERE it.item_id IN (SELECT id FROM items)
ORDER BY it.item_id, it.tag COLLATE NOCASE, it.tag`

	listItemTagsByRoomSQL = `
SELECT it.item_id, it.tag
FROM item_tags it
WHERE it.item_id IN (SELECT id FROM items WHERE room_id = ?)
ORDER BY it.item_id, it.tag COLLATE NOCASE, it.tag`

	listItemTagsByContainerSQL = `
SELECT it.item_id, it.tag
FROM item_tags it
WHERE it.item_id IN (SELECT id FROM items WHERE container_id = ?)
ORDER BY it.item_id, it.tag COLLATE NOCASE, it.tag`

	searchItemTagsSQL = `
SELECT it.item_id, it.tag
FROM item_tags it
WHERE it.item_id IN (
	SELECT id FROM items WHERE name LIKE ? ESCAPE '\' OR description LIKE ? ESCAPE '\'
)
ORDER BY it.item_id, it.tag COLLATE NOCASE, it.tag`
)

// Create implements inventory.ItemRepo.
func (r *itemRepo) Create(ctx context.Context, item *inventory.Item) error {
	item.Normalize()
	if err := item.Validate(); err != nil {
		return err
	}
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := requireLocation(ctx, tx, item.Location); err != nil {
			return err
		}
		roomID, containerID := locationColumns(item.Location)
		row := tx.QueryRowContext(ctx, insertItemSQL,
			roomID, containerID, item.Name, item.Description, item.Quantity, item.Notes)
		if err := scanItem(row, item); err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("an item named %q already exists at this location: %w",
					item.Name, inventory.ErrConflict)
			}
			return fmt.Errorf("creating item: %w", err)
		}
		if err := replaceItemTags(ctx, tx, item.ID, item.Tags); err != nil {
			return err
		}
		return loadItemTags(ctx, tx, item)
	})
}

// Get implements inventory.ItemRepo.
func (r *itemRepo) Get(ctx context.Context, id inventory.ItemID) (inventory.Item, error) {
	var item inventory.Item
	err := scanItem(r.db.QueryRowContext(ctx, getItemSQL, id), &item)
	if errors.Is(err, sql.ErrNoRows) {
		return inventory.Item{}, notFound("item", int64(id))
	}
	if err != nil {
		return inventory.Item{}, fmt.Errorf("getting item %d: %w", id, err)
	}
	if err := loadItemTags(ctx, r.db, &item); err != nil {
		return inventory.Item{}, err
	}
	return item, nil
}

// List implements inventory.ItemRepo.
func (r *itemRepo) List(ctx context.Context) ([]inventory.Item, error) {
	return queryItems(ctx, r.db, listItemsSQL, listItemTagsSQL)
}

// ListByLocation implements inventory.ItemRepo.
func (r *itemRepo) ListByLocation(ctx context.Context, location inventory.Location) ([]inventory.Item, error) {
	if err := location.Validate(); err != nil {
		return nil, err
	}
	if err := requireLocation(ctx, r.db, location); err != nil {
		return nil, err
	}
	if location.IsRoom() {
		return queryItems(ctx, r.db, listItemsByRoomSQL, listItemTagsByRoomSQL, location.ID)
	}
	return queryItems(ctx, r.db, listItemsByContainerSQL, listItemTagsByContainerSQL, location.ID)
}

// Search implements inventory.ItemRepo.
func (r *itemRepo) Search(ctx context.Context, query string) ([]inventory.Item, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return []inventory.Item{}, nil
	}
	pattern := likePattern(trimmed)
	return queryItems(ctx, r.db, searchItemsSQL, searchItemTagsSQL, pattern, pattern)
}

// Update implements inventory.ItemRepo.
func (r *itemRepo) Update(ctx context.Context, item *inventory.Item) error {
	item.Normalize()
	if err := item.Validate(); err != nil {
		return err
	}
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := requireLocation(ctx, tx, item.Location); err != nil {
			return err
		}
		roomID, containerID := locationColumns(item.Location)
		row := tx.QueryRowContext(ctx, updateItemSQL,
			roomID, containerID, item.Name, item.Description, item.Quantity, item.Notes, item.ID)
		err := scanItem(row, item)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("item", int64(item.ID))
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("an item named %q already exists at this location: %w",
				item.Name, inventory.ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("updating item %d: %w", item.ID, err)
		}
		if err := replaceItemTags(ctx, tx, item.ID, item.Tags); err != nil {
			return err
		}
		return loadItemTags(ctx, tx, item)
	})
}

// Move implements inventory.ItemRepo.
func (r *itemRepo) Move(ctx context.Context, id inventory.ItemID, destination inventory.Location) error {
	if err := destination.Validate(); err != nil {
		return err
	}
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := requireLocation(ctx, tx, destination); err != nil {
			return err
		}
		roomID, containerID := locationColumns(destination)
		var moved int64
		err := tx.QueryRowContext(ctx, moveItemSQL, roomID, containerID, id).Scan(&moved)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("item", int64(id))
		}
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("the destination already holds an item with this name: %w", inventory.ErrConflict)
			}
			return fmt.Errorf("moving item %d: %w", id, err)
		}
		return nil
	})
}

// Delete implements inventory.ItemRepo.
func (r *itemRepo) Delete(ctx context.Context, id inventory.ItemID) error {
	var deleted inventory.ItemID
	err := r.db.QueryRowContext(ctx, deleteItemSQL, id).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("item", int64(id))
	}
	if err != nil {
		return fmt.Errorf("deleting item %d: %w", id, err)
	}
	return nil
}

// requireLocation fails with ErrNotFound when the referenced room or
// container does not exist.
func requireLocation(ctx context.Context, q querier, location inventory.Location) error {
	switch location.Kind {
	case inventory.LocationRoom:
		return requireRoom(ctx, q, inventory.RoomID(location.ID))
	case inventory.LocationContainer:
		return requireContainer(ctx, q, inventory.ContainerID(location.ID))
	default:
		return inventory.NewValidationError("location.kind", "must be a room or a container location")
	}
}

// locationColumns maps a location onto the room_id and container_id columns.
func locationColumns(location inventory.Location) (roomID, containerID any) {
	if location.IsRoom() {
		return location.ID, nil
	}
	return nil, location.ID
}

// scanItem reads an item row into item, rebuilding the location from
// whichever of room_id and container_id is set.
func scanItem(row rowScanner, item *inventory.Item) error {
	var room, container sql.NullInt64
	var created, updated string
	if err := row.Scan(&item.ID, &room, &container, &item.Name, &item.Description, &item.Quantity, &item.Notes, &created, &updated); err != nil {
		return err
	}
	switch {
	case room.Valid:
		item.Location = inventory.RoomLocation(inventory.RoomID(room.Int64))
	case container.Valid:
		item.Location = inventory.ContainerLocation(inventory.ContainerID(container.Int64))
	default:
		return fmt.Errorf("item %d has no location", item.ID)
	}
	var err error
	if item.CreatedAt, err = parseTime(created); err != nil {
		return err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return err
	}
	return nil
}

// replaceItemTags rewrites the tags of one item with a delete-and-insert:
// the set is small and fully owned by the item, so replacing it is simpler
// than diffing it.
func replaceItemTags(ctx context.Context, tx *sql.Tx, itemID inventory.ItemID, tags []string) error {
	if _, err := tx.ExecContext(ctx, deleteItemTagsSQL, itemID); err != nil {
		return fmt.Errorf("clearing tags of item %d: %w", itemID, err)
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, insertItemTagSQL, itemID, tag); err != nil {
			return fmt.Errorf("storing tag %q of item %d: %w", tag, itemID, err)
		}
	}
	return nil
}

// loadItemTags reads the tags of one item, ordered case-insensitively.
func loadItemTags(ctx context.Context, q querier, item *inventory.Item) error {
	rows, err := q.QueryContext(ctx, getItemTagsSQL, item.ID)
	if err != nil {
		return fmt.Errorf("loading tags of item %d: %w", item.ID, err)
	}
	defer rows.Close()

	tags := make([]string, 0, 4)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return err
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("loading tags of item %d: %w", item.ID, err)
	}
	item.Tags = tags
	return nil
}

// attachItemTags loads the tags of a whole listing with one query and folds
// them into the items, so listings do not pay one query per item.
func attachItemTags(ctx context.Context, q querier, query string, args []any, items []inventory.Item) error {
	for index := range items {
		if items[index].Tags == nil {
			items[index].Tags = []string{}
		}
	}
	if len(items) == 0 {
		return nil
	}

	byID := make(map[inventory.ItemID]int, len(items))
	for index, item := range items {
		byID[item.ID] = index
	}

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("listing item tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID inventory.ItemID
		var tag string
		if err := rows.Scan(&itemID, &tag); err != nil {
			return err
		}
		if index, ok := byID[itemID]; ok {
			items[index].Tags = append(items[index].Tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("listing item tags: %w", err)
	}
	return nil
}

// queryItems runs one of the constant item listings, scans the result and
// attaches the tags of every item.
func queryItems(ctx context.Context, q querier, query, tagsQuery string, args ...any) ([]inventory.Item, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing items: %w", err)
	}
	defer rows.Close()

	items := make([]inventory.Item, 0)
	for rows.Next() {
		var item inventory.Item
		if err := scanItem(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing items: %w", err)
	}
	if err := attachItemTags(ctx, q, tagsQuery, args, items); err != nil {
		return nil, err
	}
	return items, nil
}

// likePattern turns a search query into a LIKE pattern that matches it as a
// substring. The query is data, not a pattern: its wildcards are escaped, so
// searching for "50%" finds a literal "50%" instead of everything.
func likePattern(query string) string {
	escaper := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + escaper.Replace(query) + "%"
}
