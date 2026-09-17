-- Tags live in their own table: they are part of the item aggregate but a
-- relation rather than a column, so tag searches (Phase 5) can use the index
-- and joins instead of scanning JSON.
--
-- Tags are case-insensitive: the column collation applies to the primary key,
-- so "Bagno" and "bagno" cannot coexist on the same item. Tags that stay on
-- different items may repeat freely. ON DELETE CASCADE keeps tag rows with
-- their item, since they have no meaning on their own.
CREATE TABLE item_tags (
    item_id INTEGER NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    tag     TEXT    NOT NULL COLLATE NOCASE,
    PRIMARY KEY (item_id, tag)
);

CREATE INDEX idx_item_tags_tag ON item_tags (tag);
