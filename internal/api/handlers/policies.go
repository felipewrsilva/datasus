package handlers

import (
	"encoding/json"
	"net/http"

	"datasus/internal/repository"
)

// PoliciesHandler serves GET/PUT download policy configuration.
type PoliciesHandler struct {
	repo       *repository.PolicyRepository
	syncRunner interface {
		Trigger(reason, actor string)
	}
}

func NewPoliciesHandler(repo *repository.PolicyRepository, syncRunner interface {
	Trigger(reason, actor string)
}) *PoliciesHandler {
	return &PoliciesHandler{repo: repo, syncRunner: syncRunner}
}

func (h *PoliciesHandler) Get(w http.ResponseWriter, r *http.Request) {
	policy, err := h.repo.GetPolicies(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, policy)
}

// putBody is the shape for PUT /api/policies.
type putBody struct {
	SelectedCatalogs []string                         `json:"selected_catalogs"`
	SelectedStates   []string                         `json:"selected_states"`
	SelectedPeriods  repository.PolicyPeriods         `json:"selected_periods"`
	Processing       repository.ProcessingStages      `json:"processing"`
	Directories      repository.ProcessingDirectories `json:"directories"`
}

func (h *PoliciesHandler) Put(w http.ResponseWriter, r *http.Request) {
	var body putBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, errBadRequest("invalid JSON body"), 400)
		return
	}
	if err := h.repo.ReplacePolicies(r.Context(), repository.GlobalPolicy{
		SelectedCatalogs: body.SelectedCatalogs,
		SelectedStates:   body.SelectedStates,
		SelectedPeriods:  body.SelectedPeriods,
		Processing:       body.Processing,
		Directories:      body.Directories,
	}); err != nil {
		jsonError(w, err, 400)
		return
	}
	if h.syncRunner != nil {
		h.syncRunner.Trigger("policy_saved", "api")
	}
	policy, err := h.repo.GetPolicies(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, policy)
}
