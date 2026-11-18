package sdk

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func checkoutSnapshot(version int, enabled bool) string {
	state := "false"
	if enabled {
		state = "true"
	}
	return `{"environment": "production", "version": ` + strconv.Itoa(version) + `, "flags": {
		"new-checkout": {"enabled": ` + state + `, "rules": [], "fallthrough": {"value": true}}
	}}`
}

type liveServer struct {
	*httptest.Server
	heartbeat time.Duration
	streams   atomic.Int64

	mu      sync.Mutex
	body    string
	conns   map[*liveConn]struct{}
	stopped bool
}

type liveConn struct {
	events chan string
	drop   chan struct{}
}

func newLiveServer(t *testing.T, body string, heartbeat time.Duration) *liveServer {
	t.Helper()
	s := &liveServer{body: body, heartbeat: heartbeat, conns: map[*liveConn]struct{}{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Stop)
	return s
}

func (s *liveServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+testKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/sdk/v1/flags" {
		s.mu.Lock()
		body := s.body
		s.mu.Unlock()
		_, _ = io.WriteString(w, body)
		return
	}

	conn := &liveConn{events: make(chan string, 16), drop: make(chan struct{})}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	s.streams.Add(1)
	s.conns[conn] = struct{}{}
	conn.events <- s.body
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()

	var heartbeat <-chan time.Time
	if s.heartbeat > 0 {
		ticker := time.NewTicker(s.heartbeat)
		defer ticker.Stop()
		heartbeat = ticker.C
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-conn.drop:
			return
		case body := <-conn.events:
			_, _ = io.WriteString(w, sseSnapshot(body))
		case <-heartbeat:
			_, _ = io.WriteString(w, ": ping\n\n")
		}
		w.(http.Flusher).Flush()
	}
}

func (s *liveServer) Publish(body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body = body
	for conn := range s.conns {
		conn.events <- body
	}
}

func (s *liveServer) SetQuietly(body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body = body
}

func (s *liveServer) DropStreams() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.conns {
		close(conn.drop)
		delete(s.conns, conn)
	}
}

func (s *liveServer) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.DropStreams()
	s.Close()
}

func fastReconnect(url string) Config {
	return Config{
		BaseURL:           url,
		SDKKey:            testKey,
		ReconnectMinDelay: 10 * time.Millisecond,
		ReconnectMaxDelay: 50 * time.Millisecond,
	}
}

