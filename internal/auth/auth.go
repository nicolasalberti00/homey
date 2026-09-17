package auth

// Token authentication for the API and future MCP clients. Tokens are
// generated as random secrets and stored only as SHA-256 hashes: the
// plaintext is shown exactly once, at creation time.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Scope grants a capability to a token. write implies read.
type Scope string

const (
	// ScopeRead allows read operations (GET).
	ScopeRead Scope = "read"
	// ScopeWrite grants read plus every mutating operation.
	ScopeWrite Scope = "write"
)

// Scopes is the set of scopes granted to a token, in normalized order.
type Scopes []Scope

// ParseScopes parses a comma-separated scope list such as "read,write".
// Entries are trimmed, lowercased and de-duplicated; unknown scopes and
// empty lists are errors.
func ParseScopes(raw string) (Scopes, error) {
	scopes := make(Scopes, 0, 2)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		switch Scope(entry) {
		case ScopeRead, ScopeWrite:
		default:
			return nil, fmt.Errorf("%w: unknown scope %q", ErrInvalid, entry)
		}
		if !scopes.Contains(Scope(entry)) {
			scopes = append(scopes, Scope(entry))
		}
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrInvalid)
	}
	return scopes, nil
}

// Contains reports whether the set grants scope.
func (s Scopes) Contains(scope Scope) bool {
	for _, entry := range s {
		if entry == scope {
			return true
		}
	}
	return false
}

// CanWrite reports whether the set grants write (and therefore read).
func (s Scopes) CanWrite() bool { return s.Contains(ScopeWrite) }

// String renders the set as a comma-separated list, matching the storage
// format.
func (s Scopes) String() string {
	parts := make([]string, len(s))
	for i, scope := range s {
		parts[i] = string(scope)
	}
	return strings.Join(parts, ",")
}

// Confirmation encodes the per-token policy for destructive operations.
type Confirmation string

const (
	// ConfirmationRequired means destructive operations ask for an explicit
	// two-step confirmation (the default and safest policy).
	ConfirmationRequired Confirmation = "required"
	// ConfirmationBypass marks a trusted token allowed to perform destructive
	// operations directly. It is explicit per token and audited by Phase 6.
	ConfirmationBypass Confirmation = "bypass"
)

// ErrInvalid reports invalid token input (name, scopes or policy).
var ErrInvalid = errors.New("auth: invalid token input")

// ErrNotFound reports that the token does not exist (for example it was
// revoked).
var ErrNotFound = errors.New("auth: token not found")

// Token is an authenticated API identity. It never carries the secret: the
// plaintext lives only in the hands of the client.
type Token struct {
	ID                      int64
	Name                    string
	Scopes                  Scopes
	DestructiveConfirmation Confirmation
	CreatedAt               time.Time
	LastUsedAt              *time.Time
}

// CanWrite reports whether the token may perform mutating requests.
func (t Token) CanWrite() bool { return t.Scopes.CanWrite() }

// NewToken generates a fresh plaintext token. The format is self-identifying
// ("homey_" + 43 base64url characters) so leaked values are easy to
// recognize in logs.
func NewToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// tokenPrefix marks generated tokens.
const tokenPrefix = "homey_"

// hashPrefix versioning the stored hash: the format is "sha256:<hex>", so a
// stronger scheme (for example Argon2id) can be introduced later without
// invalidating existing tokens.
const hashPrefix = "sha256:"

// Hash returns the versioned hash of a token: this is what gets stored.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hashPrefix + hex.EncodeToString(sum[:])
}

// Verify compares a presented plaintext token against a stored hash in
// constant time. Bare hex hashes (without the version prefix) are accepted
// for data written before the versioned format existed.
func Verify(plaintext, stored string) bool {
	stored = strings.TrimPrefix(stored, hashPrefix)
	return subtle.ConstantTimeCompare([]byte(hashHex(plaintext)), []byte(stored)) == 1
}

// hashHex returns the raw hex digest of the token.
func hashHex(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// Validate checks the inputs of token creation.
func Validate(name string, scopes Scopes, policy Confirmation) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalid)
	}
	if len(scopes) == 0 {
		return fmt.Errorf("%w: at least one scope is required", ErrInvalid)
	}
	switch policy {
	case ConfirmationRequired, ConfirmationBypass:
	default:
		return fmt.Errorf("%w: unknown destructive-confirmation policy %q", ErrInvalid, policy)
	}
	return nil
}

// Store is the persistence port for tokens: adapters implement it (the
// SQLite implementation lives in internal/storage).
type Store interface {
	// Create stores a token hash and returns the token metadata. Inputs are
	// validated here, so callers can trust what they get back.
	Create(ctx context.Context, name string, scopes Scopes, policy Confirmation, tokenHash string) (Token, error)
	// List returns every token, oldest first.
	List(ctx context.Context) ([]Token, error)
	// Revoke removes a token. It fails with ErrNotFound for unknown IDs.
	Revoke(ctx context.Context, id int64) error
	// Authenticate resolves a token hash, refreshing its last-used timestamp
	// in the same statement. It returns false for unknown or revoked hashes.
	Authenticate(ctx context.Context, tokenHash string) (Token, bool)
}
