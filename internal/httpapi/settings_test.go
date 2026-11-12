package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func createCheckoutFlag(t *testing.T, h http.Handler) {
	t.Helper()
	wantStatus(t, send(t, h, asAdmin(http.MethodPost, "/api/flags", checkoutFlag)), http.StatusCreated)
}

func TestNewFlagIsOffInEveryEnvironment(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	body := decode[flagResponse](t, send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout", "")))

	if len(body.Environments) != 2 || body.Environments["staging"].Enabled || body.Environments["production"].Enabled {
		t.Fatalf("environments = %+v, want staging and production both off", body.Environments)
	}
}

func TestTurningAFlagOnInStagingLeavesProductionOff(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	rec := send(t, h, asAdmin(http.MethodPut, "/api/flags/new-checkout/environments/staging", `{"enabled":true}`))
	wantStatus(t, rec, http.StatusOK)
	if got := decode[settingResponse](t, rec); !got.Enabled || got.Environment != "staging" || got.Flag != "new-checkout" {
		t.Fatalf("PUT response = %+v", got)
	}

	staging := decode[settingResponse](t, send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout/environments/staging", "")))
	production := decode[settingResponse](t, send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout/environments/production", "")))
	if !staging.Enabled || production.Enabled {
		t.Fatalf("staging = %v, production = %v, want true and false", staging.Enabled, production.Enabled)
	}

	flag := decode[flagResponse](t, send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout", "")))
	if !flag.Environments["staging"].Enabled || flag.Environments["production"].Enabled {
		t.Fatalf("flag environments = %+v", flag.Environments)
	}
}

func TestTurningAFlagOffAgain(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	path := "/api/flags/new-checkout/environments/production"
	send(t, h, asAdmin(http.MethodPut, path, `{"enabled":true}`))

	rec := send(t, h, asAdmin(http.MethodPut, path, `{"enabled":false}`))

	wantStatus(t, rec, http.StatusOK)
	if decode[settingResponse](t, rec).Enabled {
		t.Fatal("flag is still on")
	}
}

func TestSettingNotFound(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	tests := map[string]struct {
		path string
		want string
	}{
		"unknown environment": {"/api/flags/new-checkout/environments/qa", `environment "qa"`},
		"unknown flag":        {"/api/flags/missing/environments/staging", `flag "missing"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPut} {
				rec := send(t, h, asAdmin(method, tc.path, `{"enabled":true}`))
				p := wantProblem(t, rec, http.StatusNotFound)
				if !strings.Contains(p.Detail, tc.want) {
					t.Fatalf("%s detail = %q, want it to mention %s", method, p.Detail, tc.want)
				}
			}
		})
	}
}

func TestPutSettingRejectsInvalidInput(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	for name, body := range map[string]string{
		"missing enabled": `{}`,
		"enabled as text": `{"enabled":"yes"}`,
		"unknown field":   `{"enabled":true,"enabeld":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := send(t, h, asAdmin(http.MethodPut, "/api/flags/new-checkout/environments/staging", body))
			wantProblem(t, rec, http.StatusBadRequest)
		})
	}
}

func TestPutSettingNeedsAnAdminKey(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	rec := send(t, h, request{method: http.MethodPut, path: "/api/flags/new-checkout/environments/staging", body: `{"enabled":true}`})
	wantProblem(t, rec, http.StatusUnauthorized)
}

func TestListEnvironments(t *testing.T) {
	rec := send(t, newTestHandler(t), asAdmin(http.MethodGet, "/api/environments", ""))

	wantStatus(t, rec, http.StatusOK)
	got := strings.TrimSpace(rec.Body.String())
	want := `{"environments":[{"key":"staging","name":"Staging"},{"key":"production","name":"Production"}]}`
	if got != want {
		t.Fatalf("body = %s\nwant   %s", got, want)
	}
}
