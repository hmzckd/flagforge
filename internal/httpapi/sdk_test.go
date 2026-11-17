package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/hmzckd/flagforge/internal/store"
)

func createSDKKey(t *testing.T, h http.Handler, env string) sdkKeyResponse {
	t.Helper()
	rec := send(t, h, asAdmin(http.MethodPost, "/api/environments/"+env+"/sdk-keys", ""))
	wantStatus(t, rec, http.StatusCreated)
	return decode[sdkKeyResponse](t, rec)
}

type sdkResult struct {
	code int
	etag string
	body string
}

func sdkGet(t *testing.T, h http.Handler, key, ifNoneMatch string) sdkResult {
	t.Helper()
	req := request{method: http.MethodGet, path: "/sdk/v1/flags", token: key}
	if ifNoneMatch != "" {
		req.headers = map[string]string{"If-None-Match": ifNoneMatch}
	}
	rec := send(t, h, req)
	return sdkResult{code: rec.Code, etag: rec.Header().Get("ETag"), body: rec.Body.String()}
}

func TestCreateSDKKeyShowsTheKeyOnlyOnce(t *testing.T) {
	h := newTestHandler(t)

	created := createSDKKey(t, h, "production")

	if !strings.HasPrefix(created.Key, "ffsdk_") || len(created.Key) < 40 {
		t.Fatalf("key = %q, want ffsdk_ followed by a long random part", created.Key)
	}
	if created.Prefix != created.Key[:10] || created.Environment != "production" {
		t.Fatalf("created = %+v", created)
	}

	rec := send(t, h, asAdmin(http.MethodGet, "/api/environments/production/sdk-keys", ""))
	wantStatus(t, rec, http.StatusOK)
	if strings.Contains(rec.Body.String(), created.Key) {
		t.Fatal("listing must never show the full key")
	}
	list := decode[struct {
		SDKKeys []sdkKeyResponse `json:"sdk_keys"`
	}](t, rec)
	if len(list.SDKKeys) != 1 || list.SDKKeys[0].Prefix != created.Prefix {
		t.Fatalf("list = %+v", list)
	}
}

func TestTwoKeysAreNeverTheSame(t *testing.T) {
	h := newTestHandler(t)
	if createSDKKey(t, h, "staging").Key == createSDKKey(t, h, "staging").Key {
		t.Fatal("two keys are equal")
	}
}

func TestSDKKeyForUnknownEnvironment(t *testing.T) {
	h := newTestHandler(t)
	wantProblem(t, send(t, h, asAdmin(http.MethodPost, "/api/environments/qa/sdk-keys", "")), http.StatusNotFound)
	wantProblem(t, send(t, h, asAdmin(http.MethodGet, "/api/environments/qa/sdk-keys", "")), http.StatusNotFound)
}

func TestSnapshotContainsTheEnvironmentsSettings(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	putSetting(t, h, "/api/flags/new-checkout/environments/staging", turkeyOnly)
	stagingKey := createSDKKey(t, h, "staging").Key
	productionKey := createSDKKey(t, h, "production").Key

	staging := sdkGet(t, h, stagingKey, "")
	production := sdkGet(t, h, productionKey, "")

	if staging.code != http.StatusOK || production.code != http.StatusOK {
		t.Fatalf("status = %d and %d", staging.code, production.code)
	}
	var stagingSnap, productionSnap store.Snapshot
	decodeInto(t, staging.body, &stagingSnap)
	decodeInto(t, production.body, &productionSnap)

	if stagingSnap.Environment != "staging" || !stagingSnap.Flags["new-checkout"].Enabled || len(stagingSnap.Flags["new-checkout"].Rules) != 1 {
		t.Fatalf("staging snapshot = %+v", stagingSnap)
	}
	if productionSnap.Environment != "production" || productionSnap.Flags["new-checkout"].Enabled {
		t.Fatalf("production snapshot = %+v", productionSnap)
	}
	if staging.etag != `"`+strconv.FormatInt(stagingSnap.Version, 10)+`"` {
		t.Fatalf("ETag = %s, version = %d", staging.etag, stagingSnap.Version)
	}
}

