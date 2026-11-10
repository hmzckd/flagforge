package config

import (
	"fmt"
	"log/slog"
	"net"
)

type Config struct {
	Addr     string
	LogLevel slog.Level
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{Addr: ":8080", LogLevel: slog.LevelInfo}

	if v := getenv("FLAGFORGE_ADDR"); v != "" {
		cfg.Addr = v
	}
	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		return Config{}, fmt.Errorf("FLAGFORGE_ADDR must look like host:port or :port, got %q", cfg.Addr)
	}

	if v := getenv("FLAGFORGE_LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("FLAGFORGE_LOG_LEVEL must be debug, info, warn or error, got %q", v)
		}
	}

	return cfg, nil
}
