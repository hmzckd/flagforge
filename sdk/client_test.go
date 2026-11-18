package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "ffsdk_test"

const turkeySnapshot = `{
	"environment": "production",
	"version": 7,
	"flags": {
		"new-checkout": {
			"key": "new-checkout",
			"enabled": true,
			"rules": [{"clauses": [{"attribute": "country", "op": "in", "values": ["TR"]}], "serve": {"value": true}}],
			"fallthrough": {"value": false}
		}
	}
}`

type fakeServer struct {
	*httptest.Server
	fetches atomic.Int64
}

func newFakeServer(t *testing.T, handler http.HandlerFunc) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sdk/v1/flags" {
			f.fetches.Add(1)
		}
		handler(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func serveSnapshot(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/sdk/v1/flags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		case "/sdk/v1/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, sseSnapshot(body))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}
}

func sseSnapshot(body string) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(body)); err != nil {
		compact.Reset()
		compact.WriteString(body)
	}
	return "event: snapshot\ndata: " + compact.String() + "\n\n"
}

func newClient(t *testing.T, cfg Config, wantReady bool) *Client {
	t.Helper()
	client, err := New(context.Background(), cfg)
	t.Cleanup(func() { client.Close() })
	if wantReady && err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func TestBoolEvaluatesLocallyAfterStartup(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(turkeySnapshot))

	client := newClient(t, Config{BaseURL: srv.URL, SDKKey: testKey}, true)
	fetchesAfterStart := srv.fetches.Load()

	tr := User{Key: "u-1", Attributes: map[string]string{"country": "TR"}}
	de := User{Key: "u-2", Attributes: map[string]string{"country": "DE"}}
	for range 1000 {
		if !client.Bool("new-checkout", tr, false) {
			t.Fatal("TR user must get true")
		}
		if client.Bool("new-checkout", de, true) {
			t.Fatal("DE user must get false")
		}
	}

	if got := srv.fetches.Load(); got != fetchesAfterStart {
		t.Fatalf("Bool made %d network calls, want 0", got-fetchesAfterStart)
	}
	if !client.Ready() || client.Version() != 7 {
		t.Fatalf("Ready = %v, Version = %d", client.Ready(), client.Version())
	}
}

func TestBoolUsesTheSameBucketsAsTheServer(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(`{"version": 1, "flags": {
		"new-checkout": {"enabled": true, "rules": [], "fallthrough": {"rollout": {"true_percent": 50}}}
	}}`))
	client := newClient(t, Config{BaseURL: srv.URL, SDKKey: testKey}, true)

	if client.Bool("new-checkout", User{Key: "u-1"}, true) {
		t.Fatal("u-1 is in bucket 6473 of new-checkout, so a 50% rollout must serve false")
	}
	if !client.Bool("new-checkout", User{Key: "u-2"}, false) {
		t.Fatal("u-2 is in bucket 3478 of new-checkout, so a 50% rollout must serve true")
	}
}

func TestUnknownFlagReturnsTheDefault(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(turkeySnapshot))
	client := newClient(t, Config{BaseURL: srv.URL, SDKKey: testKey}, true)

	if !client.Bool("does-not-exist", User{Key: "u-1"}, true) || client.Bool("does-not-exist", User{Key: "u-1"}, false) {
		t.Fatal("unknown flag must return the default value")
	}
}

func TestServerDownServesDefaults(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(turkeySnapshot))
	url := srv.URL
	srv.Close()

	start := time.Now()
	client, err := New(context.Background(), Config{BaseURL: url, SDKKey: testKey})
	t.Cleanup(func() { client.Close() })

	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("err = %v, want ErrNotInitialized", err)
	}
	if client == nil {
		t.Fatal("New must return a usable client even when it fails")
	}
	if !client.Bool("new-checkout", User{Key: "u-1"}, true) || client.Bool("new-checkout", User{Key: "u-1"}, false) {
		t.Fatal("client without flags must return the default value")
	}
	if client.Ready() {
		t.Fatal("client must not be ready")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("New took %v with the server down", elapsed)
	}
}

func TestSlowServerDoesNotBlockStartupForLong(t *testing.T) {
	release := make(chan struct{})
	srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)

	start := time.Now()
	client, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: testKey, InitTimeout: 200 * time.Millisecond})
	elapsed := time.Since(start)
	t.Cleanup(func() { client.Close() })

	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("err = %v, want ErrNotInitialized", err)
	}
	if elapsed < 150*time.Millisecond || elapsed > time.Second {
		t.Fatalf("New returned after %v, want about the 200ms init timeout", elapsed)
	}
	if client.Bool("new-checkout", User{Key: "u-1"}, true) != true {
		t.Fatal("want the default")
	}
}

func TestRejectedKey(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(turkeySnapshot))
	client, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: "wrong"})
	t.Cleanup(func() { client.Close() })
	if !errors.Is(err, ErrNotInitialized) || !strings.Contains(err.Error(), "SDK key was rejected") {
		t.Fatalf("err = %v", err)
	}
}

func TestBrokenSnapshotServesDefaults(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(`{"flags": [1, 2, 3]}`))
	client, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: testKey})
	t.Cleanup(func() { client.Close() })
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("err = %v", err)
	}
	if !client.Bool("new-checkout", User{Key: "u-1"}, true) {
		t.Fatal("want the default")
	}
}

func TestMissingConfig(t *testing.T) {
	client, err := New(context.Background(), Config{})
	t.Cleanup(func() { client.Close() })
	if !errors.Is(err, ErrNotInitialized) || client == nil {
		t.Fatalf("client = %v, err = %v", client, err)
	}
}

func TestNilClientNeverPanics(t *testing.T) {
	var client *Client
	if !client.Bool("new-checkout", User{Key: "u-1"}, true) || client.Ready() || client.Version() != 0 || client.Close() != nil {
		t.Fatal("nil client must behave like a client without flags")
	}
}