func TestSnapshotNotModified(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	key := createSDKKey(t, h, "production").Key

	first := sdkGet(t, h, key, "")
	second := sdkGet(t, h, key, first.etag)

	if second.code != http.StatusNotModified || second.body != "" || second.etag != first.etag {
		t.Fatalf("second = %d %q etag %s, want 304 with no body and the same ETag", second.code, second.body, second.etag)
	}

	putSetting(t, h, "/api/flags/new-checkout/environments/production", `{"enabled":true}`)
	third := sdkGet(t, h, key, first.etag)

	if third.code != http.StatusOK || third.etag == first.etag {
		t.Fatalf("after a change: %d with ETag %s, want 200 with a new ETag", third.code, third.etag)
	}
}

func TestChangingOneEnvironmentLeavesTheOtherVersionAlone(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	stagingKey := createSDKKey(t, h, "staging").Key
	before := sdkGet(t, h, stagingKey, "")

	putSetting(t, h, "/api/flags/new-checkout/environments/production", `{"enabled":true}`)

	if got := sdkGet(t, h, stagingKey, before.etag); got.code != http.StatusNotModified {
		t.Fatalf("staging = %d, want 304 after a production-only change", got.code)
	}
}

func TestCreatingAFlagChangesEveryEnvironmentVersion(t *testing.T) {
	h := newTestHandler(t)
	stagingKey := createSDKKey(t, h, "staging").Key
	before := sdkGet(t, h, stagingKey, "")

	createCheckoutFlag(t, h)

	if got := sdkGet(t, h, stagingKey, before.etag); got.code != http.StatusOK || !strings.Contains(got.body, "new-checkout") {
		t.Fatalf("staging = %d %s, want 200 with the new flag", got.code, got.body)
	}
}

func TestSDKEndpointRejectsBadKeys(t *testing.T) {
	h := newTestHandler(t)
	created := createSDKKey(t, h, "production")
	wantStatus(t, send(t, h, asAdmin(http.MethodDelete, "/api/environments/production/sdk-keys/"+strconv.FormatInt(created.ID, 10), "")), http.StatusNoContent)

	for name, token := range map[string]string{
		"no key":        "",
		"unknown key":   "ffsdk_not-a-real-key-0000000000000000000000000",
		"admin key":     testAdminKey,
		"revoked key":   created.Key,
		"prefix of key": created.Prefix,
	} {
		t.Run(name, func(t *testing.T) {
			rec := send(t, h, request{method: http.MethodGet, path: "/sdk/v1/flags", token: token})
			wantProblem(t, rec, http.StatusUnauthorized)
		})
	}
}

func TestRevokeSDKKey(t *testing.T) {
	h := newTestHandler(t)
	created := createSDKKey(t, h, "production")
	path := "/api/environments/production/sdk-keys/" + strconv.FormatInt(created.ID, 10)

	wantStatus(t, send(t, h, asAdmin(http.MethodDelete, path, "")), http.StatusNoContent)
	wantStatus(t, send(t, h, asAdmin(http.MethodDelete, path, "")), http.StatusNoContent)

	list := decode[struct {
		SDKKeys []sdkKeyResponse `json:"sdk_keys"`
	}](t, send(t, h, asAdmin(http.MethodGet, "/api/environments/production/sdk-keys", "")))
	if list.SDKKeys[0].RevokedAt == nil {
		t.Fatal("revoked_at is empty")
	}

	wantProblem(t, send(t, h, asAdmin(http.MethodDelete, "/api/environments/staging/sdk-keys/"+strconv.FormatInt(created.ID, 10), "")), http.StatusNotFound)
	wantProblem(t, send(t, h, asAdmin(http.MethodDelete, "/api/environments/production/sdk-keys/999999", "")), http.StatusNotFound)
	wantProblem(t, send(t, h, asAdmin(http.MethodDelete, "/api/environments/production/sdk-keys/abc", "")), http.StatusNotFound)
}

func TestETagMatches(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{`"7"`, true},
		{`W/"7"`, true},
		{`"6", "7"`, true},
		{`*`, true},
		{`"6"`, false},
		{``, false},
		{`7`, false},
	}
	for _, tc := range tests {
		if got := etagMatches(tc.header, `"7"`); got != tc.want {
			t.Errorf("etagMatches(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}
