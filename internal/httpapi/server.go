package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/hmzckd/flagforge/internal/store"
	"github.com/hmzckd/flagforge/internal/stream"
)

type Deps struct {
	Logger    *slog.Logger
	Store     *store.Store
	Hub       *stream.Hub
	AdminKeys map[string]string

	HeartbeatInterval  time.Duration
	StreamWriteTimeout time.Duration
}

type api struct {
	logger *slog.Logger
	store  *store.Store
	hub    *stream.Hub

	heartbeatInterval  time.Duration
	streamWriteTimeout time.Duration
}

func NewHandler(deps Deps) http.Handler {
	a := &api{
		logger:             deps.Logger,
		store:              deps.Store,
		hub:                deps.Hub,
		heartbeatInterval:  deps.HeartbeatInterval,
		streamWriteTimeout: deps.StreamWriteTimeout,
	}
	if a.heartbeatInterval == 0 {
		a.heartbeatInterval = 15 * time.Second
	}
	if a.streamWriteTimeout == 0 {
		a.streamWriteTimeout = 10 * time.Second
	}
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
	mux.Handle("GET /sdk/v1/stream", a.requireSDKKey(http.HandlerFunc(a.sdkStream)))

	return logRequests(deps.Logger, mux)
}
