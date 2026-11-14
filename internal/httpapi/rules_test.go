package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/hmzckd/flagforge/eval"
)

const turkeyOnly = `{
	"enabled": true,
	"rules": [
		{"clauses": [{"attribute": "country", "op": "in", "values": ["TR"]}], "serve": {"value": true}}
	],
	"fallthrough": {"value": false}
}`

func evaluateUser(t *testing.T, h http.Handler, env, user string) evaluateResponse {
	t.Helper()
	rec := send(t, h, asAdmin(http.MethodPost, "/api/environments/"+env+"/evaluate", `{"flag":"new-checkout","user":`+user+`}`))
	wantStatus(t, rec, http.StatusOK)
	return decode[evaluateResponse](t, rec)
}

func TestTargetingRuleServesTrueOnlyToMatchingUsers(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	wantStatus(t, send(t, h, asAdmin(http.MethodPut, "/api/flags/new-checkout/environments/production", turkeyOnly)), http.StatusOK)

	tr := evaluateUser(t, h, "production", `{"key":"u-1","attributes":{"country":"TR"}}`)
	de := evaluateUser(t, h, "production", `{"key":"u-2","attributes":{"country":"DE"}}`)

	if !tr.Value || tr.Reason != (eval.Reason{Kind: eval.ReasonRuleMatch, RuleIndex: 0}) {
		t.Fatalf("TR user = %+v, want true by rule 0", tr)
	}
	if de.Value || de.Reason.Kind != eval.ReasonFallthrough {
		t.Fatalf("DE user = %+v, want false by fallthrough", de)
	}
}

func TestRulesAreStoredAndReturned(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	send(t, h, asAdmin(http.MethodPut, "/api/flags/new-checkout/environments/production", turkeyOnly))

	got := decode[settingResponse](t, send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout/environments/production", "")))

	if len(got.Rules) != 1 || got.Rules[0].Clauses[0].Values[0] != "TR" || got.Fallthrough.Value == nil || *got.Fallthrough.Value {
		t.Fatalf("setting = %+v", got)
	}
}

func TestPutWithoutRulesClearsThemAndFallsThroughToTrue(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	path := "/api/flags/new-checkout/environments/production"
	send(t, h, asAdmin(http.MethodPut, path, turkeyOnly))

	rec := send(t, h, asAdmin(http.MethodPut, path, `{"enabled":true}`))

	wantStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	if !strings.Contains(body, `"rules":[]`) || !strings.Contains(body, `"fallthrough":{"value":true}`) {
		t.Fatalf("body = %s", body)
	}
	if got := evaluateUser(t, h, "production", `{"key":"u-2","attributes":{"country":"DE"}}`); !got.Value {
		t.Fatalf("DE user = %+v, want true", got)
	}
}

func TestNewFlagHasNoRulesAndFallsThroughToTrue(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	body := send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout/environments/staging", "")).Body.String()

	if !strings.Contains(body, `"rules":[]`) || !strings.Contains(body, `"fallthrough":{"value":true}`) {
		t.Fatalf("body = %s", body)
	}
}

func TestPutRejectsInvalidRules(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	tests := map[string]struct {
		body string
		want string
	}{
		"unknown operator": {
			`{"enabled":true,"rules":[{"clauses":[{"attribute":"country","op":"equals","values":["TR"]}],"serve":{"value":true}}]}`,
			`rules[0].clauses[0].op: unknown operator "equals"`,
		},
		"rule without serve": {
			`{"enabled":true,"rules":[{"clauses":[{"attribute":"country","op":"in","values":["TR"]}]}]}`,
			"rules[0].serve: value is required",
		},
		"empty fallthrough": {
			`{"enabled":true,"fallthrough":{}}`,
			"fallthrough: value is required",
		},
		"unknown field inside a clause": {
			`{"enabled":true,"rules":[{"clauses":[{"attribute":"country","op":"in","values":["TR"],"negate":true}],"serve":{"value":true}}]}`,
			"negate",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := send(t, h, asAdmin(http.MethodPut, "/api/flags/new-checkout/environments/production", tc.body))
			p := wantProblem(t, rec, http.StatusBadRequest)
			if !strings.Contains(p.Detail, tc.want) {
				t.Fatalf("detail = %q, want it to contain %q", p.Detail, tc.want)
			}
		})
	}
}
