package httpapi

import (
	"log/slog"
	"net/http"
)

func NewHandler(logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	return logRequests(logger, mux)
}
