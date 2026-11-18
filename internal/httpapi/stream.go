package httpapi

import (
	"net/http"
	"time"

	"github.com/hmzckd/flagforge/internal/stream"
)

var heartbeatEvent = []byte(": ping\n\n")

func (a *api) sdkStream(w http.ResponseWriter, r *http.Request) {
	env := sdkEnvironment(r.Context())
	rc := http.NewResponseController(w)

	sub, unsubscribe := a.hub.Subscribe(env)
	defer unsubscribe()

	snap, err := a.store.Snapshot(r.Context(), env)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	first, err := stream.EncodeSnapshot(snap)
	if err != nil {
		a.internalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(data []byte) bool {
		if err := rc.SetWriteDeadline(time.Now().Add(a.streamWriteTimeout)); err != nil {
			a.logger.Error("stream write deadline is not supported", "error", err)
			return false
		}
		if _, err := w.Write(data); err != nil {
			a.logger.Info("stream client dropped", "environment", env, "error", err)
			return false
		}
		if err := rc.Flush(); err != nil {
			a.logger.Info("stream client dropped", "environment", env, "error", err)
			return false
		}
		return true
	}

	if !write(first.Data) {
		return
	}
	lastSent := first.Version

	heartbeat := time.NewTicker(a.heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-a.hub.Done():
			return
		case e := <-sub.Events():
			if e.Version <= lastSent {
				continue
			}
			if !write(e.Data) {
				return
			}
			lastSent = e.Version
		case <-heartbeat.C:
			if !write(heartbeatEvent) {
				return
			}
		}
	}
}
