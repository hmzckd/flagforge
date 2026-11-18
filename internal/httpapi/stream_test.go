package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hmzckd/flagforge/internal/store"
	"github.com/hmzckd/flagforge/internal/stream"
)

type sseEvent struct {
	name string
	id   string
	data string
}

type sseClient struct {
	resp   *http.Response
	reader *bufio.Reader
	cancel context.CancelFunc
}

func openStream(t *testing.T, serverURL, key string) *sseClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/sdk/v1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open stream: %v", err)
	}
	c := &sseClient{resp: resp, reader: bufio.NewReader(resp.Body), cancel: cancel}
	t.Cleanup(c.close)
	return c
}

func (c *sseClient) close() {
	c.cancel()
	c.resp.Body.Close()
}

func (c *sseClient) next(t *testing.T, timeout time.Duration) sseEvent {
	t.Helper()
	type result struct {
		e   sseEvent
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var e sseEvent
		for {
			line, err := c.reader.ReadString('\n')
			if err != nil {
				ch <- result{err: err}
				return
			}
			line = strings.TrimSuffix(line, "\n")
			switch {
			case line == "":
				ch <- result{e: e}
				return
			case strings.HasPrefix(line, ":"):
				e.name = "comment"
				e.data = strings.TrimSpace(strings.TrimPrefix(line, ":"))
			case strings.HasPrefix(line, "event: "):
				e.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				e.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				e.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("read stream: %v", r.err)
		}
		return r.e
	case <-time.After(timeout):
		t.Fatalf("no event within %v", timeout)
		return sseEvent{}
	}
}

func startStreamServer(t *testing.T, deps Deps) (*httptest.Server, http.Handler, *stream.Hub) {
	t.Helper()
	inst := newTestInstance(t, deps, 50*time.Millisecond)
	srv := httptest.NewServer(inst.handler)
	t.Cleanup(srv.Close)
	return srv, inst.handler, inst.hub
}

func TestStreamSendsTheSnapshotRightAway(t *testing.T) {
	srv, h, _ := startStreamServer(t, Deps{})
	createCheckoutFlag(t, h)
	key := createSDKKey(t, h, "production").Key

	c := openStream(t, srv.URL, key)
	if got := c.resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	e := c.next(t, 2*time.Second)

	var snap store.Snapshot
	if err := json.Unmarshal([]byte(e.data), &snap); err != nil {
		t.Fatalf("data is not a snapshot: %v (%q)", err, e.data)
	}
	if e.name != "snapshot" || e.id != strconv.FormatInt(snap.Version, 10) || snap.Environment != "production" {
		t.Fatalf("event = %+v", e)
	}
	if _, ok := snap.Flags["new-checkout"]; !ok {
		t.Fatalf("snapshot has no new-checkout: %s", e.data)
	}
}

func TestStreamSendsANewSnapshotAfterAChange(t *testing.T) {
	srv, h, _ := startStreamServer(t, Deps{})
	createCheckoutFlag(t, h)
	key := createSDKKey(t, h, "production").Key
	c := openStream(t, srv.URL, key)
	first := c.next(t, 2*time.Second)

	start := time.Now()
	putSetting(t, h, "/api/flags/new-checkout/environments/production", `{"enabled":true}`)
	e := c.next(t, 2*time.Second)

	firstID, _ := strconv.Atoi(first.id)
	if id, _ := strconv.Atoi(e.id); id <= firstID {
		t.Fatalf("id went from %s to %s", first.id, e.id)
	}
	if !strings.Contains(e.data, `"enabled":true`) {
		t.Fatalf("new snapshot = %s", e.data)
	}
	t.Logf("change reached the stream in %v", time.Since(start))
}

func TestStreamIgnoresOtherEnvironments(t *testing.T) {
	srv, h, _ := startStreamServer(t, Deps{HeartbeatInterval: 300 * time.Millisecond})
	createCheckoutFlag(t, h)
	c := openStream(t, srv.URL, createSDKKey(t, h, "staging").Key)
	c.next(t, 2*time.Second)

	putSetting(t, h, "/api/flags/new-checkout/environments/production", `{"enabled":true}`)

	if e := c.next(t, 2*time.Second); e.name != "comment" {
		t.Fatalf("staging stream got %+v after a production change, want only a heartbeat", e)
	}
}

func TestStreamSendsHeartbeats(t *testing.T) {
	srv, h, _ := startStreamServer(t, Deps{HeartbeatInterval: 50 * time.Millisecond})
	c := openStream(t, srv.URL, createSDKKey(t, h, "production").Key)
	c.next(t, 2*time.Second)

	if e := c.next(t, 2*time.Second); e.name != "comment" || e.data != "ping" {
		t.Fatalf("event = %+v, want a ping comment", e)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDisconnectedClientIsCleanedUp(t *testing.T) {
	srv, h, hub := startStreamServer(t, Deps{})
	c := openStream(t, srv.URL, createSDKKey(t, h, "production").Key)
	c.next(t, 2*time.Second)
	waitFor(t, "the subscriber to register", func() bool { return hub.Subscribers("production") == 1 })

	c.close()

	waitFor(t, "the subscriber to go away", func() bool { return hub.Subscribers("production") == 0 })
}

func TestClosingTheHubEndsEveryStream(t *testing.T) {
	srv, h, hub := startStreamServer(t, Deps{})
	c := openStream(t, srv.URL, createSDKKey(t, h, "production").Key)
	c.next(t, 2*time.Second)

	hub.Close()

	done := make(chan error, 1)
	go func() {
		_, err := c.reader.ReadString('\n')
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stream is still open")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end after the hub was closed")
	}
}

func TestSlowClientIsDropped(t *testing.T) {
	srv, h, hub := startStreamServer(t, Deps{StreamWriteTimeout: 200 * time.Millisecond, HeartbeatInterval: time.Hour})
	createCheckoutFlag(t, h)
	key := createSDKKey(t, h, "production").Key

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(4096)
	}
	fmt.Fprintf(conn, "GET /sdk/v1/stream HTTP/1.1\r\nHost: test\r\nAuthorization: Bearer %s\r\n\r\n", key)
	waitFor(t, "the slow client to subscribe", func() bool { return hub.Subscribers("production") == 1 })

	bigRules := bigRuleSet()
	for i := 0; hub.Subscribers("production") == 1; i++ {
		if i == 40 {
			t.Fatal("a client that never reads was not dropped")
		}
		rec := putSetting(t, h, "/api/flags/new-checkout/environments/production", fmt.Sprintf(`{"enabled":%v,"rules":%s}`, i%2 == 0, bigRules))
		wantStatus(t, rec, http.StatusOK)
		time.Sleep(100 * time.Millisecond)
	}
}

func bigRuleSet() string {
	values := make([]string, 100)
	for i := range values {
		values[i] = `"` + strings.Repeat("x", 250) + strconv.Itoa(i) + `"`
	}
	rule := `{"clauses":[{"attribute":"email","op":"in","values":[` + strings.Join(values, ",") + `]}],"serve":{"value":true}}`
	rules := make([]string, 35)
	for i := range rules {
		rules[i] = rule
	}
	return "[" + strings.Join(rules, ",") + "]"
}

func TestStreamNeedsAnSDKKey(t *testing.T) {
	rec := send(t, newTestHandler(t), request{method: http.MethodGet, path: "/sdk/v1/stream", token: testAdminKey})
	wantProblem(t, rec, http.StatusUnauthorized)
}
