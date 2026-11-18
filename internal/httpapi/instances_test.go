package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/hmzckd/flagforge/internal/testdb"
)

func TestChangeOnOneInstanceReachesStreamsOnTheOther(t *testing.T) {
	a := newTestInstance(t, Deps{}, 50*time.Millisecond)
	b := newTestInstance(t, Deps{}, 50*time.Millisecond)
	serverB := httptest.NewServer(b.handler)
	t.Cleanup(serverB.Close)

	createCheckoutFlag(t, a.handler)
	key := createSDKKey(t, a.handler, "production").Key
	streamB := openStream(t, serverB.URL, key)
	streamB.next(t, 2*time.Second)

	start := time.Now()
	putSetting(t, a.handler, "/api/flags/new-checkout/environments/production", `{"enabled":true}`)
	e := streamB.next(t, 2*time.Second)

	if !strings.Contains(e.data, `"enabled":true`) {
		t.Fatalf("instance B streamed %s", e.data)
	}
	t.Logf("change made on A reached a stream on B in %v", time.Since(start))
}

func TestNewFlagOnOneInstanceReachesEveryEnvironmentOnTheOther(t *testing.T) {
	a := newTestInstance(t, Deps{}, 50*time.Millisecond)
	b := newTestInstance(t, Deps{}, 50*time.Millisecond)
	serverB := httptest.NewServer(b.handler)
	t.Cleanup(serverB.Close)

	staging := openStream(t, serverB.URL, createSDKKey(t, a.handler, "staging").Key)
	production := openStream(t, serverB.URL, createSDKKey(t, a.handler, "production").Key)
	staging.next(t, 2*time.Second)
	production.next(t, 2*time.Second)

	createCheckoutFlag(t, a.handler)

	for name, c := range map[string]*sseClient{"staging": staging, "production": production} {
		if e := c.next(t, 2*time.Second); !strings.Contains(e.data, "new-checkout") {
			t.Fatalf("%s stream on B got %s", name, e.data)
		}
	}
}

func TestListenerCatchesUpAfterLosingItsConnection(t *testing.T) {
	inst := newTestInstance(t, Deps{}, 500*time.Millisecond)
	srv := httptest.NewServer(inst.handler)
	t.Cleanup(srv.Close)
	createCheckoutFlag(t, inst.handler)
	c := openStream(t, srv.URL, createSDKKey(t, inst.handler, "production").Key)
	c.next(t, 2*time.Second)

	killListenerConnections(t)
	waitFor(t, "the listener to notice", func() bool { return !inst.listener.Listening() })

	putSetting(t, inst.handler, "/api/flags/new-checkout/environments/production", `{"enabled":true}`)

	e := c.next(t, 5*time.Second)
	if !strings.Contains(e.data, `"enabled":true`) {
		t.Fatalf("stream got %s", e.data)
	}
	if got := inst.listener.Connects(); got != 2 {
		t.Fatalf("listener connected %d times, want 2 (first connect and one reconnect)", got)
	}
}

func killListenerConnections(t *testing.T) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), testdb.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var killed int
	err = conn.QueryRow(context.Background(),
		`SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE application_name = 'flagforge-listener'`,
	).Scan(&killed)
	if err != nil || killed == 0 {
		t.Fatalf("terminated %d listener connections: %v", killed, err)
	}
}
