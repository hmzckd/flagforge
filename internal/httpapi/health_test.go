package httpapi

import (
	"net/http"
	"testing"
)

func TestHealthReturnsOK(t *testing.T) {
	rec := send(t, newTestHandler(t), request{method: http.MethodGet, path: "/health"})

	wantStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if body := decode[map[string]string](t, rec); body["status"] != "ok" {
		t.Fatalf(`status field = %q, want "ok"`, body["status"])
	}
}

func TestHealthRejectsPost(t *testing.T) {
	rec := send(t, newTestHandler(t), request{method: http.MethodPost, path: "/health"})
	wantStatus(t, rec, http.StatusMethodNotAllowed)
}

func TestReadyWhenDatabaseIsReachable(t *testing.T) {
	rec := send(t, newTestHandler(t), request{method: http.MethodGet, path: "/ready"})

	wantStatus(t, rec, http.StatusOK)
	if body := decode[map[string]string](t, rec); body["status"] != "ready" {
		t.Fatalf(`status field = %q, want "ready"`, body["status"])
	}
}
