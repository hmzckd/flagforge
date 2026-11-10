package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const checkoutFlag = `{"key":"new-checkout","name":"New checkout","description":"One page checkout"}`

func TestCreateFlag(t *testing.T) {
	h := newTestHandler(t)

	rec := send(t, h, asAdmin(http.MethodPost, "/api/flags", checkoutFlag))

	wantStatus(t, rec, http.StatusCreated)
	if got := rec.Header().Get("Location"); got != "/api/flags/new-checkout" {
		t.Fatalf("Location = %q", got)
	}
	body := decode[flagResponse](t, rec)
	if body.Key != "new-checkout" || body.Name != "New checkout" || body.Description != "One page checkout" {
		t.Fatalf("body = %+v", body)
	}
	if time.Since(body.CreatedAt) > time.Minute {
		t.Fatalf("created_at = %v, want about now", body.CreatedAt)
	}
}

func TestCreatedFlagCanBeReadAndListed(t *testing.T) {
	h := newTestHandler(t)
	send(t, h, asAdmin(http.MethodPost, "/api/flags", checkoutFlag))
	send(t, h, asAdmin(http.MethodPost, "/api/flags", `{"key":"dark-mode","name":"Dark mode"}`))

	rec := send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout", ""))
	wantStatus(t, rec, http.StatusOK)
	if body := decode[flagResponse](t, rec); body.Name != "New checkout" {
		t.Fatalf("body = %+v", body)
	}

	rec = send(t, h, asAdmin(http.MethodGet, "/api/flags", ""))
	wantStatus(t, rec, http.StatusOK)
	list := decode[struct{ Flags []flagResponse }](t, rec)
	if len(list.Flags) != 2 || list.Flags[0].Key != "dark-mode" || list.Flags[1].Key != "new-checkout" {
		t.Fatalf("flags = %+v, want dark-mode then new-checkout", list.Flags)
	}
}

func TestListIsEmptyArrayWhenThereAreNoFlags(t *testing.T) {
	rec := send(t, newTestHandler(t), asAdmin(http.MethodGet, "/api/flags", ""))

	wantStatus(t, rec, http.StatusOK)
	if got := strings.TrimSpace(rec.Body.String()); got != `{"flags":[]}` {
		t.Fatalf("body = %s", got)
	}
}

func TestCreatingTheSameKeyTwiceIsAConflict(t *testing.T) {
	h := newTestHandler(t)
	send(t, h, asAdmin(http.MethodPost, "/api/flags", checkoutFlag))

	rec := send(t, h, asAdmin(http.MethodPost, "/api/flags", checkoutFlag))

	p := wantProblem(t, rec, http.StatusConflict)
	if !strings.Contains(p.Detail, "new-checkout") {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestUnknownFlagIsNotFound(t *testing.T) {
	rec := send(t, newTestHandler(t), asAdmin(http.MethodGet, "/api/flags/missing", ""))
	wantProblem(t, rec, http.StatusNotFound)
}

func TestCreateFlagRejectsInvalidInput(t *testing.T) {
	tests := map[string]string{
		"empty body":           ``,
		"broken json":          `{"key":`,
		"unknown field":        `{"key":"a","name":"A","enabeld":true}`,
		"two objects":          `{"key":"a","name":"A"}{"key":"b","name":"B"}`,
		"upper case key":       `{"key":"New-Checkout","name":"A"}`,
		"key with space":       `{"key":"new checkout","name":"A"}`,
		"key starts with dash": `{"key":"-new","name":"A"}`,
		"key too long":         `{"key":"` + strings.Repeat("a", 65) + `","name":"A"}`,
		"missing name":         `{"key":"a"}`,
		"name too long":        `{"key":"a","name":"` + strings.Repeat("ş", 101) + `"}`,
		"description too long": `{"key":"a","name":"A","description":"` + strings.Repeat("d", 501) + `"}`,
	}
	h := newTestHandler(t)
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rec := send(t, h, asAdmin(http.MethodPost, "/api/flags", body))
			wantProblem(t, rec, http.StatusBadRequest)
		})
	}
}

func TestCreateFlagRejectsHugeBody(t *testing.T) {
	body := `{"key":"a","name":"A","description":"` + strings.Repeat("x", 2<<20) + `"}`
	rec := send(t, newTestHandler(t), asAdmin(http.MethodPost, "/api/flags", body))

	p := wantProblem(t, rec, http.StatusBadRequest)
	if !strings.Contains(p.Detail, "at most") {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestAdminEndpointsNeedAValidKey(t *testing.T) {
	h := newTestHandler(t)
	tests := map[string]request{
		"no header":       {method: http.MethodGet, path: "/api/flags"},
		"wrong key":       {method: http.MethodGet, path: "/api/flags", token: "wrong-key-0123456789abcdef0123456789"},
		"basic scheme":    {method: http.MethodGet, path: "/api/flags", headers: map[string]string{"Authorization": "Basic " + testAdminKey}},
		"create flag":     {method: http.MethodPost, path: "/api/flags", body: checkoutFlag},
		"get single flag": {method: http.MethodGet, path: "/api/flags/new-checkout"},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			rec := send(t, h, req)
			wantProblem(t, rec, http.StatusUnauthorized)
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("WWW-Authenticate header is missing")
			}
		})
	}

	rec := send(t, h, asAdmin(http.MethodGet, "/api/flags", ""))
	wantStatus(t, rec, http.StatusOK)
}
