package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hmzckd/flagforge/internal/store"
)

type SnapshotLoader interface {
	Snapshot(ctx context.Context, env string) (store.Snapshot, error)
}

type Event struct {
	Version int64
	Data    []byte
}

func EncodeSnapshot(snap store.Snapshot) (Event, error) {
	body, err := json.Marshal(snap)
	if err != nil {
		return Event{}, err
	}
	data := fmt.Appendf(nil, "event: snapshot\nid: %d\ndata: %s\n\n", snap.Version, body)
	return Event{Version: snap.Version, Data: data}, nil
}

type Subscriber struct {
	env    string
	events chan Event
}

func (s *Subscriber) Events() <-chan Event {
	return s.events
}

func (s *Subscriber) offer(e Event) {
	select {
	case s.events <- e:
		return
	default:
	}
	select {
	case <-s.events:
	default:
	}
	select {
	case s.events <- e:
	default:
	}
}

type Hub struct {
	loader SnapshotLoader
	logger *slog.Logger

	mu        sync.Mutex
	subs      map[string]map[*Subscriber]struct{}
	published map[string]int64
	dirty     map[string]bool

	wake      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func NewHub(loader SnapshotLoader, logger *slog.Logger) *Hub {
	return &Hub{
		loader:    loader,
		logger:    logger,
		subs:      map[string]map[*Subscriber]struct{}{},
		published: map[string]int64{},
		dirty:     map[string]bool{},
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

func (h *Hub) Subscribe(env string) (*Subscriber, func()) {
	sub := &Subscriber{env: env, events: make(chan Event, 1)}

	h.mu.Lock()
	if h.subs[env] == nil {
		h.subs[env] = map[*Subscriber]struct{}{}
	}
	h.subs[env][sub] = struct{}{}
	h.mu.Unlock()

	return sub, func() {
		h.mu.Lock()
		delete(h.subs[env], sub)
		if len(h.subs[env]) == 0 {
			delete(h.subs, env)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Subscribers(env string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[env])
}

func (h *Hub) Done() <-chan struct{} {
	return h.done
}

func (h *Hub) Close() {
	h.closeOnce.Do(func() { close(h.done) })
}

func (h *Hub) Notify(env string) {
	h.mu.Lock()
	h.dirty[env] = true
	h.mu.Unlock()
	h.signal()
}

func (h *Hub) NotifyAll() {
	h.mu.Lock()
	for env := range h.subs {
		h.dirty[env] = true
	}
	h.mu.Unlock()
	h.signal()
}

func (h *Hub) signal() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.done:
			return
		case <-h.wake:
			h.refreshDirty(ctx)
		}
	}
}

func (h *Hub) refreshDirty(ctx context.Context) {
	h.mu.Lock()
	envs := make([]string, 0, len(h.dirty))
	for env := range h.dirty {
		if len(h.subs[env]) > 0 {
			envs = append(envs, env)
		}
	}
	h.dirty = map[string]bool{}
	h.mu.Unlock()

	for _, env := range envs {
		loadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		snap, err := h.loader.Snapshot(loadCtx, env)
		cancel()
		if err != nil {
			h.logger.Error("load snapshot for stream", "environment", env, "error", err)
			continue
		}
		h.Publish(snap)
	}
}

func (h *Hub) Publish(snap store.Snapshot) {
	event, err := EncodeSnapshot(snap)
	if err != nil {
		h.logger.Error("encode snapshot", "environment", snap.Environment, "error", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if snap.Version <= h.published[snap.Environment] {
		return
	}
	h.published[snap.Environment] = snap.Version
	for sub := range h.subs[snap.Environment] {
		sub.offer(event)
	}
}
