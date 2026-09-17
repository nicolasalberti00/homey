package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nicolasalberti00/homey/internal/inventory"
)

// containerRepo is the SQLite implementation of inventory.ContainerRepo.
type containerRepo struct {
	db *sql.DB
}

const (
	insertContainerSQL = `
INSERT INTO containers (room_id, parent_id, name, description)
VALUES (?, ?, ?, ?)
RETURNING id, room_id, parent_id, name, description, created_at, updated_at`

	getContainerSQL = `
SELECT id, room_id, parent_id, name, description, created_at, updated_at
FROM containers
WHERE id = ?`

	listContainersByRoomSQL = `
SELECT id, room_id, parent_id, name, description, created_at, updated_at
FROM containers
WHERE room_id = ?
ORDER BY name COLLATE NOCASE, id`

	updateContainerSQL = `
UPDATE containers
SET parent_id = ?,
    name = ?,
    description = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?
RETURNING id, room_id, parent_id, name, description, created_at, updated_at`

	deleteContainerSQL = `DELETE FROM containers WHERE id = ?`

	containerExistsSQL = `SELECT 1 FROM containers WHERE id = ?`

	containerRoomSQL = `SELECT room_id FROM containers WHERE id = ?`

	containerContentsSQL = `
SELECT EXISTS (SELECT 1 FROM containers WHERE parent_id = ?),
       EXISTS (SELECT 1 FROM items WHERE container_id = ?)`
)

// Create implements inventory.ContainerRepo.
func (r *containerRepo) Create(ctx context.Context, container *inventory.Container) error {
	container.Normalize()
	if err := container.Validate(); err != nil {
		return err
	}
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := requireRoom(ctx, tx, container.RoomID); err != nil {
			return err
		}
		if container.ParentID != nil {
			if err := requireParentInRoom(ctx, tx, *container.ParentID, container.RoomID); err != nil {
				return err
			}
		}
		row := tx.QueryRowContext(ctx, insertContainerSQL,
			container.RoomID, nullableContainerID(container.ParentID), container.Name, container.Description)
		if err := scanContainer(row, container); err != nil {
			return fmt.Errorf("creating container: %w", err)
		}
		return nil
	})
}

// Get implements inventory.ContainerRepo.
func (r *containerRepo) Get(ctx context.Context, id inventory.ContainerID) (inventory.Container, error) {
	var container inventory.Container
	err := scanContainer(r.db.QueryRowContext(ctx, getContainerSQL, id), &container)
	if errors.Is(err, sql.ErrNoRows) {
		return inventory.Container{}, notFound("container", int64(id))
	}
	if err != nil {
		return inventory.Container{}, fmt.Errorf("getting container %d: %w", id, err)
	}
	return container, nil
}

// ListByRoom implements inventory.ContainerRepo.
func (r *containerRepo) ListByRoom(ctx context.Context, roomID inventory.RoomID) ([]inventory.Container, error) {
	if err := requireRoom(ctx, r.db, roomID); err != nil {
		return nil, err
	}
	return queryContainers(ctx, r.db, listContainersByRoomSQL, roomID)
}

// Update implements inventory.ContainerRepo.
func (r *containerRepo) Update(ctx context.Context, container *inventory.Container) error {
	container.Normalize()
	if err := container.Validate(); err != nil {
		return err
	}
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		var storedRoom inventory.RoomID
		err := tx.QueryRowContext(ctx, containerRoomSQL, container.ID).Scan(&storedRoom)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("container", int64(container.ID))
		}
		if err != nil {
			return fmt.Errorf("loading container %d: %w", container.ID, err)
		}
		if storedRoom != container.RoomID {
			return inventory.NewValidationError("room_id", "cannot change the room of a container; move the container instead")
		}
		if container.ParentID != nil {
			if err := requireParentInRoom(ctx, tx, *container.ParentID, container.RoomID); err != nil {
				return err
			}
		}
		row := tx.QueryRowContext(ctx, updateContainerSQL,
			nullableContainerID(container.ParentID), container.Name, container.Description, container.ID)
		if err := scanContainer(row, container); err != nil {
			return fmt.Errorf("updating container %d: %w", container.ID, err)
		}
		return nil
	})
}

// Delete implements inventory.ContainerRepo.
func (r *containerRepo) Delete(ctx context.Context, id inventory.ContainerID) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := requireContainer(ctx, tx, id); err != nil {
			return err
		}

		var containers, items int
		if err := tx.QueryRowContext(ctx, containerContentsSQL, id, id).Scan(&containers, &items); err != nil {
			return fmt.Errorf("checking contents of container %d: %w", id, err)
		}
		if containers > 0 || items > 0 {
			return fmt.Errorf("%w: container %d still holds %d container(s) and %d item(s)",
				inventory.ErrConflict, id, containers, items)
		}

		if _, err := tx.ExecContext(ctx, deleteContainerSQL, id); err != nil {
			return fmt.Errorf("deleting container %d: %w", id, err)
		}
		return nil
	})
}

// requireContainer fails with ErrNotFound when the container does not exist.
func requireContainer(ctx context.Context, q querier, id inventory.ContainerID) error {
	var one int
	err := q.QueryRowContext(ctx, containerExistsSQL, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("container", int64(id))
	}
	if err != nil {
		return fmt.Errorf("loading container %d: %w", id, err)
	}
	return nil
}

// requireParentInRoom fails with ErrNotFound for a missing parent and with a
// validation error for a parent that lives in another room.
func requireParentInRoom(ctx context.Context, q querier, parentID inventory.ContainerID, roomID inventory.RoomID) error {
	var storedRoom inventory.RoomID
	err := q.QueryRowContext(ctx, containerRoomSQL, parentID).Scan(&storedRoom)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("parent container", int64(parentID))
	}
	if err != nil {
		return fmt.Errorf("loading container %d: %w", parentID, err)
	}
	if storedRoom != roomID {
		return inventory.NewValidationError("parent_id", "must reference a container in the same room")
	}
	return nil
}

// nullableContainerID converts an optional parent ID into a query parameter.
func nullableContainerID(id *inventory.ContainerID) any {
	if id == nil {
		return nil
	}
	return int64(*id)
}

// scanContainer reads a container row into container.
func scanContainer(row rowScanner, container *inventory.Container) error {
	var parent sql.NullInt64
	var created, updated string
	if err := row.Scan(&container.ID, &container.RoomID, &parent, &container.Name, &container.Description, &created, &updated); err != nil {
		return err
	}
	container.ParentID = containerIDFrom(parent)
	var err error
	if container.CreatedAt, err = parseTime(created); err != nil {
		return err
	}
	if container.UpdatedAt, err = parseTime(updated); err != nil {
		return err
	}
	return nil
}

// containerIDFrom converts a nullable parent_id column into an optional ID.
func containerIDFrom(parent sql.NullInt64) *inventory.ContainerID {
	if !parent.Valid {
		return nil
	}
	id := inventory.ContainerID(parent.Int64)
	return &id
}

// queryContainers runs one of the constant container listings and scans the
// result.
func queryContainers(ctx context.Context, q querier, query string, args ...any) ([]inventory.Container, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	defer rows.Close()

	containers := make([]inventory.Container, 0)
	for rows.Next() {
		var container inventory.Container
		if err := scanContainer(rows, &container); err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	return containers, nil
}
