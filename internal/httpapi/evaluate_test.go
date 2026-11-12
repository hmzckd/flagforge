package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const evaluateCheckout = `{"flag":"new-checkout","user":{"key":"u-1","attributes":{"country":"TR"}}}`

func TestEvaluateOffFlag(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	rec := send(t, h, asAdmin(http.MethodPost, "/api/environments/production/evaluate", evaluateCheckout))

	wantStatus(t, rec, http.StatusOK)
	got := strings.TrimSpace(rec.Body.String())
	if want := `{"flag":"new-checkout","value":false,"reason":{"kind":"OFF"}}`; got != want {
		t.Fatalf("body = %s\nwant   %s", got, want)
	}
}

func TestEvaluateOnFlagFallsThrough(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	send(t, h, asAdmin(http.MethodPut, "/api/flags/new-checkout/environments/staging", `{"enabled":true}`))

	staging := send(t, h, asAdmin(http.MethodPost, "/api/environments/staging/evaluate", evaluateCheckout))
	production := send(t, h, asAdmin(http.MethodPost, "/api/environments/production/evaluate", evaluateCheckout))

	if got := strings.TrimSpace(staging.Body.String()); got != `{"flag":"new-checkout","value":true,"reason":{"kind":"FALLTHROUGH"}}` {
		t.Fatalf("staging = %s", got)
	}
	if got := strings.TrimSpace(production.Body.String()); got != `{"flag":"new-checkout","value":false,"reason":{"kind":"OFF"}}` {
		t.Fatalf("production = %s", got)
	}
}

func TestEvaluateErrors(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	tests := map[string]struct {
		path   string
		body   string
		status int
	}{
		"unknown flag":        {"/api/environments/staging/evaluate", `{"flag":"missing","user":{"key":"u-1"}}`, http.StatusNotFound},
		"unknown environment": {"/api/environments/qa/evaluate", evaluateCheckout, http.StatusNotFound},
		"missing flag":        {"/api/environments/staging/evaluate", `{"user":{"key":"u-1"}}`, http.StatusBadRequest},
		"missing user key":    {"/api/environments/staging/evaluate", `{"flag":"new-checkout","user":{}}`, http.StatusBadRequest},
		"number attribute":    {"/api/environments/staging/evaluate", `{"flag":"new-checkout","user":{"key":"u-1","attributes":{"age":30}}}`, http.StatusBadRequest},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			wantProblem(t, send(t, h, asAdmin(http.MethodPost, tc.path, tc.body)), tc.status)
		})
	}
}

func TestEvaluateNeedsAnAdminKey(t *testing.T) {
	rec := send(t, newTestHandler(t), request{method: http.MethodPost, path: "/api/environments/staging/evaluate", body: evaluateCheckout})
	wantProblem(t, rec, http.StatusUnauthorized)
}
