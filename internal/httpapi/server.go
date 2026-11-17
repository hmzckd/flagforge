package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/hmzckd/flagforge/internal/store"
)

type Deps struct {
	Logger    *slog.Logger
	Store     *store.Store
	AdminKeys map[string]string
}

type api struct {
	logger *slog.Logger
	store  *store.Store
}

func NewHandler(deps Deps) http.Handler {
	a := &api{logger: deps.Logger, store: deps.Store}
	admin := requireAdmin(deps.AdminKeys)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /ready", a.handleReady)

	mux.Handle("POST /api/flags", admin(http.HandlerFunc(a.createFlag)))
	mux.Handle("GET /api/flags", admin(http.HandlerFunc(a.listFlags)))
	mux.Handle("GET /api/flags/{key}", admin(http.HandlerFunc(a.getFlag)))
	mux.Handle("GET /api/flags/{key}/environments/{env}", admin(http.HandlerFunc(a.getSetting)))
	mux.Handle("PUT /api/flags/{key}/environments/{env}", admin(http.HandlerFunc(a.putSetting)))
	mux.Handle("GET /api/flags/{key}/audit", admin(http.HandlerFunc(a.listAudit)))
	mux.Handle("GET /api/environments", admin(http.HandlerFunc(a.listEnvironments)))
	mux.Handle("POST /api/environments/{env}/evaluate", admin(http.HandlerFunc(a.evaluate)))
	mux.Handle("POST /api/environments/{env}/sdk-keys", admin(http.HandlerFunc(a.createSDKKey)))
	mux.Handle("GET /api/environments/{env}/sdk-keys", admin(http.HandlerFunc(a.listSDKKeys)))
	mux.Handle("DELETE /api/environments/{env}/sdk-keys/{id}", admin(http.HandlerFunc(a.revokeSDKKey)))

	mux.Handle("GET /sdk/v1/flags", a.requireSDKKey(http.HandlerFunc(a.sdkFlags)))

	return logRequests(deps.Logger, mux)
}
