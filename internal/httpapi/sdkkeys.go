package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/hmzckd/flagforge/internal/store"
)

type sdkKeyResponse struct {
	ID          int64      `json:"id"`
	Environment string     `json:"environment"`
	Prefix      string     `json:"prefix"`
	Key         string     `json:"key,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

func toSDKKeyResponse(k store.SDKKey) sdkKeyResponse {
	return sdkKeyResponse{ID: k.ID, Environment: k.Environment, Prefix: k.Prefix, CreatedAt: k.CreatedAt, RevokedAt: k.RevokedAt}
}

func (a *api) createSDKKey(w http.ResponseWriter, r *http.Request) {
	key, plain, err := a.store.CreateSDKKey(r.Context(), r.PathValue("env"))
	if err != nil {
		a.storeError(w, r, err)
		return
	}
	a.logger.Info("sdk key created", "environment", key.Environment, "id", key.ID, "actor", actorFrom(r.Context()))

	resp := toSDKKeyResponse(key)
	resp.Key = plain
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, resp)
}

func (a *api) listSDKKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.store.ListSDKKeys(r.Context(), r.PathValue("env"))
	if err != nil {
		a.storeError(w, r, err)
		return
	}
	out := make([]sdkKeyResponse, 0, len(keys))
	for _, k := range keys {
		out = append(out, toSDKKeyResponse(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sdk_keys": out})
}

func (a *api) revokeSDKKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "sdk key not found")
		return
	}
	err = a.store.RevokeSDKKey(r.Context(), r.PathValue("env"), id)
	if errors.Is(err, store.ErrSDKKeyNotFound) {
		writeProblem(w, http.StatusNotFound, "sdk key not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.logger.Info("sdk key revoked", "environment", r.PathValue("env"), "id", id, "actor", actorFrom(r.Context()))
	w.WriteHeader(http.StatusNoContent)
}
