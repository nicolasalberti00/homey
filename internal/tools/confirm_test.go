package tools

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestConfirmerBindsTokensToTheActionAndBurnsThemOnce checks the two promises a
// confirmation token makes: it authorises one specific action, and it works
// exactly once.
func TestConfirmerBindsTokensToTheActionAndBurnsThemOnce(t *testing.T) {
	confirmer := NewConfirmer(time.Minute)

	token, _, err := confirmer.issue("delete_item", 7)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if token == "" {
		t.Fatal("issue handed back an empty token")
	}

	// A token for one action cannot be spent on another, nor on another item.
	if confirmer.redeem("move_item", token, 7) {
		t.Fatal("a token was accepted for a different tool")
	}
	if confirmer.redeem("delete_item", token, 8) {
		t.Fatal("a token was accepted for a different item")
	}
	// It is still spendable on the action it was issued for...
	if !confirmer.redeem("delete_item", token, 7) {
		t.Fatal("the token was refused for the action it was issued for")
	}
	// ...but only once.
	if confirmer.redeem("delete_item", token, 7) {
		t.Fatal("the token could be spent twice")
	}
}

// TestConfirmerExpiresTokens checks the short life of a confirmation: after the
// time to live passes the token is refused.
func TestConfirmerExpiresTokens(t *testing.T) {
	now := time.Now()
	confirmer := NewConfirmer(time.Minute)
	confirmer.now = func() time.Time { return now }

	token, expires, err := confirmer.issue("delete_item", 7)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if want := now.Add(time.Minute); !expires.Equal(want) {
		t.Fatalf("expires = %v, want %v", expires, want)
	}

	now = now.Add(time.Minute + time.Second)
	if confirmer.redeem("delete_item", token, 7) {
		t.Fatal("an expired token was accepted")
	}
}

// TestConfirmerTokensAreUnique checks that two confirmations of the same action
// do not share a token, so an old one cannot be guessed or replayed.
func TestConfirmerTokensAreUnique(t *testing.T) {
	confirmer := NewConfirmer(time.Minute)

	first, _, err := confirmer.issue("delete_item", 7)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	second, _, err := confirmer.issue("delete_item", 7)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if first == second {
		t.Fatal("two confirmations share a token")
	}
}

// TestConfirmerForgetsExpiredTokens checks the token map is swept as it grows: a
// confirmation nobody can spend any more is dropped, not kept forever.
func TestConfirmerForgetsExpiredTokens(t *testing.T) {
	now := time.Now()
	confirmer := NewConfirmer(time.Minute)
	confirmer.now = func() time.Time { return now }

	if _, _, err := confirmer.issue("delete_item", 1); err != nil {
		t.Fatalf("issue: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, _, err := confirmer.issue("delete_item", 2); err != nil {
		t.Fatalf("issue: %v", err)
	}

	confirmer.mu.Lock()
	defer confirmer.mu.Unlock()
	if len(confirmer.tokens) != 1 {
		t.Fatalf("the confirmer holds %d tokens, want only the live one", len(confirmer.tokens))
	}
}

// TestConfirmerWithoutATTLUsesTheDefault keeps a zero-valued confirmer usable:
// a caller that does not pick a lifetime gets the safe default.
func TestConfirmerWithoutATTLUsesTheDefault(t *testing.T) {
	confirmer := NewConfirmer(0)
	if confirmer.ttl != DefaultConfirmTTL {
		t.Fatalf("ttl = %v, want the default %v", confirmer.ttl, DefaultConfirmTTL)
	}
}

// TestCallerTravelsInTheContext checks the transport can hand an identity down
// to a tool without the tool knowing about authentication.
func TestCallerTravelsInTheContext(t *testing.T) {
	if _, found := CallerFrom(t.Context()); found {
		t.Fatal("a bare context already carried a caller")
	}
	want := Caller{Name: "automation", BypassConfirmation: true}
	ctx := WithCaller(t.Context(), want)
	got, found := CallerFrom(ctx)
	if !found {
		t.Fatal("the caller did not travel in the context")
	}
	if got != want {
		t.Fatalf("caller = %+v, want %+v", got, want)
	}
}

// auditLog is a spy that remembers what the destructive tools recorded, so a
// test can assert on the audit trail without a storage layer.
type auditLog struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (l *auditLog) record(_ context.Context, event AuditEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *auditLog) all() []AuditEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	events := make([]AuditEvent, len(l.events))
	copy(events, l.events)
	return events
}
