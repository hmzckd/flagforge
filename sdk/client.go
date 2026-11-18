// Package sdk lets a Go application read FlagForge flags.
//
// The client downloads every flag of one environment when it starts and then
// answers Bool calls from memory, without any network call. A background
// stream keeps that copy up to date. When FlagForge cannot be reached, Bool
// keeps using the last flags it saw, or the default value the caller passed
// in if it never saw any.
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
	"sync"
	"sync/atomic"
	"time"

	"github.com/hmzckd/flagforge/eval"
)

type User = eval.User

type Config struct {
	BaseURL     string
	SDKKey      string
	InitTimeout time.Duration
	Logger      *slog.Logger

	ReconnectMinDelay time.Duration
	ReconnectMaxDelay time.Duration
	StreamIdleTimeout time.Duration
}

var ErrNotInitialized = errors.New("flagforge: could not load flags, serving default values")

var errKeyRejected = errors.New("the SDK key was rejected")

type snapshot struct {
	version int64
	flags   map[string]eval.Flag
}

type Client struct {
	cfg       Config
	transport *http.Transport
	http      *http.Client
	snapshot  atomic.Pointer[snapshot]

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	cfg = withDefaults(cfg)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	c := &Client{cfg: cfg, transport: transport, http: &http.Client{Transport: transport}}

	if cfg.BaseURL == "" || cfg.SDKKey == "" {
		return c, fmt.Errorf("%w: BaseURL and SDKKey are required", ErrNotInitialized)
	}

	initCtx, cancelInit := context.WithTimeout(ctx, cfg.InitTimeout)
	snap, initErr := c.fetchSnapshot(initCtx)
	cancelInit()
	if initErr == nil {
		c.snapshot.Store(snap)
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go c.runStream(streamCtx)

	if initErr != nil {
		return c, fmt.Errorf("%w: %v", ErrNotInitialized, initErr)
	}
	return c, nil
}

func withDefaults(cfg Config) Config {
	if cfg.InitTimeout <= 0 {
		cfg.InitTimeout = 2 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.ReconnectMinDelay <= 0 {
		cfg.ReconnectMinDelay = time.Second
	}
	if cfg.ReconnectMaxDelay <= 0 {
		cfg.ReconnectMaxDelay = 30 * time.Second
	}
	if cfg.StreamIdleTimeout <= 0 {
		cfg.StreamIdleTimeout = 45 * time.Second
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return cfg
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
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		c.wg.Wait()
		c.transport.CloseIdleConnections()
	})
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

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, errKeyRejected
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return parseSnapshot(body)
}
