package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/nicolasalberti00/homey/internal/auth"
)

func TestTokenStoreRoundTrip(t *testing.T) {
	store := NewTokenStore(openDB(t, mustMigrate(t)))
	ctx := context.Background()

	token, err := store.Create(ctx, "UI", auth.Scopes{auth.ScopeWrite}, auth.ConfirmationBypass, "hash-one")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token.ID <= 0 || token.Name != "UI" || !token.CanWrite() {
		t.Fatalf("created = %+v", token)
	}
	if token.DestructiveConfirmation != auth.ConfirmationBypass {
		t.Fatalf("policy = %v", token.DestructiveConfirmation)
	}

	// Authenticate resolves the hash and refreshes the last-used timestamp.
	found, ok := store.Authenticate(ctx, "hash-one")
	if !ok {
		t.Fatal("Authenticate returned false for a known hash")
	}
	if found.ID != token.ID {
		t.Fatalf("authenticated token = %+v", found)
	}
	if found.LastUsedAt == nil {
		t.Fatal("last_used_at should be set after authentication")
	}

	// An unknown hash is not authenticated.
	if _, ok := store.Authenticate(ctx, "hash-nothing"); ok {
		t.Fatal("Authenticate accepted an unknown hash")
	}

	// List returns the token.
	tokens, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != token.ID {
		t.Fatalf("list = %+v", tokens)
	}

	// Revoke removes the token; authentication stops working.
	if err := store.Revoke(ctx, token.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok := store.Authenticate(ctx, "hash-one"); ok {
		t.Fatal("Authenticate accepted a revoked token")
	}
	if err := store.Revoke(ctx, token.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("double revoke = %v, want ErrNotFound", err)
	}
}

func TestTokenStoreCreateValidates(t *testing.T) {
	store := NewTokenStore(openDB(t, mustMigrate(t)))
	ctx := context.Background()

	if _, err := store.Create(ctx, "  ", auth.Scopes{auth.ScopeRead}, auth.ConfirmationRequired, "hash"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("empty name = %v, want ErrInvalid", err)
	}
	if _, err := store.Create(ctx, "x", nil, auth.ConfirmationRequired, "hash"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("empty scopes = %v, want ErrInvalid", err)
	}
	if _, err := store.Create(ctx, "x", auth.Scopes{auth.ScopeRead}, "wrong", "hash"); !errors.Is(err, auth.ErrInvalid) {
		t.Fatalf("bad policy = %v, want ErrInvalid", err)
	}
}
