package storage

// The SQLite implementation of auth.Store. Only token hashes are stored;
// the plaintext lives with the client and is shown once at creation.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/nicolasalberti00/homey/internal/auth"
)

const (
	insertTokenSQL = `
INSERT INTO api_tokens (name, token_hash, scopes, destructive_confirmation)
VALUES (?, ?, ?, ?)
RETURNING id, name, scopes, destructive_confirmation, created_at, last_used_at`

	listTokensSQL = `
SELECT id, name, scopes, destructive_confirmation, created_at, last_used_at
FROM api_tokens
ORDER BY id`

	revokeTokenSQL = `DELETE FROM api_tokens WHERE id = ? RETURNING id`

	// authenticateTokenSQL resolves a hash and refreshes the last-used
	// timestamp in one statement.
	authenticateTokenSQL = `
UPDATE api_tokens
SET last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE token_hash = ?
RETURNING id, name, scopes, destructive_confirmation, created_at, last_used_at`
)

// tokenStore is the auth.Store adapter backed by api_tokens.
type tokenStore struct {
	db *sql.DB
}

// NewTokenStore builds the token store on db.
func NewTokenStore(db *sql.DB) auth.Store {
	return &tokenStore{db: db}
}

// Create implements auth.Store.
func (s *tokenStore) Create(ctx context.Context, name string, scopes auth.Scopes, policy auth.Confirmation, tokenHash string) (auth.Token, error) {
	if err := auth.Validate(name, scopes, policy); err != nil {
		return auth.Token{}, err
	}
	row := s.db.QueryRowContext(ctx, insertTokenSQL,
		strings.TrimSpace(name), tokenHash, scopes.String(), string(policy))
	return scanToken(row)
}

// List implements auth.Store.
func (s *tokenStore) List(ctx context.Context) ([]auth.Token, error) {
	rows, err := s.db.QueryContext(ctx, listTokensSQL)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	defer rows.Close()

	tokens := make([]auth.Token, 0)
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	return tokens, nil
}

// Revoke implements auth.Store.
func (s *tokenStore) Revoke(ctx context.Context, id int64) error {
	var revoked int64
	err := s.db.QueryRowContext(ctx, revokeTokenSQL, id).Scan(&revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("token %d: %w", id, auth.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("revoking token %d: %w", id, err)
	}
	return nil
}

// Authenticate implements auth.Store.
func (s *tokenStore) Authenticate(ctx context.Context, tokenHash string) (auth.Token, bool) {
	token, err := scanToken(s.db.QueryRowContext(ctx, authenticateTokenSQL, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Token{}, false
	}
	if err != nil {
		// A failing lookup is reported as "not authenticated": the middleware
		// answers 401 either way and the cause is a storage problem, not the
		// caller's.
		return auth.Token{}, false
	}
	return token, true
}

// scanToken reads one api_tokens row (with a nullable last_used_at).
func scanToken(row rowScanner) (auth.Token, error) {
	var (
		token             auth.Token
		scopes            string
		confirmation      string
		created, lastUsed sql.NullString
	)
	if err := row.Scan(&token.ID, &token.Name, &scopes, &confirmation, &created, &lastUsed); err != nil {
		return auth.Token{}, err
	}
	parsed, err := auth.ParseScopes(scopes)
	if err != nil {
		return auth.Token{}, fmt.Errorf("token %d: %w", token.ID, err)
	}
	token.Scopes = parsed
	token.DestructiveConfirmation = auth.Confirmation(confirmation)
	if token.CreatedAt, err = parseTime(created.String); err != nil {
		return auth.Token{}, err
	}
	if lastUsed.Valid {
		value, err := parseTime(lastUsed.String)
		if err != nil {
			return auth.Token{}, fmt.Errorf("token %d: %w", token.ID, err)
		}
		token.LastUsedAt = &value
	}
	return token, nil
}
