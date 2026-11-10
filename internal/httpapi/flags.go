package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/hmzckd/flagforge/internal/store"
)

var flagKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type createFlagRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type flagResponse struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

func toFlagResponse(f store.Flag) flagResponse {
	return flagResponse{Key: f.Key, Name: f.Name, Description: f.Description, CreatedAt: f.CreatedAt}
}

func (req createFlagRequest) validate() error {
	switch {
	case !flagKeyPattern.MatchString(req.Key):
		return errors.New("key must be 1-64 characters of lowercase letters, digits and '-', starting with a letter or digit")
	case req.Name == "" || utf8.RuneCountInString(req.Name) > 100:
		return errors.New("name is required and must be at most 100 characters")
	case utf8.RuneCountInString(req.Description) > 500:
		return errors.New("description must be at most 500 characters")
	}
	return nil
}

func (a *api) createFlag(w http.ResponseWriter, r *http.Request) {
	var req createFlagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	flag, err := a.store.CreateFlag(r.Context(), store.NewFlag{Key: req.Key, Name: req.Name, Description: req.Description})
	if errors.Is(err, store.ErrAlreadyExists) {
		writeProblem(w, http.StatusConflict, fmt.Sprintf("flag %q already exists", req.Key))
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/flags/"+flag.Key)
	writeJSON(w, http.StatusCreated, toFlagResponse(flag))
}

func (a *api) listFlags(w http.ResponseWriter, r *http.Request) {
	flags, err := a.store.ListFlags(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	out := make([]flagResponse, 0, len(flags))
	for _, f := range flags {
		out = append(out, toFlagResponse(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"flags": out})
}

func (a *api) getFlag(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	flag, err := a.store.GetFlag(r.Context(), key)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, fmt.Sprintf("flag %q not found", key))
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFlagResponse(flag))
}
