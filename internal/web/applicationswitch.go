package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

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
	return func(ctx context.Context, _ *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		workspaceID, _ := data.WorkspaceScope(ctx)
		apps := rendering.CurrentWorkspace(ctx).Applications
		out := make([]*data.Record, 0, len(apps))
		for _, app := range apps {
			state, detail := "Active", plural2(len(app.Machines), "machine")
			if app.Inactive {
				state, detail = "Inactive", "Switched off: nobody can open it. Its "+plural2(len(app.Machines), "machine")+" and records are kept."
			}
			out = append(out, &data.Record{ID: app.ID, MachineID: "mch_workspace_application", WorkspaceID: workspaceID,
				Values: map[string]any{"fld_id": app.ID, "fld_title": app.Name, "fld_detail": detail, "fld_state": state}})
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
