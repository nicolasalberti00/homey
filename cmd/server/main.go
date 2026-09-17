// Command server runs homey: a self-hosted, platform-independent home
// inventory with a Web UI and first-class LLM/MCP support.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nicolasalberti00/homey/internal/config"
	"github.com/nicolasalberti00/homey/internal/logging"
	"github.com/nicolasalberti00/homey/internal/server"
	"github.com/nicolasalberti00/homey/internal/storage"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `homey — self-hosted home inventory

usage:
  homey [serve] [flags]        start the HTTP server (default command)
  homey version                print the version
  homey migrate <command>      manage the database schema
                               commands: up, down, version, force <version>

Flags: "homey serve --help" lists the configuration flags.
Environment variables (HOMEY_*) are documented in the README.`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "homey: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "serve":
			args = args[1:]
		case "version":
			fmt.Printf("homey %s\n", version)
			return nil
		case "migrate":
			return runMigrate(args[1:])
		case "help", "-h", "--help":
			fmt.Println(usage)
			return nil
		}
	}
	return runServe(args)
}

func runServe(args []string) error {
	cfg, err := config.Load(args, os.Getenv, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	logger, err := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return err
	}

	if err := storage.MigrateUp(cfg.DBPath); err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := server.New(cfg, logger, db)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("homey listening",
			"addr", cfg.Listen,
			"db", cfg.DBPath,
			"mcp_enabled", cfg.MCPEnabled,
			"version", version,
		)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
		return nil
	}
}

func runMigrate(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: homey migrate <up|down|version|force <version>> [flags]")
	}
	command := args[0]
	rest := args[1:]

	var forceVersion int
	if command == "force" {
		if len(rest) == 0 {
			return errors.New("usage: homey migrate force <version> [flags]")
		}
		v, err := strconv.Atoi(rest[0])
		if err != nil {
			return fmt.Errorf("invalid force version %q: %w", rest[0], err)
		}
		forceVersion = v
		rest = rest[1:]
	}

	cfg, err := config.Load(rest, os.Getenv, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	switch command {
	case "up":
		if err := storage.MigrateUp(cfg.DBPath); err != nil {
			return err
		}
		fmt.Println("migrations applied")
		return nil
	case "down":
		if err := storage.MigrateDown(cfg.DBPath); err != nil {
			return err
		}
		fmt.Println("migrations rolled back")
		return nil
	case "version":
		v, dirty, err := storage.MigrationVersion(cfg.DBPath)
		if err != nil {
			return err
		}
		fmt.Printf("schema version: %d (dirty: %t)\n", v, dirty)
		return nil
	case "force":
		if err := storage.MigrateForce(cfg.DBPath, forceVersion); err != nil {
			return err
		}
		fmt.Printf("schema version forced to %d\n", forceVersion)
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q (expected up, down, version or force)", command)
	}
}
