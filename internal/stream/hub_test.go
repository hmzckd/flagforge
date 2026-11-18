package stream

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/hmzckd/flagforge/eval"
	"github.com/hmzckd/flagforge/internal/store"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func snapshot(env string, version int64) store.Snapshot {
	return store.Snapshot{Environment: env, Version: version, Flags: map[string]eval.Flag{}}
}

func TestEncodeSnapshot(t *testing.T) {
	e, err := EncodeSnapshot(snapshot("production", 42))
	if err != nil {
		t.Fatal(err)
	}
	want := "event: snapshot\nid: 42\ndata: {\"environment\":\"production\",\"version\":42,\"flags\":{}}\n\n"
	if string(e.Data) != want || e.Version != 42 {
		t.Fatalf("event = %q (version %d)\nwant    %q", e.Data, e.Version, want)
	}
}

func TestPublishReachesOnlyTheSameEnvironment(t *testing.T) {
	h := NewHub(nil, testLogger())
	prod, unsubscribeProd := h.Subscribe("production")
	defer unsubscribeProd()
	staging, unsubscribeStaging := h.Subscribe("staging")
	defer unsubscribeStaging()

	h.Publish(snapshot("production", 2))

	select {
	case e := <-prod.Events():
		if e.Version != 2 {
			t.Fatalf("version = %d", e.Version)
		}
	default:
		t.Fatal("production subscriber got nothing")
	}
	select {
	case e := <-staging.Events():
		t.Fatalf("staging subscriber got %q", e.Data)
	default:
	}
}

func TestSlowSubscriberNeverBlocksAndKeepsOnlyTheLatest(t *testing.T) {
	h := NewHub(nil, testLogger())
	slow, unsubscribe := h.Subscribe("production")
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for v := int64(1); v <= 1000; v++ {
			h.Publish(snapshot("production", v))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a subscriber that never reads")
	}

	if e := <-slow.Events(); e.Version != 1000 {
		t.Fatalf("slow subscriber got version %d, want only the latest (1000)", e.Version)
	}
}

func TestOlderSnapshotsAreNotPublishedAfterNewerOnes(t *testing.T) {
	h := NewHub(nil, testLogger())
	sub, unsubscribe := h.Subscribe("production")
	defer unsubscribe()

	h.Publish(snapshot("production", 5))
	<-sub.Events()
	h.Publish(snapshot("production", 4))
	h.Publish(snapshot("production", 5))

	select {
	case e := <-sub.Events():
		t.Fatalf("got version %d after 5 was already published", e.Version)
	default:
	}
}

func TestUnsubscribeRemovesTheSubscriber(t *testing.T) {
	h := NewHub(nil, testLogger())
	_, unsubscribe := h.Subscribe("production")
	if h.Subscribers("production") != 1 {
		t.Fatal("subscriber was not added")
	}
	unsubscribe()
	if h.Subscribers("production") != 0 {
		t.Fatal("subscriber was not removed")
	}
}

type countingLoader struct {
	mu      sync.Mutex
	calls   map[string]int
	version int64
}

func (l *countingLoader) Snapshot(ctx context.Context, env string) (store.Snapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls[env]++
	l.version++
	return snapshot(env, l.version), nil
}

func (l *countingLoader) count(env string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls[env]
}

func TestNotifyLoadsTheSnapshotOncePerChangeForAllSubscribers(t *testing.T) {
	loader := &countingLoader{calls: map[string]int{}}
	h := NewHub(loader, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)

	subs := make([]*Subscriber, 50)
	for i := range subs {
		var unsubscribe func()
		subs[i], unsubscribe = h.Subscribe("production")
		defer unsubscribe()
	}

	h.Notify("production")
	for i, sub := range subs {
		select {
		case <-sub.Events():
		case <-time.After(2 * time.Second):
			t.Fatalf("subscriber %d got nothing", i)
		}
	}
	if got := loader.count("production"); got != 1 {
		t.Fatalf("snapshot was loaded %d times for 50 subscribers, want 1", got)
	}
}

func TestNotifySkipsEnvironmentsNobodyListensTo(t *testing.T) {
	loader := &countingLoader{calls: map[string]int{}}
	h := NewHub(loader, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)

	sub, unsubscribe := h.Subscribe("staging")
	defer unsubscribe()
	h.Notify("production")
	h.NotifyAll()

	select {
	case <-sub.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("staging subscriber got nothing after NotifyAll")
	}
	if got := loader.count("production"); got != 0 {
		t.Fatalf("production was loaded %d times with no subscribers", got)
	}
}

func TestCloseIsSafeToCallTwice(t *testing.T) {
	h := NewHub(nil, testLogger())
	h.Close()
	h.Close()
	select {
	case <-h.Done():
	default:
		t.Fatal("Done is not closed")
	}
}
