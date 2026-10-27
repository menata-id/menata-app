package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	Version  string            `json:"version,omitempty"`
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

// builderRequest is the body of both builder routes: the operations, and for an apply the version the plan was made
// against and the removals the owner ticked.
type builderRequest struct {
	Ops     []builder.Op `json:"ops"`
	Version string       `json:"version,omitempty"`
	Confirm []string     `json:"confirm,omitempty"`
}

func readBuilderRequest(w http.ResponseWriter, req *http.Request) (builderRequest, bool) {
	var body builderRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxBuilderBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || len(body.Ops) == 0 {
		http.Error(w, `expected {"ops": [...]} with at least one operation`, http.StatusBadRequest)
		return body, false
	}
	return body, true
}

// planBuilderOperations reviews a list of Apps Builder operations against one installed Application of the current
// Workspace (Fase 1 of menata-app-document audits/2026-10-27-kajian-apps-builder.md). It is a POST because the list is a
// body, not because it writes: it never does. Applying a plan is the one write path, not this route.
func planBuilderOperations(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, ok := readBuilderRequest(w, req)
		if !ok {
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
	view := builderPlanView{Items: []builderPlanItem{}, Refusals: plan.Plan.Refusals, Version: plan.Version}
	for _, it := range plan.Plan.Items {
		view.Items = append(view.Items, builderPlanItem{Op: it.Op, What: it.What, Confirm: it.Confirm})
	}
	for _, f := range plan.Files {
		view.Files = append(view.Files, builderFile{Path: f.Path, Status: f.Status, Diff: f.Diff})
	}
	return view
}

// applyBuilderOperations writes what planBuilderOperations showed -- the one write: builder.Commit, i.e. the assistant's own
// writer -- after a snapshot, and reloads the Workspace. It refuses a plan made against an installation that has since changed
// (409) and a removal the owner did not tick (422), exactly as publishing a conversation does.
func applyBuilderOperations(store *data.Store, cfg config.Config, reload func(slug string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, ok := readBuilderRequest(w, req)
		if !ok {
			return
		}
		if body.Version == "" {
			http.Error(w, "version is required: apply the plan you reviewed", http.StatusBadRequest)
			return
		}
		ctx, appID := req.Context(), chi.URLParam(req, "app")
		manifestPath, _, err := workspaceInstallation(ctx, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		snapshotBeforeWrite(cfg, manifestPath)
		err = builder.Commit(manifestPath, appID, body.Ops, body.Version, body.Confirm)
		var rejected *installer.RejectedError
		switch {
		case errors.Is(err, builder.ErrStale):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case errors.As(err, &rejected):
			http.Error(w, rejected.Error(), http.StatusUnprocessableEntity)
			return
		case err != nil:
			serverError(w, err)
			return
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		log.Printf("BUILDER-APPLY app=%s workspace=%s ops=%d", appID, workspaceID, len(body.Ops))
		if reload == nil {
			serverError(w, errors.New("builder: written, but this process has no reload hook -- a restart will pick it up"))
			return
		}
		if err := reloadCurrentWorkspace(ctx, store, reload); err != nil {
			serverError(w, fmt.Errorf("builder: written, but the live reload failed -- a restart will pick it up: %w", err))
			return
		}
		version, _ := builder.Version(manifestPath)
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]any{"applied": true, "version": version})
	}
}
