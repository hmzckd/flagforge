package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hmzckd/flagforge/internal/store"
	"github.com/hmzckd/flagforge/internal/testdb"
)

const testAdminKey = "test-admin-key-0123456789abcdef0123"

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return NewHandler(Deps{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:     store.New(testdb.Pool(t)),
		AdminKeys: map[string]string{"tester": testAdminKey},
	})
}

type request struct {
	method  string
	path    string
	body    string
	token   string
	headers map[string]string
}

func send(t *testing.T, h http.Handler, req request) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if req.body != "" {
		body = strings.NewReader(req.body)
	}
	r := httptest.NewRequest(req.method, req.path, body)
	if req.token != "" {
		r.Header.Set("Authorization", "Bearer "+req.token)
	}
	for k, v := range req.headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func asAdmin(method, path, body string) request {
	return request{method: method, path: path, body: body, token: testAdminKey}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, status, rec.Body.String())
	}
}

func wantProblem(t *testing.T, rec *httptest.ResponseRecorder, status int) problem {
	t.Helper()
	wantStatus(t, rec, status)
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	p := decode[problem](t, rec)
	if p.Status != status || p.Title != http.StatusText(status) {
		t.Fatalf("problem = %+v", p)
	}
	return p
}
