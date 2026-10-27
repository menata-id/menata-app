package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/builder"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/installer"
)

// maxBuilderBody bounds a request of operations; a list of edits is a few hundred bytes.
const maxBuilderBody = 64 << 10

// builderPlanView is the answer to a list of operations: what they would do (PlanUpdate's own lines), what the policy
// refuses, and the files the writer would produce (the inspector's Code tab). Nothing has been written.
type builderPlanView struct {
	Items    []builderPlanItem `json:"items"`
	Refusals []string          `json:"refusals,omitempty"`
	Files    []builderFile     `json:"files,omitempty"`
	Error    string            `json:"error,omitempty"`
}

type builderPlanItem struct {
	Op      string `json:"op"`
	What    string `json:"what"`
	Confirm string `json:"confirm,omitempty"`
}

type builderFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Diff   string `json:"diff"`
}

// planBuilderOperations reviews a list of Apps Builder operations against one installed Application of the current
// Workspace (Fase 1 of menata-app-document audits/2026-10-27-kajian-apps-builder.md). It is a POST because the list is a
// body, not because it writes: it never does. Applying a plan is the one write path, not this route.
func planBuilderOperations(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Ops []builder.Op `json:"ops"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxBuilderBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil || len(body.Ops) == 0 {
			http.Error(w, `expected {"ops": [...]} with at least one operation`, http.StatusBadRequest)
			return
		}
		manifestPath, _, err := workspaceInstallation(req.Context(), store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		plan, err := builder.Review(manifestPath, chi.URLParam(req, "app"), body.Ops)
		view := builderPlanViewOf(plan)
		var rejected *installer.RejectedError
		switch {
		case errors.As(err, &rejected):
			view.Error = rejected.Error()
			w.WriteHeader(http.StatusUnprocessableEntity)
		case err != nil:
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, view)
	}
}

func builderPlanViewOf(plan builder.Plan) builderPlanView {
	view := builderPlanView{Items: []builderPlanItem{}, Refusals: plan.Plan.Refusals}
	for _, it := range plan.Plan.Items {
		view.Items = append(view.Items, builderPlanItem{Op: it.Op, What: it.What, Confirm: it.Confirm})
	}
	for _, f := range plan.Files {
		view.Files = append(view.Files, builderFile{Path: f.Path, Status: f.Status, Diff: f.Diff})
	}
	return view
}
