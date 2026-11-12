package httpapi

import (
	"errors"
	"net/http"

	"github.com/hmzckd/flagforge/eval"
	"github.com/hmzckd/flagforge/internal/store"
)

type evaluateRequest struct {
	Flag string    `json:"flag"`
	User eval.User `json:"user"`
}

type evaluateResponse struct {
	Flag   string      `json:"flag"`
	Value  bool        `json:"value"`
	Reason eval.Reason `json:"reason"`
}

func (req evaluateRequest) validate() error {
	switch {
	case req.Flag == "":
		return errors.New("flag is required")
	case req.User.Key == "":
		return errors.New("user.key is required")
	}
	return nil
}

func (a *api) evaluate(w http.ResponseWriter, r *http.Request) {
	var req evaluateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	setting, err := a.store.GetFlagSetting(r.Context(), req.Flag, r.PathValue("env"))
	if errors.Is(err, store.ErrFlagNotFound) {
		writeProblem(w, http.StatusNotFound, "flag \""+req.Flag+"\" not found")
		return
	}
	if err != nil {
		a.storeError(w, r, err)
		return
	}

	result := eval.Evaluate(setting.EvalFlag(), req.User)
	writeJSON(w, http.StatusOK, evaluateResponse{Flag: req.Flag, Value: result.Value, Reason: result.Reason})
}
