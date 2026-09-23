-- Aliases are alternative names for an item: "giravite" for a "cacciavite".
-- They live in their own table like tags, so search can match them through an
-- index instead of scanning a JSON column.
--
-- Aliases are case-insensitive within their item: the column collation
-- applies to the primary key, so "Giravite" and "giravite" cannot coexist on
-- the same item. The same alias may repeat on different items. ON DELETE
-- CASCADE keeps alias rows with their item, since they have no meaning on
-- their own.
CREATE TABLE item_aliases (
    item_id INTEGER NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    alias   TEXT    NOT NULL COLLATE NOCASE,
    PRIMARY KEY (item_id, alias)
);

CREATE INDEX idx_item_aliases_alias ON item_aliases (alias);
