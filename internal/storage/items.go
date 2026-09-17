package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
			return fmt.Errorf("creating item: %w", err)
		}
		return nil
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
	return item, nil
}

// List implements inventory.ItemRepo.
func (r *itemRepo) List(ctx context.Context) ([]inventory.Item, error) {
	return queryItems(ctx, r.db, listItemsSQL)
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
		return queryItems(ctx, r.db, listItemsByRoomSQL, location.ID)
	}
	return queryItems(ctx, r.db, listItemsByContainerSQL, location.ID)
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
		if err != nil {
			return fmt.Errorf("updating item %d: %w", item.ID, err)
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

// queryItems runs one of the constant item listings and scans the result.
func queryItems(ctx context.Context, q querier, query string, args ...any) ([]inventory.Item, error) {
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
	return items, nil
}
