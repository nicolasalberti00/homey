-- API tokens authenticate API and MCP clients. Only the SHA-256 hash of a
-- token is stored. scopes is a comma-separated list (read, write);
-- destructive_confirmation encodes the per-client policy for destructive
-- tools: "required" (two-step confirmation) or "bypass" (trusted clients).
CREATE TABLE api_tokens (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    name                     TEXT    NOT NULL,
    token_hash               TEXT    NOT NULL UNIQUE,
    scopes                   TEXT    NOT NULL DEFAULT 'read',
    destructive_confirmation TEXT    NOT NULL DEFAULT 'required'
                             CHECK (destructive_confirmation IN ('required', 'bypass')),
    created_at               TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at             TEXT
);
