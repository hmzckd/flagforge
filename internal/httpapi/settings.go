package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/hmzckd/flagforge/internal/store"
)

type settingResponse struct {
	Flag        string    `json:"flag"`
	Environment string    `json:"environment"`
	Enabled     bool      `json:"enabled"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toSettingResponse(s store.FlagSetting) settingResponse {
	return settingResponse{Flag: s.FlagKey, Environment: s.Environment, Enabled: s.Enabled, UpdatedAt: s.UpdatedAt}
}

type putSettingRequest struct {
	Enabled *bool `json:"enabled"`
}

func (req putSettingRequest) validate() error {
	if req.Enabled == nil {
		return errors.New("enabled is required")
	}
	return nil
}

func (a *api) getSetting(w http.ResponseWriter, r *http.Request) {
	setting, err := a.store.GetFlagSetting(r.Context(), r.PathValue("key"), r.PathValue("env"))
	if err != nil {
		a.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSettingResponse(setting))
}

func (a *api) putSetting(w http.ResponseWriter, r *http.Request) {
	var req putSettingRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	setting, err := a.store.UpdateFlagSetting(r.Context(), r.PathValue("key"), r.PathValue("env"),
		store.SettingChange{Enabled: *req.Enabled})
	if err != nil {
		a.storeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toSettingResponse(setting))
}

func (a *api) listEnvironments(w http.ResponseWriter, r *http.Request) {
	envs, err := a.store.ListEnvironments(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	type environmentResponse struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	out := make([]environmentResponse, 0, len(envs))
	for _, e := range envs {
		out = append(out, environmentResponse{Key: e.Key, Name: e.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": out})
}
