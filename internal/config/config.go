package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"
)

const minAdminKeyLength = 32

var adminNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type Config struct {
	Addr        string
	LogLevel    slog.Level
	DatabaseURL string
	AdminKeys   map[string]string
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

	cfg.DatabaseURL = getenv("FLAGFORGE_DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("FLAGFORGE_DATABASE_URL is required")
	}

	keys, err := parseAdminKeys(getenv("FLAGFORGE_ADMIN_KEYS"))
	if err != nil {
		return Config{}, err
	}
	cfg.AdminKeys = keys

	return cfg, nil
}

func parseAdminKeys(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("FLAGFORGE_ADMIN_KEYS is required, for example hamza=<at least 32 characters>")
	}
	keys := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		name, key, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			return nil, errors.New("FLAGFORGE_ADMIN_KEYS entries must look like name=key")
		}
		if !adminNamePattern.MatchString(name) {
			return nil, fmt.Errorf("FLAGFORGE_ADMIN_KEYS: invalid name %q (lowercase letters, digits, - and _)", name)
		}
		if len(key) < minAdminKeyLength {
			return nil, fmt.Errorf("FLAGFORGE_ADMIN_KEYS: key for %q must be at least %d characters", name, minAdminKeyLength)
		}
		if _, dup := keys[name]; dup {
			return nil, fmt.Errorf("FLAGFORGE_ADMIN_KEYS: name %q is used twice", name)
		}
		keys[name] = key
	}
	return keys, nil
}
