package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

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

// runEditField is the Edit tab of the builder for a Field: the record is the part the tree names ("<app>/machines/<machine>/fields/<field>"),
// and the form says the Field's new name and whether it is required. It makes the same two operations a conversation would
// (rename_field, set_required) and writes them through builder.Commit -- the one write path, which load-verifies and rolls back -- after a
// snapshot, then reloads the Workspace and returns to the part, so the person sees it renamed in the tree and in the preview.
func runEditField(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	appID, rest, _ := strings.Cut(record, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 4 || parts[0] != "machines" || parts[2] != "fields" || appID == "" {
		http.Error(w, "this part is not a Field", http.StatusBadRequest)
		return "", false
	}
	name := strings.TrimSpace(req.PostForm.Get("name"))
	if name == "" {
		http.Error(w, "a Field needs a name", http.StatusBadRequest)
		return "", false
	}
	if env.reload == nil {
		serverError(w, errors.New("builder: this process has no reload hook configured"))
		return "", false
	}
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	version, err := builder.Version(manifestPath)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	ops := []builder.Op{
		{Kind: builder.OpRenameField, Machine: parts[1], Field: parts[3], Value: name},
		{Kind: builder.OpSetRequired, Machine: parts[1], Field: parts[3], Flag: req.PostForm.Get("required") != ""},
	}
	snapshotBeforeWrite(env.cfg, manifestPath)
	err = builder.Commit(manifestPath, appID, ops, version, nil)
	var rejected *installer.RejectedError
	switch {
	case errors.Is(err, builder.ErrStale):
		http.Error(w, err.Error(), http.StatusConflict)
		return "", false
	case errors.As(err, &rejected):
		http.Error(w, rejected.Error(), http.StatusUnprocessableEntity)
		return "", false
	case err != nil:
		serverError(w, err)
		return "", false
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	log.Printf("BUILDER-APPLY app=%s workspace=%s ops=%d", appID, workspaceID, len(ops))
	if err := reloadCurrentWorkspace(ctx, env.store, env.reload); err != nil {
		serverError(w, fmt.Errorf("builder: written, but the live reload failed -- a restart will pick it up: %w", err))
		return "", false
	}
	return "/builder/" + url.PathEscape(appID) + "?" + url.Values{"node": {record}, "tab": {"edit"}}.Encode(), true
}
