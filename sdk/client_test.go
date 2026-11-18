package sdk

import (
	"context"
	"errors"
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
	hits atomic.Int64
}

func newFakeServer(t *testing.T, handler http.HandlerFunc) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func serveSnapshot(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sdk/v1/flags" || r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestBoolEvaluatesLocallyAfterStartup(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(turkeySnapshot))

	client, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: testKey})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer client.Close()
	hitsAfterStart := srv.hits.Load()

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

	if got := srv.hits.Load(); got != hitsAfterStart {
		t.Fatalf("Bool made %d network calls, want 0", got-hitsAfterStart)
	}
	if !client.Ready() || client.Version() != 7 {
		t.Fatalf("Ready = %v, Version = %d", client.Ready(), client.Version())
	}
}

func TestBoolUsesTheSameBucketsAsTheServer(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(`{"version": 1, "flags": {
		"new-checkout": {"enabled": true, "rules": [], "fallthrough": {"rollout": {"true_percent": 50}}}
	}}`))
	client, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: testKey})
	if err != nil {
		t.Fatal(err)
	}

	if client.Bool("new-checkout", User{Key: "u-1"}, true) {
		t.Fatal("u-1 is in bucket 6473 of new-checkout, so a 50% rollout must serve false")
	}
	if !client.Bool("new-checkout", User{Key: "u-2"}, false) {
		t.Fatal("u-2 is in bucket 3478 of new-checkout, so a 50% rollout must serve true")
	}
}

func TestUnknownFlagReturnsTheDefault(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(turkeySnapshot))
	client, _ := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: testKey})

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
	_, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: "wrong"})
	if !errors.Is(err, ErrNotInitialized) || !strings.Contains(err.Error(), "SDK key was rejected") {
		t.Fatalf("err = %v", err)
	}
}

func TestBrokenSnapshotServesDefaults(t *testing.T) {
	srv := newFakeServer(t, serveSnapshot(`{"flags": [1, 2, 3]}`))
	client, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: testKey})
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("err = %v", err)
	}
	if !client.Bool("new-checkout", User{Key: "u-1"}, true) {
		t.Fatal("want the default")
	}
}

func TestMissingConfig(t *testing.T) {
	client, err := New(context.Background(), Config{})
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
