package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAdminPutsTheKeyNameIntoTheContext(t *testing.T) {
	keys := map[string]string{
		"hamza": "hamza-key-0123456789abcdef0123456789",
		"ci":    "ci-key-0123456789abcdef0123456789abc",
	}
	var seen string
	h := requireAdmin(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = actorFrom(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+keys["ci"])
	h.ServeHTTP(httptest.NewRecorder(), r)

	if seen != "ci" {
		t.Fatalf("actor = %q, want ci", seen)
	}
}
