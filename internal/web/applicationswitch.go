package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"menata.app/internal/composition"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/rendering"
)

// runDeactivateApplication and runReactivateApplication are the Workspace admin's switch for an Application (registry
// RuntimeActions "deactivate_application"/"reactivate_application"). Each saves the installation first, writes or removes
// `inactive: true` in the Application's own file (installer.SetApplicationInactive, load-verified), and reloads the Workspace.
// Asking for the state it is already in changes nothing and reloads nothing.
func runDeactivateApplication(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	return switchApplication(w, req, env, record, true)
}

func runReactivateApplication(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	return switchApplication(w, req, env, record, false)
}

func switchApplication(w http.ResponseWriter, req *http.Request, env actionEnv, appID string, inactive bool) (string, bool) {
	ctx := req.Context()
	if env.reload == nil {
		serverError(w, errors.New("this process has no reload hook configured"))
		return "", false
	}
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	snapshotBeforeWrite(env.cfg, manifestPath)
	changed, err := installer.SetApplicationInactive(manifestPath, appID, inactive)
	var rejected *installer.RejectedError
	switch {
	case errors.As(err, &rejected):
		http.Error(w, rejected.Error(), http.StatusUnprocessableEntity)
		return "", false
	case err != nil:
		serverError(w, err)
		return "", false
	case !changed:
		return "", true
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	log.Printf("APP-SWITCH app=%s workspace=%s inactive=%v", appID, workspaceID, inactive)
	if err := reloadCurrentWorkspace(ctx, env.store, env.reload); err != nil {
		serverError(w, errors.New("the switch was written but the live reload failed -- a process restart will pick it up: "+err.Error()))
		return "", false
	}
	return "", true
}

// workspaceApplicationsProvider is the reader of the `workspace_applications` source: every Application installed in the current
// Workspace in its installation's order, with whether it is switched off and what its Machines amount to. It is the admin's
// list, which is why it keeps the Applications the menu and Home leave out.
func workspaceApplicationsProvider() composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		workspaceID, _ := data.WorkspaceScope(ctx)
		apps := rendering.CurrentWorkspace(ctx).Applications
		sessions, err := l.AISessionsForHome(ctx)
		if err != nil {
			return nil, err
		}
		origins := map[string]time.Time{}
		for _, s := range sessions {
			if s.Status == data.AISessionStatusPublished && s.ApplicationID != "" && s.PublishedAt != nil {
				if at, seen := origins[s.ApplicationID]; !seen || s.PublishedAt.Before(at) {
					origins[s.ApplicationID] = *s.PublishedAt
				}
			}
		}
		now := time.Now()
		out := make([]*data.Record, 0, len(apps))
		for _, app := range apps {
			state, detail, undo := "Active", plural2(len(app.Machines), "machine"), "None"
			if app.Inactive {
				state, detail = "Inactive", "Switched off: nobody can open it. Its "+plural2(len(app.Machines), "machine")+" and records are kept."
			}
			if at, ok := origins[app.ID]; ok && now.Sub(at) <= domain.PublishUndoWindow {
				undo = "Undo"
				detail += ". Published by the assistant just now: you can take it back until " + at.Add(domain.PublishUndoWindow).Local().Format("15:04") + "."
			}
			out = append(out, &data.Record{ID: app.ID, MachineID: "mch_workspace_application", WorkspaceID: workspaceID,
				Values: map[string]any{"fld_id": app.ID, "fld_title": app.Name, "fld_detail": detail, "fld_state": state, "fld_undo": undo, "fld_open": builderPreviewRoute(app)}})
		}
		return out, nil
	}
}

func plural2(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func (e actionEnv) clock() time.Time {
	if e.now != nil {
		return e.now()
	}
	return time.Now()
}

// runUndoPublish takes back an Application the assistant published within domain.PublishUndoWindow: it removes the Application's files and its own
// Machines' files from the Workspace (installer.RemoveApplication), then what the database holds for it (roles, grants), and reopens the
// proposal for review. It refuses an Application the assistant did not publish, one past the window, and one any of whose Machines holds a
// record -- those are people's work, and the way back for them is Switch off.
func runUndoPublish(w http.ResponseWriter, req *http.Request, env actionEnv, appID string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	refuse := func(msg string) (string, bool) {
		http.Error(w, msg, http.StatusUnprocessableEntity)
		return "", false
	}
	published, ok, err := env.store.AIApplicationOrigin(ctx, workspaceID, appID)
	switch {
	case err != nil:
		serverError(w, err)
		return "", false
	case !ok:
		return refuse("only an application the assistant published can be taken back")
	case env.clock().Sub(published) > domain.PublishUndoWindow:
		return refuse("the ten minutes to take this back have passed; switch it off instead")
	}
	app, found := rendering.CurrentWorkspace(ctx).ApplicationByID(appID)
	if !found {
		return refuse("application " + appID + " is not installed in this workspace")
	}
	counts, err := env.store.CountRecordsByMachine(ctx, app.Machines)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	for id, n := range counts {
		if n > 0 {
			return refuse("people have already added records to " + id + ", so this cannot be taken back; switch it off instead")
		}
	}
	manifestPath, _, err := workspaceInstallation(ctx, env.store, env.cfg)
	if err != nil {
		serverError(w, err)
		return "", false
	}
	snapshotBeforeWrite(env.cfg, manifestPath)
	if _, err := installer.RemoveApplication(manifestPath, appID); err != nil {
		var rejected *installer.RejectedError
		if errors.As(err, &rejected) {
			return refuse(rejected.Error())
		}
		serverError(w, err)
		return "", false
	}
	log.Printf("APP-UNDO app=%s workspace=%s", appID, workspaceID)
	if err := env.store.ForgetPublishedApplication(ctx, workspaceID, appID); err != nil {
		serverError(w, fmt.Errorf("the application was taken out but clearing its roles failed: %w", err))
		return "", false
	}
	if env.reload == nil {
		serverError(w, errors.New("taken out, but this process has no reload hook configured -- a restart will pick it up"))
		return "", false
	}
	if err := reloadCurrentWorkspace(ctx, env.store, env.reload); err != nil {
		serverError(w, fmt.Errorf("taken out, but the live reload failed -- a restart will pick it up: %w", err))
		return "", false
	}
	return "", true
}

// builderPreviewRoute is the address the builder frames to show an Application: its home screen asked for in builder mode. "" for an Application with
// no screen of its own to show.
func builderPreviewRoute(app domain.Application) string {
	if app.HomeRoute == "" {
		return ""
	}
	u, err := url.Parse(app.HomeRoute)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("builder", "1")
	u.RawQuery = q.Encode()
	return u.String()
}
