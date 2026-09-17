-- Names are unique within their scope, case-insensitively (SQLite NOCASE
-- folds ASCII only, which is fine for MVP):
--   * a room name is unique across the inventory;
--   * a container name is unique within its room, at any nesting depth;
--   * an item name is unique within its location: a room or a container.

DROP INDEX idx_rooms_name;

CREATE UNIQUE INDEX idx_rooms_name_unique ON rooms (name COLLATE NOCASE);

CREATE UNIQUE INDEX idx_containers_room_name_unique ON containers (room_id, name COLLATE NOCASE);

CREATE UNIQUE INDEX idx_items_room_name_unique ON items (room_id, name COLLATE NOCASE) WHERE room_id IS NOT NULL;

CREATE UNIQUE INDEX idx_items_container_name_unique ON items (container_id, name COLLATE NOCASE) WHERE container_id IS NOT NULL;
