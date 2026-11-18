// Package sdk lets a Go application read FlagForge flags.
//
// The client downloads every flag of one environment when it starts and then
// answers Bool calls from memory, without any network call. When FlagForge
// cannot be reached, Bool returns the default value the caller passed in.
package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hmzckd/flagforge/eval"
)

type User = eval.User

type Config struct {
	BaseURL     string
	SDKKey      string
	InitTimeout time.Duration
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

var ErrNotInitialized = errors.New("flagforge: could not load flags, serving default values")

type snapshot struct {
	version int64
	flags   map[string]eval.Flag
}

type Client struct {
	cfg      Config
	snapshot atomic.Pointer[snapshot]
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.InitTimeout <= 0 {
		cfg.InitTimeout = 2 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	c := &Client{cfg: cfg}

	if cfg.BaseURL == "" || cfg.SDKKey == "" {
		return c, fmt.Errorf("%w: BaseURL and SDKKey are required", ErrNotInitialized)
	}

	initCtx, cancel := context.WithTimeout(ctx, cfg.InitTimeout)
	defer cancel()
	snap, err := c.fetchSnapshot(initCtx)
	if err != nil {
		return c, fmt.Errorf("%w: %v", ErrNotInitialized, err)
	}
	c.snapshot.Store(snap)
	return c, nil
}

func (c *Client) Bool(flagKey string, user User, defaultValue bool) bool {
	if c == nil {
		return defaultValue
	}
	snap := c.snapshot.Load()
	if snap == nil {
		return defaultValue
	}
	flag, ok := snap.flags[flagKey]
	if !ok {
		return defaultValue
	}
	return eval.Evaluate(flag, user).Value
}

func (c *Client) Ready() bool {
	return c != nil && c.snapshot.Load() != nil
}

func (c *Client) Version() int64 {
	if c == nil {
		return 0
	}
	if snap := c.snapshot.Load(); snap != nil {
		return snap.version
	}
	return 0
}

func (c *Client) Close() error {
	return nil
}

type snapshotDocument struct {
	Environment string               `json:"environment"`
	Version     int64                `json:"version"`
	Flags       map[string]eval.Flag `json:"flags"`
}

func parseSnapshot(data []byte) (*snapshot, error) {
	var doc snapshotDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	flags := make(map[string]eval.Flag, len(doc.Flags))
	for key, f := range doc.Flags {
		f.Key = key
		flags[key] = f
	}
	return &snapshot{version: doc.Version, flags: flags}, nil
}

func (c *Client) fetchSnapshot(ctx context.Context) (*snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/sdk/v1/flags", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.SDKKey)

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, errors.New("the SDK key was rejected")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return parseSnapshot(body)
}
