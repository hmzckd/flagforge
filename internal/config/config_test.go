package config

import (
	"log/slog"
	"testing"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
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
		"FLAGFORGE_ADDR":      "127.0.0.1:9090",
		"FLAGFORGE_LOG_LEVEL": "debug",
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
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := map[string]map[string]string{
		"address without port": {"FLAGFORGE_ADDR": "localhost"},
		"unknown log level":    {"FLAGFORGE_LOG_LEVEL": "loud"},
	}
	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(envFrom(env)); err == nil {
				t.Fatal("Load returned no error")
			}
		})
	}
}
