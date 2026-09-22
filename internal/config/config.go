// Package config loads homey configuration from environment variables and
// command-line flags.
//
// Precedence, from lowest to highest:
//
//  1. built-in defaults
//  2. environment variables (HOMEY_*)
//  3. command-line flags
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

// Default values applied when neither environment nor flags provide one.
const (
	DefaultListen    = ":8080"
	DefaultDataDir   = "./data"
	DefaultLogLevel  = "info"
	DefaultLogFormat = "text"
	// DefaultRateLimitWrites allows this many mutating /api/v1 requests per
	// client IP per minute.
	DefaultRateLimitWrites = 60
	// DefaultRateLimitAuthFailures allows this many failed authentication
	// attempts per client IP per minute.
	DefaultRateLimitAuthFailures = 10
)

// Config holds every knob homey needs at startup.
type Config struct {
	// Listen is the TCP address the HTTP server binds to.
	Listen string
	// DataDir is the directory for persistent state (SQLite database).
	DataDir string
	// DBPath is the SQLite database file.
	DBPath string
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
	// LogFormat is either "text" (human friendly) or "json".
	LogFormat string
	// CORSOrigins lists allowed browser origins; empty means same-origin only.
	CORSOrigins []string
	// MCPEnabled switches the MCP endpoint (Phase 6) on or off.
	MCPEnabled bool
	// RateLimitWrites caps mutating /api/v1 requests per client IP per
	// minute; 0 disables the limit.
	RateLimitWrites int
	// RateLimitAuthFailures caps failed authentication attempts per client
	// IP per minute; 0 disables the limit.
	RateLimitAuthFailures int
}

var (
	validLogLevels  = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	validLogFormats = map[string]bool{"text": true, "json": true}
)

// Load builds a Config from args (typically os.Args[1:]) and getenv. getenv
// may be nil, in which case only defaults and flags are considered. Usage
// output goes to stderr.
func Load(args []string, getenv func(string) string, stderr io.Writer) (*Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	mcpEnabled := true
	if raw := getenv("HOMEY_MCP_ENABLED"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("HOMEY_MCP_ENABLED: %q is not a boolean", raw)
		}
		mcpEnabled = parsed
	}

	rateWrites, err := intFromEnv(getenv, "HOMEY_RATE_LIMIT_WRITES", DefaultRateLimitWrites)
	if err != nil {
		return nil, err
	}
	rateAuthFailures, err := intFromEnv(getenv, "HOMEY_RATE_LIMIT_AUTH_FAILURES", DefaultRateLimitAuthFailures)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Listen:      valueOrDefault(getenv("HOMEY_LISTEN"), DefaultListen),
		DataDir:     valueOrDefault(getenv("HOMEY_DATA_DIR"), DefaultDataDir),
		DBPath:      getenv("HOMEY_DB_PATH"),
		LogLevel:    valueOrDefault(getenv("HOMEY_LOG_LEVEL"), DefaultLogLevel),
		LogFormat:   valueOrDefault(getenv("HOMEY_LOG_FORMAT"), DefaultLogFormat),
		CORSOrigins: splitList(getenv("HOMEY_CORS_ORIGINS")),
		MCPEnabled:  mcpEnabled,

		RateLimitWrites:       rateWrites,
		RateLimitAuthFailures: rateAuthFailures,
	}

	fs := flag.NewFlagSet("homey", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "homey configuration flags:")
		fs.PrintDefaults()
	}

	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "address the HTTP server listens on")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "directory for persistent data")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "path to the SQLite database (default <data-dir>/homey.db)")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, error")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json")
	var corsFlag string
	fs.StringVar(&corsFlag, "cors-origins", strings.Join(cfg.CORSOrigins, ","), "comma-separated list of allowed CORS origins")
	fs.BoolVar(&cfg.MCPEnabled, "mcp", cfg.MCPEnabled, "enable the MCP endpoint")
	fs.IntVar(&cfg.RateLimitWrites, "rate-limit-writes", cfg.RateLimitWrites, "mutating /api/v1 requests per IP per minute (0 disables)")
	fs.IntVar(&cfg.RateLimitAuthFailures, "rate-limit-auth-failures", cfg.RateLimitAuthFailures, "failed authentication attempts per IP per minute (0 disables)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Only touch CORSOrigins when the flag was passed explicitly, so that an
	// explicit empty value clears the environment variable.
	corsSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "cors-origins" {
			corsSet = true
		}
	})
	if corsSet {
		cfg.CORSOrigins = splitList(corsFlag)
	}

	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "homey.db")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	var problems []string
	if strings.TrimSpace(c.Listen) == "" {
		problems = append(problems, "listen address must not be empty")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		problems = append(problems, "data dir must not be empty")
	}
	if !validLogLevels[c.LogLevel] {
		problems = append(problems, fmt.Sprintf("log level %q is not one of debug, info, warn, error", c.LogLevel))
	}
	if !validLogFormats[c.LogFormat] {
		problems = append(problems, fmt.Sprintf("log format %q is not one of text, json", c.LogFormat))
	}
	if c.RateLimitWrites < 0 {
		problems = append(problems, "rate limit for writes must not be negative")
	}
	if c.RateLimitAuthFailures < 0 {
		problems = append(problems, "rate limit for auth failures must not be negative")
	}
	if len(problems) > 0 {
		return errors.New("invalid configuration: " + strings.Join(problems, "; "))
	}
	return nil
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// intFromEnv reads a non-negative integer setting from the environment,
// falling back to def when unset.
func intFromEnv(getenv func(string) string, name string, def int) (int, error) {
	raw := getenv(name)
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s: %q must be a non-negative integer", name, raw)
	}
	return parsed, nil
}
