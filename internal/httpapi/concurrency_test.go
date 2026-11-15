package httpapi

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
)

const stagingPath = "/api/flags/new-checkout/environments/staging"

func putWithIfMatch(ifMatch, body string) request {
	req := asAdmin(http.MethodPut, stagingPath, body)
	if ifMatch != "" {
		req.headers = map[string]string{"If-Match": ifMatch}
	}
	return req
}

func TestGetSettingReturnsAnETag(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	rec := send(t, h, asAdmin(http.MethodGet, stagingPath, ""))

	wantStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("ETag"); got != `"1"` {
		t.Fatalf("ETag = %s, want \"1\"", got)
	}
	if got := decode[settingResponse](t, rec).Version; got != 1 {
		t.Fatalf("version = %d, want 1", got)
	}
}

func TestPutNeedsIfMatch(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	for name, ifMatch := range map[string]string{
		"missing":   "",
		"not a tag": "1",
		"star":      "*",
		"two tags":  `"1", "2"`,
	} {
		t.Run(name, func(t *testing.T) {
			wantProblem(t, send(t, h, putWithIfMatch(ifMatch, `{"enabled":true}`)), http.StatusPreconditionRequired)
		})
	}
}

func TestPutWithCurrentETagSucceedsAndReturnsTheNextOne(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	rec := send(t, h, putWithIfMatch(`"1"`, `{"enabled":true}`))

	wantStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("ETag"); got != `"2"` {
		t.Fatalf("ETag = %s, want \"2\"", got)
	}
}

func TestPutWithOldETagIsRejected(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	wantStatus(t, send(t, h, putWithIfMatch(`"1"`, `{"enabled":true}`)), http.StatusOK)

	rec := send(t, h, putWithIfMatch(`"1"`, `{"enabled":false}`))

	wantProblem(t, rec, http.StatusPreconditionFailed)
	if got := decode[settingResponse](t, send(t, h, asAdmin(http.MethodGet, stagingPath, ""))); !got.Enabled || got.Version != 2 {
		t.Fatalf("setting = %+v, want the first change (enabled, version 2) to survive", got)
	}
}

func TestTwoAdminsChangingTheSameVersionAtOnce(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)

	const admins = 10
	statuses := make([]int, admins)
	var wg sync.WaitGroup
	for i := range admins {
		wg.Go(func() {
			body := `{"enabled":true}`
			if i%2 == 1 {
				body = `{"enabled":false}`
			}
			statuses[i] = send(t, h, putWithIfMatch(`"1"`, body)).Code
		})
	}
	wg.Wait()

	ok, failed := 0, 0
	for _, s := range statuses {
		switch s {
		case http.StatusOK:
			ok++
		case http.StatusPreconditionFailed:
			failed++
		default:
			t.Fatalf("unexpected status %d", s)
		}
	}
	if ok != 1 || failed != admins-1 {
		t.Fatalf("%d succeeded and %d got 412, want exactly 1 and %d", ok, failed, admins-1)
	}

	entries := auditEntries(t, h)
	updates := 0
	for _, e := range entries {
		if e.Action == "flag.setting_updated" {
			updates++
		}
	}
	if updates != 1 {
		t.Fatalf("audit has %d setting updates, want 1", updates)
	}
}

func auditEntries(t *testing.T, h http.Handler) []auditResponse {
	t.Helper()
	rec := send(t, h, asAdmin(http.MethodGet, "/api/flags/new-checkout/audit", ""))
	wantStatus(t, rec, http.StatusOK)
	return decode[struct{ Entries []auditResponse }](t, rec).Entries
}

func TestAuditRecordsWhoChangedWhat(t *testing.T) {
	h := newTestHandler(t)
	createCheckoutFlag(t, h)
	putSetting(t, h, "/api/flags/new-checkout/environments/production", `{"enabled":true}`)

	entries := auditEntries(t, h)

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	update, created := entries[0], entries[1]

	if update.Action != "flag.setting_updated" || update.Environment != "production" || update.Actor != "tester" {
		t.Fatalf("newest entry = %+v", update)
	}
	var before, after struct{ Enabled bool }
	if err := json.Unmarshal(update.Before, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(update.After, &after); err != nil {
		t.Fatal(err)
	}
	if before.Enabled || !after.Enabled {
		t.Fatalf("before = %s, after = %s", update.Before, update.After)
	}

	if created.Action != "flag.created" || created.Environment != "" || created.Actor != "tester" || string(created.Before) != "null" {
		t.Fatalf("oldest entry = %+v", created)
	}
	var flag struct{ Key, Name string }
	if err := json.Unmarshal(created.After, &flag); err != nil || flag.Key != "new-checkout" || flag.Name != "New checkout" {
		t.Fatalf("created.after = %s", created.After)
	}
}

func TestAuditOfUnknownFlagIsNotFound(t *testing.T) {
	wantProblem(t, send(t, newTestHandler(t), asAdmin(http.MethodGet, "/api/flags/missing/audit", "")), http.StatusNotFound)
}
