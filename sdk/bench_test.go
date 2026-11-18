package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hmzckd/flagforge/eval"
)

var benchUser = User{Key: "user-12345", Attributes: map[string]string{"country": "TR"}}

func BenchmarkBoolLocal(b *testing.B) {
	srv := httptest.NewServer(serveSnapshot(turkeySnapshot))
	defer srv.Close()
	client, err := New(context.Background(), Config{BaseURL: srv.URL, SDKKey: testKey})
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		client.Bool("new-checkout", benchUser, false)
	}
}

func BenchmarkEvaluateOverHTTP(b *testing.B) {
	snap, err := parseSnapshot([]byte(turkeySnapshot))
	if err != nil {
		b.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Flag string    `json:"flag"`
			User eval.User `json:"user"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(eval.Evaluate(snap.flags[req.Flag], req.User))
	}))
	defer srv.Close()
	body, _ := json.Marshal(map[string]any{"flag": "new-checkout", "user": benchUser})

	for b.Loop() {
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		var result eval.Result
		_ = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
	}
}

func BenchmarkSnapshotReadAtomicPointer(b *testing.B) {
	var p atomic.Pointer[snapshot]
	p.Store(&snapshot{version: 1})
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Load().version
		}
	})
}

func BenchmarkSnapshotReadRWMutex(b *testing.B) {
	var mu sync.RWMutex
	snap := &snapshot{version: 1}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			_ = snap.version
			mu.RUnlock()
		}
	})
}
