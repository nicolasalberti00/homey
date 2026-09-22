package main

// Token management commands: create, list and revoke API tokens. Tokens are
// the only credentials for the /api/v1 API; the plaintext is shown once.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/nicolasalberti00/homey/internal/auth"
	"github.com/nicolasalberti00/homey/internal/config"
	"github.com/nicolasalberti00/homey/internal/storage"
)

func runToken(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: homey token <create|list|revoke <id>> [flags]")
	}
	switch args[0] {
	case "create":
		return runTokenCreate(args[1:])
	case "list":
		return runTokenList(args[1:])
	case "revoke":
		return runTokenRevoke(args[1:])
	default:
		return fmt.Errorf("unknown token command %q (expected create, list or revoke)", args[0])
	}
}

// defaultDBPath computes the standard database path from the configuration,
// so token commands address the same database as the server unless --db says
// otherwise.
func defaultDBPath() string {
	cfg, err := config.Load(nil, os.Getenv, io.Discard)
	if err != nil {
		return ""
	}
	return cfg.DBPath
}

func tokenDBPath(fs *flag.FlagSet) *string {
	return fs.String("db", defaultDBPath(), "path to the SQLite database")
}

func openTokenStore(dbPath string) (auth.Store, func(), error) {
	if err := storage.MigrateUp(dbPath); err != nil {
		return nil, nil, fmt.Errorf("migrating database: %w", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { db.Close() }
	return storage.NewTokenStore(db), cleanup, nil
}

func runTokenCreate(args []string) error {
	fs := flag.NewFlagSet("homey token create", flag.ContinueOnError)
	name := fs.String("name", "", "human-readable name of the token (required)")
	scopes := fs.String("scope", "read", `comma-separated scopes: "read", "write" (write implies read)`)
	policy := fs.String("destructive-confirmation", string(auth.ConfirmationRequired), "per-token policy for destructive operations: required (default) or bypass")
	dbPath := tokenDBPath(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	parsedScopes, err := auth.ParseScopes(*scopes)
	if err != nil {
		return err
	}
	confirm := auth.Confirmation(*policy)
	if err := auth.Validate(*name, parsedScopes, confirm); err != nil {
		return err
	}

	plaintext, err := auth.NewToken()
	if err != nil {
		return err
	}

	store, cleanup, err := openTokenStore(*dbPath)
	if err != nil {
		return err
	}
	defer cleanup()

	token, err := store.Create(context.Background(),
		*name, parsedScopes, confirm, auth.Hash(plaintext))
	if err != nil {
		return err
	}

	fmt.Printf("token created (id %d, name %s, scopes %s, destructive_confirmation %s)\n\n%s\n\nThe token is shown only once: store it now.\n",
		token.ID, token.Name, token.Scopes, token.DestructiveConfirmation, plaintext)
	return nil
}

func runTokenList(args []string) error {
	fs := flag.NewFlagSet("homey token list", flag.ContinueOnError)
	dbPath := tokenDBPath(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, cleanup, err := openTokenStore(*dbPath)
	if err != nil {
		return err
	}
	defer cleanup()

	tokens, err := store.List(context.Background())
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		fmt.Println("no tokens")
		return nil
	}
	fmt.Printf("%-4s  %-20s  %-12s  %-10s  %-16s  %s\n", "id", "name", "scopes", "policy", "created", "last used")
	for _, token := range tokens {
		lastUsed := "never"
		if token.LastUsedAt != nil {
			lastUsed = token.LastUsedAt.Format("2006-01-02 15:04")
		}
		fmt.Printf("%-4d  %-20s  %-12s  %-10s  %-16s  %s\n",
			token.ID, token.Name, token.Scopes, token.DestructiveConfirmation,
			token.CreatedAt.Format("2006-01-02 15:04"), lastUsed)
	}
	return nil
}

func runTokenRevoke(args []string) error {
	fs := flag.NewFlagSet("homey token revoke", flag.ContinueOnError)
	dbPath := tokenDBPath(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New("usage: homey token revoke <id>")
	}
	id, err := strconv.ParseInt(rest[0], 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid token id %q", rest[0])
	}

	store, cleanup, err := openTokenStore(*dbPath)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := store.Revoke(context.Background(), id); err != nil {
		return err
	}
	fmt.Printf("token %d revoked\n", id)
	return nil
}
