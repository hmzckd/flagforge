package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type actorKey struct{}

func actorFrom(ctx context.Context) string {
	name, _ := ctx.Value(actorKey{}).(string)
	return name
}

func requireAdmin(keys map[string]string) func(http.Handler) http.Handler {
	hashed := make(map[string][32]byte, len(keys))
	for name, key := range keys {
		hashed[name] = sha256.Sum256([]byte(key))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				unauthorized(w, "missing bearer token")
				return
			}
			given := sha256.Sum256([]byte(token))

			actor := ""
			for name, want := range hashed {
				if subtle.ConstantTimeCompare(given[:], want[:]) == 1 {
					actor = name
				}
			}
			if actor == "" {
				unauthorized(w, "invalid admin key")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func unauthorized(w http.ResponseWriter, detail string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="flagforge"`)
	writeProblem(w, http.StatusUnauthorized, detail)
}
