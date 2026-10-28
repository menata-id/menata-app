package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"menata.app/internal/builder"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
)

// The Application Builder's forms do not write: they stage. What an admin does in a form is kept as pending operations for that admin and that
// Application (app_drafts, Apps Builder plan D3) until they publish, so "3 changes not published" is true across sessions and nobody else's
// view of the Application moves while they work. Publishing replays the operations through the one write path (builder.Commit, after a snapshot, load-verified,
// the Workspace reloaded) and drops the draft; discarding drops it. A form is checked against what is already staged when it is submitted, so a
// change the plan would refuse is refused there, with the plan's own words, and never sits in a draft to fail later.

// loadDraft is the operations one admin has staged for one Application.
func loadDraft(ctx context.Context, store *data.Store, appID, userID string) ([]builder.Op, error) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	raw, err := store.GetAppDraft(ctx, workspaceID, appID, userID)
	if err != nil || raw == nil {
		return nil, err
	}
	var ops []builder.Op
	if err := json.Unmarshal(raw, &ops); err != nil {
		return nil, err
	}
	return ops, nil
}

// stageBuilderOps adds ops to the admin's draft of the Application, after the plan accepts the draft with them. It answers like a write would -- 422 with the
// plan's refusal, 409 when the installation changed under the draft -- and redirects to back.
func stageBuilderOps(w http.ResponseWriter, req *http.Request, env actionEnv, appID string, ops []builder.Op, back string) (string, bool) {
	ctx := req.Context()
	userID := currentActorID(req, env)
	staged, err := loadDraft(ctx, env.store, appID, userID)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	all := append(staged, ops...)
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	var rejected *installer.RejectedError
	if _, err := builder.Review(manifestPath, appID, all); errors.As(err, &rejected) {
		http.Error(w, rejected.Error(), http.StatusUnprocessableEntity)
		return "", false
	} else if err != nil {
		serverError(w, err)
		return "", false
	}
	raw, err := json.Marshal(all)
	if err == nil {
		workspaceID, _ := data.WorkspaceScope(ctx)
		err = env.store.SaveAppDraft(ctx, workspaceID, appID, userID, raw)
	}
	if err != nil {
		serverError(w, err)
		return "", false
	}
	return back, true
}

// runPublishDraft is the Publish button: the record is the Application's id. The staged operations are written together, or none is and the draft stays.
func runPublishDraft(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	userID := currentActorID(req, env)
	ops, err := loadDraft(ctx, env.store, record, userID)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if len(ops) == 0 {
		http.Error(w, "there is nothing staged to publish", http.StatusConflict)
		return "", false
	}
	back := "/builder/" + url.PathEscape(record)
	next, ok := commitBuilderOps(w, req, env, record, ops, back)
	if !ok {
		return "", false
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	if err := env.store.DeleteAppDraft(ctx, workspaceID, record, userID); err != nil {
		serverError(w, err)
		return "", false
	}
	return next, true
}

// runDiscardDraft is the Discard button: the staged operations go and nothing was written.
func runDiscardDraft(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	if err := env.store.DeleteAppDraft(req.Context(), workspaceID, record, currentActorID(req, env)); err != nil {
		serverError(w, err)
		return "", false
	}
	return "/builder/" + url.PathEscape(record), true
}

// builderDraftsProvider is the `builder_drafts` source: one record for each Application the viewer has staged changes in -- how many, and what the plan says
// each would do (or why it would be refused now). The builder screen narrows to its Application by `fld_app`. The installation is read only for a draft that exists.
func builderDraftsProvider(store *data.Store, cfg config.Config) composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, viewer domain.Actor) ([]*data.Record, error) {
		workspaceID, _ := data.WorkspaceScope(ctx)
		drafts, err := store.ListAppDrafts(ctx, workspaceID, viewer.ID)
		if err != nil || len(drafts) == 0 {
			return nil, err
		}
		manifestPath, _, manifestErr := workspaceInstallation(ctx, store, cfg)
		var out []*data.Record
		for appID, raw := range drafts {
			var ops []builder.Op
			if json.Unmarshal(raw, &ops) != nil || len(ops) == 0 {
				continue
			}
			about := ""
			if manifestErr == nil {
				plan, err := builder.Review(manifestPath, appID, ops)
				var lines []string
				for _, it := range plan.Plan.Items {
					lines = append(lines, it.Op+": "+it.What)
				}
				if err != nil {
					lines = append(lines, "cannot be published as it stands: "+err.Error())
				}
				about = strings.Join(lines, "\n")
			}
			title := "Draft · " + strconv.Itoa(len(ops)) + " change"
			if len(ops) != 1 {
				title += "s"
			}
			out = append(out, &data.Record{ID: appID, MachineID: "mch_builder_draft", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": appID, "fld_app": appID, "fld_title": title + " not published", "fld_about": about,
			}})
		}
		return out, nil
	}
}
