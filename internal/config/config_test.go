package config

import (
	"log/slog"
	"strings"
	"testing"
)

const validKey = "0123456789abcdef0123456789abcdef"

func envFrom(values map[string]string) func(string) string {
	base := map[string]string{
		"FLAGFORGE_DATABASE_URL": "postgres://localhost/flagforge",
		"FLAGFORGE_ADMIN_KEYS":   "hamza=" + validKey,
	}
	for k, v := range values {
		base[k] = v
	}
	return func(key string) string { return base[key] }
}

func TestLoadUsesDefaults(t *testing.T) {
	cfg, err := Load(envFrom(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want INFO", cfg.LogLevel)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	cfg, err := Load(envFrom(map[string]string{
		"FLAGFORGE_ADDR":       "127.0.0.1:9090",
		"FLAGFORGE_LOG_LEVEL":  "debug",
		"FLAGFORGE_ADMIN_KEYS": "hamza=" + validKey + ", ci=" + strings.Repeat("x", 40),
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9090" {
		t.Errorf("Addr = %q, want 127.0.0.1:9090", cfg.Addr)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want DEBUG", cfg.LogLevel)
	}
	if cfg.DatabaseURL != "postgres://localhost/flagforge" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if len(cfg.AdminKeys) != 2 || cfg.AdminKeys["hamza"] != validKey || cfg.AdminKeys["ci"] != strings.Repeat("x", 40) {
		t.Errorf("AdminKeys = %v", cfg.AdminKeys)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := map[string]map[string]string{
		"address without port":   {"FLAGFORGE_ADDR": "localhost"},
		"unknown log level":      {"FLAGFORGE_LOG_LEVEL": "loud"},
		"missing database url":   {"FLAGFORGE_DATABASE_URL": ""},
		"missing admin keys":     {"FLAGFORGE_ADMIN_KEYS": ""},
		"admin key without name": {"FLAGFORGE_ADMIN_KEYS": validKey},
		"short admin key":        {"FLAGFORGE_ADMIN_KEYS": "hamza=short"},
		"bad admin name":         {"FLAGFORGE_ADMIN_KEYS": "Hamza Ç=" + validKey},
		"duplicate admin name":   {"FLAGFORGE_ADMIN_KEYS": "hamza=" + validKey + ",hamza=" + validKey},
	}
	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(envFrom(env)); err == nil {
				t.Fatal("Load returned no error")
			}
		})
	}
}
