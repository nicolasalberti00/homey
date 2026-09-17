package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// roomRepo is the SQLite implementation of inventory.RoomRepo.
type roomRepo struct {
	db *sql.DB
}

const (
	insertRoomSQL = `
INSERT INTO rooms (name, description)
VALUES (?, ?)
RETURNING id, name, description, created_at, updated_at`

	getRoomSQL = `
SELECT id, name, description, created_at, updated_at
FROM rooms
WHERE id = ?`

	listRoomsSQL = `
SELECT id, name, description, created_at, updated_at
FROM rooms
ORDER BY name COLLATE NOCASE, id`

	updateRoomSQL = `
UPDATE rooms
SET name = ?,
    description = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING id, name, description, created_at, updated_at`

	deleteRoomSQL = `DELETE FROM rooms WHERE id = ?`

	roomExistsSQL = `SELECT 1 FROM rooms WHERE id = ?`

	roomContentsSQL = `
SELECT EXISTS (SELECT 1 FROM containers WHERE room_id = ?),
       EXISTS (SELECT 1 FROM items WHERE room_id = ?)`
)

// Create implements inventory.RoomRepo.
func (r *roomRepo) Create(ctx context.Context, room *inventory.Room) error {
	room.Normalize()
	if err := room.Validate(); err != nil {
		return err
	}
	if err := scanRoom(r.db.QueryRowContext(ctx, insertRoomSQL, room.Name, room.Description), room); err != nil {
		return fmt.Errorf("creating room: %w", err)
	}
	return nil
}

// Get implements inventory.RoomRepo.
func (r *roomRepo) Get(ctx context.Context, id inventory.RoomID) (inventory.Room, error) {
	var room inventory.Room
	err := scanRoom(r.db.QueryRowContext(ctx, getRoomSQL, id), &room)
	if errors.Is(err, sql.ErrNoRows) {
		return inventory.Room{}, notFound("room", int64(id))
	}
	if err != nil {
		return inventory.Room{}, fmt.Errorf("getting room %d: %w", id, err)
	}
	return room, nil
}

// List implements inventory.RoomRepo.
func (r *roomRepo) List(ctx context.Context) ([]inventory.Room, error) {
	return queryRooms(ctx, r.db, listRoomsSQL)
}

// Update implements inventory.RoomRepo.
func (r *roomRepo) Update(ctx context.Context, room *inventory.Room) error {
	room.Normalize()
	if err := room.Validate(); err != nil {
		return err
	}
	err := scanRoom(r.db.QueryRowContext(ctx, updateRoomSQL, room.Name, room.Description, room.ID), room)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("room", int64(room.ID))
	}
	if err != nil {
		return fmt.Errorf("updating room %d: %w", room.ID, err)
	}
	return nil
}

// Delete implements inventory.RoomRepo.
func (r *roomRepo) Delete(ctx context.Context, id inventory.RoomID) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := requireRoom(ctx, tx, id); err != nil {
			return err
		}

		var containers, items int
		if err := tx.QueryRowContext(ctx, roomContentsSQL, id, id).Scan(&containers, &items); err != nil {
			return fmt.Errorf("checking contents of room %d: %w", id, err)
		}
		if containers > 0 || items > 0 {
			return fmt.Errorf("%w: room %d still holds %d container(s) and %d item(s)",
				inventory.ErrConflict, id, containers, items)
		}

		if _, err := tx.ExecContext(ctx, deleteRoomSQL, id); err != nil {
			return fmt.Errorf("deleting room %d: %w", id, err)
		}
		return nil
	})
}

// scanRoom reads a room row into room.
func scanRoom(row rowScanner, room *inventory.Room) error {
	var created, updated string
	if err := row.Scan(&room.ID, &room.Name, &room.Description, &created, &updated); err != nil {
		return err
	}
	var err error
	if room.CreatedAt, err = parseTime(created); err != nil {
		return err
	}
	if room.UpdatedAt, err = parseTime(updated); err != nil {
		return err
	}
	return nil
}

// queryRooms runs one of the constant room listings and scans the result.
func queryRooms(ctx context.Context, q querier, query string, args ...any) ([]inventory.Room, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing rooms: %w", err)
	}
	defer rows.Close()

	rooms := make([]inventory.Room, 0)
	for rows.Next() {
		var room inventory.Room
		if err := scanRoom(rows, &room); err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing rooms: %w", err)
	}
	return rooms, nil
}
