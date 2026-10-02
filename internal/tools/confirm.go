package tools

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// ErrConfirmation reports a destructive step that was not authorised: a token
// that is missing, expired, already spent, or meant for another action.
var ErrConfirmation = errors.New("confirmation required")

// DefaultConfirmTTL is how long a confirmation token stays valid when a caller
// does not ask for something else. Short on purpose: a proposal is meant to be
// confirmed in the same conversation, and an old one should not linger.
const DefaultConfirmTTL = 2 * time.Minute

// Confirmer issues and redeems the short-lived tokens that authorise one
// destructive action each. A token is bound to the tool and the entity it was
// issued for, and can be spent once, so a confirmation cannot be replayed or
// bent onto something else.
//
// Build one with NewConfirmer; the zero value is not usable.
//
// Tokens live in memory: a restart drops pending confirmations, which is the
// safe direction — nothing destructive is left half-authorised.
type Confirmer struct {
	mu     sync.Mutex
	ttl    time.Duration
	now    func() time.Time
	tokens map[string]pendingConfirmation
}

// pendingConfirmation is what a token stands for until it is spent. The entity
// is the id of whatever the tool deletes — an item, a room, a container — and
// the tool it was issued for tells the namespaces apart.
type pendingConfirmation struct {
	tool     string
	entityID int64
	expires  time.Time
}

// NewConfirmer returns a confirmer whose tokens live ttl. A non-positive ttl
// falls back to DefaultConfirmTTL.
func NewConfirmer(ttl time.Duration) *Confirmer {
	if ttl <= 0 {
		ttl = DefaultConfirmTTL
	}
	return &Confirmer{
		ttl:    ttl,
		now:    time.Now,
		tokens: make(map[string]pendingConfirmation),
	}
}

// defaultConfirmer keeps a zero-valued Inventory usable: a tool that was not
// handed a confirmer still issues tokens, through this process-wide one.
var defaultConfirmer = NewConfirmer(DefaultConfirmTTL)

// issue mints a token authorising tool on entity, and reports when it expires.
func (c *Confirmer) issue(tool string, entity int64) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := c.now().Add(c.ttl)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropExpiredLocked()
	c.tokens[token] = pendingConfirmation{tool: tool, entityID: entity, expires: expires}
	return token, expires, nil
}

// redeem spends a token. It answers true only when the token exists, has not
// expired, was issued for this exact tool and entity, and has not been spent
// before. A token for another action is left alone, so a mistake does not
// cancel a confirmation that is still pending.
func (c *Confirmer) redeem(tool, token string, entity int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	pending, found := c.tokens[token]
	if !found {
		return false
	}
	if !c.now().Before(pending.expires) {
		delete(c.tokens, token)
		return false
	}
	if pending.tool != tool || pending.entityID != entity {
		return false
	}
	delete(c.tokens, token)
	return true
}

// dropExpiredLocked clears tokens nobody can spend any more. It runs on issue,
// under the lock, so the map does not grow without bound.
func (c *Confirmer) dropExpiredLocked() {
	now := c.now()
	for token, pending := range c.tokens {
		if !now.Before(pending.expires) {
			delete(c.tokens, token)
		}
	}
}

// Caller is who a tool is running for. The transport that authenticated the
// request puts it in the context, so a tool can honour the caller's permissions
// and policies without knowing anything about tokens or HTTP. Its Name is what
// the event log records as the actor of every mutation the tool carries out.
type Caller struct {
	// Name identifies the caller in the audit trail.
	Name string
	// CanWrite marks a caller allowed to change the inventory. A read-only
	// caller can explore but not add, change, move or delete.
	CanWrite bool
	// BypassConfirmation marks a caller trusted to run destructive tools
	// without the confirmation step. It comes from the caller's own policy.
	BypassConfirmation bool
}

// callerKey is the context key the caller travels under.
type callerKey struct{}

// WithCaller returns a context carrying who is calling.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, caller)
}

// CallerFrom returns the caller stored in ctx, if the transport put one there.
func CallerFrom(ctx context.Context) (Caller, bool) {
	caller, found := ctx.Value(callerKey{}).(Caller)
	return caller, found
}

// AuditFunc has moved out: the audit trail lives in the events table the
// storage layer writes, and the tool only says who is calling.

// AuditEvent is gone with it. See internal/events for the shape of a recorded
// action, and the caller for who a tool runs for.
