package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"menata.app/internal/aiassist"
	"menata.app/internal/builder"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/rendering"
)

// The Application Builder's forms do not write: they stage. What an admin does in a form is kept as pending operations for that admin and that
// Application (app_drafts, Apps Builder plan D3) until they publish, so "3 changes not published" is true across sessions and nobody else's
// view of the Application moves while they work. Publishing replays the operations through the one write path (builder.Commit, after a snapshot, load-verified,
// the Workspace reloaded) and drops the draft; discarding drops it. A form is checked against what is already staged when it is submitted, so a
// change the plan would refuse is refused there, with the plan's own words, and never sits in a draft to fail later.

// draftLocks serialise the load-modify-save of one admin's draft of one Application (two tabs staging at once would lose an operation) and publishing it
// (a change staged between reading the draft and deleting it would be deleted unpublished). Within this process, like builder.Commit's own lock.
var draftLocks sync.Map

func lockDraft(workspaceID, appID, userID string) func() {
	mu, _ := draftLocks.LoadOrStore(workspaceID+"\x00"+appID+"\x00"+userID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	return mu.(*sync.Mutex).Unlock
}

// loadDraft is the operations one admin has staged for one Application, and the installation version they began against.
func loadDraft(ctx context.Context, store *data.Store, appID, userID string) ([]builder.Op, string, error) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	raw, base, err := store.GetAppDraft(ctx, workspaceID, appID, userID)
	if err != nil || raw == nil {
		return nil, "", err
	}
	var ops []builder.Op
	if err := json.Unmarshal(raw, &ops); err != nil {
		return nil, "", err
	}
	return ops, base, nil
}

// stagedApplication is the Application as the admin's draft would leave it: what the forms start from and are compared with, so a form that undoes a staged
// change stages the undoing instead of vanishing against what is installed.
func stagedApplication(ctx context.Context, store *data.Store, cfg config.Config, appID, userID string) (aiassist.GeneratedApplication, error) {
	ops, _, err := loadDraft(ctx, store, appID, userID)
	if err != nil {
		return aiassist.GeneratedApplication{}, err
	}
	manifestPath, _, err := workspaceInstallation(ctx, store, cfg)
	if err != nil {
		return aiassist.GeneratedApplication{}, err
	}
	return builder.Staged(manifestPath, appID, ops)
}

// stagedMachine is one Machine of stagedApplication, nil when the draft has no such Machine.
func stagedMachine(app aiassist.GeneratedApplication, machineID string) *aiassist.GeneratedMachine {
	for i := range app.Machines {
		if app.Machines[i].ID == machineID {
			return &app.Machines[i]
		}
	}
	return nil
}

// stageBuilderOps adds ops to the admin's draft of the Application, after the plan accepts the draft with them. It answers like a write would -- 422 with the
// plan's refusal -- and redirects to back. The first change of a draft records the installation it was made against.
func stageBuilderOps(w http.ResponseWriter, req *http.Request, env actionEnv, appID string, ops []builder.Op, back string) (string, bool) {
	ctx := req.Context()
	userID := currentActorID(req, env)
	workspaceID, _ := data.WorkspaceScope(ctx)
	defer lockDraft(workspaceID, appID, userID)()
	staged, base, err := loadDraft(ctx, env.store, appID, userID)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if len(staged) == 0 || base == "" {
		if base, err = builder.AppVersion(manifestPath, appID); err != nil {
			serverError(w, err)
			return "", false
		}
	}
	all := append(staged, ops...)
	// Changes that undo one another leave the Application as it is installed: that is an empty draft, not a plan to refuse as "identical".
	if now, err := builder.Staged(manifestPath, appID, all); err == nil {
		if installed, err := builder.Staged(manifestPath, appID, nil); err == nil && reflect.DeepEqual(now, installed) {
			if err := env.store.DeleteAppDraft(ctx, workspaceID, appID, userID); err != nil {
				serverError(w, err)
				return "", false
			}
			return back, true
		}
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
		err = env.store.SaveAppDraft(ctx, workspaceID, appID, userID, raw, base)
	}
	if err != nil {
		serverError(w, err)
		return "", false
	}
	return back, true
}

// runPublishDraft is the Publish button: the record is the Application's id. The staged operations are written together, or none is and the draft stays. A
// draft whose installation has changed since it began (another admin published, the assistant updated it) is refused with 409 and kept, to be discarded
// and made again: replaying it would silently overwrite what changed. The draft is dropped as soon as the files are written, before the reload, so a reload
// that fails never leaves a draft that would replay on what it already wrote.
func runPublishDraft(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	userID := currentActorID(req, env)
	workspaceID, _ := data.WorkspaceScope(ctx)
	defer lockDraft(workspaceID, record, userID)()
	ops, base, err := loadDraft(ctx, env.store, record, userID)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if len(ops) == 0 {
		http.Error(w, "there is nothing staged to publish", http.StatusConflict)
		return "", false
	}
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	if live, err := builder.AppVersion(manifestPath, record); err != nil {
		serverError(w, err)
		return "", false
	} else if base != "" && live != base {
		http.Error(w, "the application changed since this draft began: discard it and make the changes again", http.StatusConflict)
		return "", false
	}
	drop := func() error { return env.store.DeleteAppDraft(ctx, workspaceID, record, userID) }
	return commitBuilderOpsAt(w, req, env, record, ops, "", drop, "/builder/"+url.PathEscape(record))
}

// runDiscardDraft is the Discard button: the staged operations go and nothing was written.
func runDiscardDraft(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	userID := currentActorID(req, env)
	defer lockDraft(workspaceID, record, userID)()
	if err := env.store.DeleteAppDraft(req.Context(), workspaceID, record, userID); err != nil {
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
		drafts, err := store.ListAppDrafts(ctx, workspaceID, viewer.ID, "the draft bar")
		if err != nil || len(drafts) == 0 {
			return nil, err
		}
		manifestPath, _, manifestErr := workspaceInstallation(ctx, store, cfg)
		var out []*data.Record
		installed := map[string]bool{}
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			installed[app.ID] = true
		}
		for appID, raw := range drafts {
			if !installed[appID] {
				continue // a draft of an Application that is gone is not offered (and does not follow a later Application of the same id: its base version differs)
			}
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
