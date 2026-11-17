package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hmzckd/flagforge/internal/store"
)

type sdkEnvironmentKey struct{}

func sdkEnvironment(ctx context.Context) string {
	env, _ := ctx.Value(sdkEnvironmentKey{}).(string)
	return env
}

func (a *api) requireSDKKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			unauthorized(w, "missing bearer token")
			return
		}
		env, err := a.store.EnvironmentForSDKKey(r.Context(), token)
		if errors.Is(err, store.ErrSDKKeyNotFound) {
			unauthorized(w, "invalid or revoked sdk key")
			return
		}
		if err != nil {
			a.internalError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sdkEnvironmentKey{}, env)))
	})
}

func (a *api) sdkFlags(w http.ResponseWriter, r *http.Request) {
	env := sdkEnvironment(r.Context())

	version, err := a.store.EnvironmentVersion(r.Context(), env)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if etagMatches(r.Header.Get("If-None-Match"), etag(version)) {
		w.Header().Set("ETag", etag(version))
		w.WriteHeader(http.StatusNotModified)
		return
	}

	snap, err := a.store.Snapshot(r.Context(), env)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(snap.Version))
	writeJSON(w, http.StatusOK, snap)
}

func etagMatches(ifNoneMatch, current string) bool {
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == current {
			return true
		}
	}
	return false
}