func eventually(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within %v", what, within)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

var anyone = User{Key: "u-1"}

func TestStreamDeliversAChangeWithoutRestarting(t *testing.T) {
	srv := newLiveServer(t, checkoutSnapshot(1, false), 0)
	client := newClient(t, fastReconnect(srv.URL), true)
	eventually(t, time.Second, "the stream to connect", func() bool { return srv.streams.Load() == 1 })

	if client.Bool("new-checkout", anyone, true) {
		t.Fatal("flag is off, want false")
	}

	start := time.Now()
	srv.Publish(checkoutSnapshot(2, true))
	eventually(t, time.Second, "the change to reach the client", func() bool {
		return client.Bool("new-checkout", anyone, false)
	})
	t.Logf("change reached the client in %v", time.Since(start))

	if client.Version() != 2 {
		t.Fatalf("Version = %d, want 2", client.Version())
	}
}

func TestReconnectsAndCatchesUpAfterTheStreamDrops(t *testing.T) {
	srv := newLiveServer(t, checkoutSnapshot(1, false), 0)
	client := newClient(t, fastReconnect(srv.URL), true)
	eventually(t, time.Second, "the stream to connect", func() bool { return srv.streams.Load() == 1 })

	srv.SetQuietly(checkoutSnapshot(2, true))
	srv.DropStreams()

	eventually(t, time.Second, "a second connection", func() bool { return srv.streams.Load() >= 2 })
	eventually(t, time.Second, "the missed change to arrive", func() bool {
		return client.Bool("new-checkout", anyone, false)
	})
}

func TestKeepsTheLastKnownFlagsWhenTheServerGoesAway(t *testing.T) {
	srv := newLiveServer(t, checkoutSnapshot(3, true), 0)
	client := newClient(t, fastReconnect(srv.URL), true)
	eventually(t, time.Second, "the stream to connect", func() bool { return srv.streams.Load() == 1 })

	srv.Stop()
	time.Sleep(200 * time.Millisecond)

	if !client.Bool("new-checkout", anyone, false) {
		t.Fatal("client forgot the last known flags")
	}
	if !client.Ready() || client.Version() != 3 {
		t.Fatalf("Ready = %v, Version = %d", client.Ready(), client.Version())
	}
}

func TestBecomesReadyWhenTheServerComesUpLater(t *testing.T) {
	addr := freeAddress(t)
	client, err := New(context.Background(), fastReconnect("http://"+addr))
	t.Cleanup(func() { client.Close() })
	if !errors.Is(err, ErrNotInitialized) || client.Ready() {
		t.Fatalf("err = %v, Ready = %v; want a client that is not ready yet", err, client.Ready())
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &liveServer{body: checkoutSnapshot(4, true), conns: map[*liveConn]struct{}{}}
	srv.Server = httptest.NewUnstartedServer(http.HandlerFunc(srv.handle))
	srv.Listener.Close()
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Stop)

	eventually(t, 2*time.Second, "the client to become ready", client.Ready)
	if !client.Bool("new-checkout", anyone, false) {
		t.Fatal("want the flag from the server")
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestHeartbeatsKeepTheStreamOpen(t *testing.T) {
	srv := newLiveServer(t, checkoutSnapshot(1, true), 50*time.Millisecond)
	cfg := fastReconnect(srv.URL)
	cfg.StreamIdleTimeout = 200 * time.Millisecond
	newClient(t, cfg, true)

	time.Sleep(700 * time.Millisecond)
	if got := srv.streams.Load(); got != 1 {
		t.Fatalf("client opened %d streams, want 1", got)
	}
}

func TestSilentStreamIsReplaced(t *testing.T) {
	srv := newLiveServer(t, checkoutSnapshot(1, true), 0)
	cfg := fastReconnect(srv.URL)
	cfg.StreamIdleTimeout = 200 * time.Millisecond
	newClient(t, cfg, true)

	eventually(t, 2*time.Second, "a new stream after the silent one", func() bool { return srv.streams.Load() >= 2 })
}

func TestRejectedKeyStopsTheStream(t *testing.T) {
	var streams atomic.Int64
	srv := newFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sdk/v1/stream" {
			streams.Add(1)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		serveSnapshot(checkoutSnapshot(1, true))(w, r)
	})
	client := newClient(t, fastReconnect(srv.URL), true)

	eventually(t, time.Second, "the first stream attempt", func() bool { return streams.Load() == 1 })
	time.Sleep(300 * time.Millisecond)

	if got := streams.Load(); got != 1 {
		t.Fatalf("client tried %d times with a rejected key, want 1", got)
	}
	if !client.Bool("new-checkout", anyone, false) {
		t.Fatal("client must keep serving the flags it loaded")
	}
}

func TestCloseStopsEveryGoroutine(t *testing.T) {
	before := goleak.IgnoreCurrent()
	srv := newLiveServer(t, checkoutSnapshot(1, true), 0)
	client := newClient(t, fastReconnect(srv.URL), true)
	eventually(t, time.Second, "the stream to connect", func() bool { return srv.streams.Load() == 1 })

	start := time.Now()
	client.Close()
	client.Close()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Close took %v", elapsed)
	}
	srv.Stop()

	goleak.VerifyNone(t, before)
}

func TestBackoffStaysWithinItsCeiling(t *testing.T) {
	minDelay, maxDelay := 100*time.Millisecond, 3*time.Second
	for attempt := range 40 {
		ceiling := maxDelay
		if attempt < 30 {
			ceiling = min(minDelay<<attempt, maxDelay)
		}
		var lowest, highest time.Duration = ceiling, 0
		for range 2000 {
			d := backoff(attempt, minDelay, maxDelay)
			if d < 0 || d > ceiling {
				t.Fatalf("attempt %d: delay %v outside [0, %v]", attempt, d, ceiling)
			}
			lowest, highest = min(lowest, d), max(highest, d)
		}
		if lowest > ceiling/4 || highest < ceiling*3/4 {
			t.Fatalf("attempt %d: delays only between %v and %v, want them spread over [0, %v]", attempt, lowest, highest, ceiling)
		}
	}
}
