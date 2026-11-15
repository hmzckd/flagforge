package httpapi

import (
	"encoding/json"
	"net/http"
	"time"
)

type auditResponse struct {
	ID          int64           `json:"id"`
	Action      string          `json:"action"`
	Environment string          `json:"environment,omitempty"`
	Actor       string          `json:"actor"`
	Before      json.RawMessage `json:"before"`
	After       json.RawMessage `json:"after"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (a *api) listAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := a.store.ListAudit(r.Context(), r.PathValue("key"))
	if err != nil {
		a.storeError(w, r, err)
		return
	}
	out := make([]auditResponse, 0, len(entries))
	for _, e := range entries {
		before := e.Before
		if before == nil {
			before = json.RawMessage("null")
		}
		out = append(out, auditResponse{
			ID:          e.ID,
			Action:      e.Action,
			Environment: e.Environment,
			Actor:       e.Actor,
			Before:      before,
			After:       e.After,
			CreatedAt:   e.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}
