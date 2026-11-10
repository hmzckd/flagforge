package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEveryRequestIsLoggedWithItsStatus(t *testing.T) {
	var logs bytes.Buffer
	handler := NewHandler(slog.New(slog.NewJSONHandler(&logs, nil)))

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var line struct {
		Msg    string `json:"msg"`
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, logs.String())
	}
	if line.Msg != "request" || line.Method != "GET" || line.Path != "/does-not-exist" || line.Status != http.StatusNotFound {
		t.Fatalf("unexpected log line: %+v", line)
	}
}
