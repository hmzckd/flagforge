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
	mux.Handle("GET /api/environments", admin(http.HandlerFunc(a.listEnvironments)))
	mux.Handle("POST /api/environments/{env}/evaluate", admin(http.HandlerFunc(a.evaluate)))

	return logRequests(deps.Logger, mux)
}
