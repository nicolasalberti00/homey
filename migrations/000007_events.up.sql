-- Events are the audit trail of the inventory: one row per mutation, written
-- by the storage adapter in the same transaction as the mutation it describes,
-- so the history is complete by construction and cannot drift from the data.
--
-- actor is the token name behind the mutation (empty when the system acted on
-- its own), tool the MCP tool that carried it out when one did, and
-- confirmation how a destructive step was authorised: confirmed or bypassed.
-- The three together are the audit of the LLM actions.
--
-- payload is the JSON a future undo needs: the state of the entity as it now
-- stands, the state it was in when it went away, or the places of a move.
CREATE TABLE events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    type         TEXT    NOT NULL CHECK (length(type) <= 40),
    entity_kind  TEXT    NOT NULL CHECK (entity_kind IN ('room', 'container', 'item')),
    entity_id    INTEGER NOT NULL CHECK (entity_id > 0),
    actor        TEXT    NOT NULL DEFAULT '',
    tool         TEXT    NOT NULL DEFAULT '',
    confirmation TEXT    NOT NULL DEFAULT '',
    payload      TEXT    NOT NULL DEFAULT '{}',
    at           TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_events_entity ON events (entity_kind, entity_id, id);
CREATE INDEX idx_events_type ON events (type);
