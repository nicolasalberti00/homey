-- Rooms are the top level of the inventory: Kitchen, Garage, Office, …
CREATE TABLE rooms (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_rooms_name ON rooms (name);

-- Containers are nested inside rooms. parent_id points at another container
-- in the same room; NULL means the container sits directly in the room.
CREATE TABLE containers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    room_id     INTEGER NOT NULL REFERENCES rooms (id),
    parent_id   INTEGER REFERENCES containers (id),
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_containers_room ON containers (room_id);
CREATE INDEX idx_containers_parent ON containers (parent_id);

-- An item lives in exactly one location: a room or a container.
CREATE TABLE items (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    room_id      INTEGER REFERENCES rooms (id),
    container_id INTEGER REFERENCES containers (id),
    name         TEXT    NOT NULL,
    description  TEXT    NOT NULL DEFAULT '',
    quantity     INTEGER NOT NULL DEFAULT 1 CHECK (quantity >= 0),
    notes        TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK ((room_id IS NULL) <> (container_id IS NULL))
);

CREATE INDEX idx_items_room ON items (room_id);
CREATE INDEX idx_items_container ON items (container_id);
CREATE INDEX idx_items_name ON items (name);
