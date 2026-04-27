package handlers

import (
	"encoding/json"
	"net/http"

	"datasus/internal/repository"
)

// PoliciesHandler serves GET/PUT download policy configuration.
type PoliciesHandler struct {
	repo *repository.PolicyRepository
}

func NewPoliciesHandler(repo *repository.PolicyRepository) *PoliciesHandler {
	return &PoliciesHandler{repo: repo}
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
	SelectedCatalogs []string                    `json:"selected_catalogs"`
	SelectedPeriods  repository.PolicyPeriods    `json:"selected_periods"`
	Processing       repository.ProcessingStages `json:"processing"`
}

func (h *PoliciesHandler) Put(w http.ResponseWriter, r *http.Request) {
	var body putBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, errBadRequest("invalid JSON body"), 400)
		return
	}
	if err := h.repo.ReplacePolicies(r.Context(), repository.GlobalPolicy{
		SelectedCatalogs: body.SelectedCatalogs,
		SelectedPeriods:  body.SelectedPeriods,
		Processing:       body.Processing,
	}); err != nil {
		jsonError(w, err, 400)
		return
	}
	policy, err := h.repo.GetPolicies(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, policy)
}
