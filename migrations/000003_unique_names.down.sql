DROP INDEX idx_items_container_name_unique;
DROP INDEX idx_items_room_name_unique;
DROP INDEX idx_containers_room_name_unique;
DROP INDEX idx_rooms_name_unique;

CREATE INDEX idx_rooms_name ON rooms (name);
