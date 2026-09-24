package tools

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/nicolasalberti00/homey/internal/inventory"
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

// pendingConfirmation is what a token stands for until it is spent.
type pendingConfirmation struct {
	tool    string
	itemID  inventory.ItemID
	expires time.Time
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

// issue mints a token authorising tool on itemID, and reports when it expires.
func (c *Confirmer) issue(tool string, itemID inventory.ItemID) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := c.now().Add(c.ttl)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropExpiredLocked()
	c.tokens[token] = pendingConfirmation{tool: tool, itemID: itemID, expires: expires}
	return token, expires, nil
}

// redeem spends a token. It answers true only when the token exists, has not
// expired, was issued for this exact tool and item, and has not been spent
// before. A token for another action is left alone, so a mistake does not
// cancel a confirmation that is still pending.
func (c *Confirmer) redeem(tool, token string, itemID inventory.ItemID) bool {
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
	if pending.tool != tool || pending.itemID != itemID {
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
// and policies without knowing anything about tokens or HTTP.
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

// AuditEvent records one destructive action that happened, in the shape an
// audit trail needs: which tool ran, on what, for whom, and how the
// confirmation step was resolved.
type AuditEvent struct {
	Tool         string
	ItemID       inventory.ItemID
	Caller       string
	Confirmation string
}

// AuditFunc receives an audit event. A nil AuditFunc records nothing; the
// server wires one, and Phase 7 routes the same events into the audit table.
type AuditFunc func(ctx context.Context, event AuditEvent)
