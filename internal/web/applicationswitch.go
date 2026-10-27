package web

import (
	"errors"
	"log"
	"net/http"

	"menata.app/internal/data"
	"menata.app/internal/installer"
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
