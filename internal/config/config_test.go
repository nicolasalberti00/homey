package config

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func load(t *testing.T, args []string, env map[string]string) (*Config, error) {
	t.Helper()
	return Load(args, envFrom(env), &bytes.Buffer{})
}

func TestDefaults(t *testing.T) {
	cfg, err := load(t, nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != DefaultListen {
		t.Errorf("Listen = %q, want %q", cfg.Listen, DefaultListen)
	}
	if cfg.DataDir != DefaultDataDir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, DefaultDataDir)
	}
	if cfg.DBPath != "data/homey.db" {
		t.Errorf("DBPath = %q, want data/homey.db", cfg.DBPath)
	}
	if cfg.LogLevel != DefaultLogLevel || cfg.LogFormat != DefaultLogFormat {
		t.Errorf("log defaults = %q/%q, want %q/%q", cfg.LogLevel, cfg.LogFormat, DefaultLogLevel, DefaultLogFormat)
	}
	if cfg.MCPEnabled != true {
		t.Error("MCPEnabled should default to true")
	}
	if cfg.CORSOrigins != nil {
		t.Errorf("CORSOrigins = %v, want nil", cfg.CORSOrigins)
	}
}

func TestEnvOverridesDefaults(t *testing.T) {
	cfg, err := load(t, nil, map[string]string{
		"HOMEY_LISTEN":       ":9999",
		"HOMEY_DATA_DIR":     "/srv/homey",
		"HOMEY_LOG_LEVEL":    "warn",
		"HOMEY_LOG_FORMAT":   "json",
		"HOMEY_CORS_ORIGINS": "https://a.example, https://b.example",
		"HOMEY_MCP_ENABLED":  "false",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":9999" {
		t.Errorf("Listen = %q, want :9999", cfg.Listen)
	}
	if cfg.DBPath != "/srv/homey/homey.db" {
		t.Errorf("DBPath = %q, want /srv/homey/homey.db", cfg.DBPath)
	}
	if cfg.LogLevel != "warn" || cfg.LogFormat != "json" {
		t.Errorf("log = %q/%q, want warn/json", cfg.LogLevel, cfg.LogFormat)
	}
	if cfg.MCPEnabled {
		t.Error("MCPEnabled should be false")
	}
	want := []string{"https://a.example", "https://b.example"}
	if !reflect.DeepEqual(cfg.CORSOrigins, want) {
		t.Errorf("CORSOrigins = %v, want %v", cfg.CORSOrigins, want)
	}
}

func TestEnvDBPathOverridesDataDir(t *testing.T) {
	cfg, err := load(t, nil, map[string]string{"HOMEY_DB_PATH": "/tmp/custom.db"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBPath != "/tmp/custom.db" {
		t.Errorf("DBPath = %q, want /tmp/custom.db", cfg.DBPath)
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	cfg, err := load(t,
		[]string{"--listen", ":7000", "--log-level", "debug", "--db", "/tmp/flag.db"},
		map[string]string{"HOMEY_LISTEN": ":9999", "HOMEY_LOG_LEVEL": "warn"},
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":7000" {
		t.Errorf("Listen = %q, want :7000", cfg.Listen)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.DBPath != "/tmp/flag.db" {
		t.Errorf("DBPath = %q, want /tmp/flag.db", cfg.DBPath)
	}
}

func TestExplicitEmptyCORSFlagClearsEnv(t *testing.T) {
	cfg, err := load(t,
		[]string{"--cors-origins", ""},
		map[string]string{"HOMEY_CORS_ORIGINS": "https://a.example"},
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CORSOrigins != nil {
		t.Errorf("CORSOrigins = %v, want nil", cfg.CORSOrigins)
	}
}

func TestInvalidLogLevel(t *testing.T) {
	_, err := load(t, nil, map[string]string{"HOMEY_LOG_LEVEL": "verbose"})
	if err == nil || !strings.Contains(err.Error(), "log level") {
		t.Fatalf("expected log level error, got %v", err)
	}
}

func TestInvalidMCPBoolean(t *testing.T) {
	_, err := load(t, nil, map[string]string{"HOMEY_MCP_ENABLED": "maybe"})
	if err == nil || !strings.Contains(err.Error(), "HOMEY_MCP_ENABLED") {
		t.Fatalf("expected MCP boolean error, got %v", err)
	}
}

func TestUnknownFlagFails(t *testing.T) {
	if _, err := load(t, []string{"--nope"}, nil); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}
