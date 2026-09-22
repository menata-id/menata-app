package web

import (
	"context"
	"net/http"
	"strings"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showCreateWorkspace renders /switch-workspace's own "Add workspace" affordance
// (ui-sample/choose-workspace.html's dashed-border row, comment: "Admin-only: shown when the
// signed-in user is an admin of at least one workspace, so they can create a new one"). Gated
// server-side the same way the mockup states, not just by the button's own visibility -- the same
// defense-in-depth posture requireWorkspaceAdmin gives the existing Workspace-administration
// screens, applied here without that middleware because creating a *new* Workspace has no current
// Workspace for it to check.
func showCreateWorkspace(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			redirectTo(w, req, "/home")
			return
		}
		isAdmin, err := isAdminOfAnyWorkspace(ctx, store, email)
		if err != nil {
			serverError(w, err)
			return
		}
		if !isAdmin {
			redirectTo(w, req, "/switch-workspace")
			return
		}
		render(ctx, w, rendering.CreateWorkspacePage(""))
	}
}

// submitCreateWorkspace creates the new Workspace and its first mch_user record
// (registerWorkspace's own shape, data.go), and joins them with an admin membership -- but writes
// no new credential: the identity creating it is already signed in and already owns one. Ends by
// re-pointing the session cookie at the new Workspace's own mch_user record
// (submitSwitchWorkspace's own last step) so the new Workspace opens immediately rather than
// leaving the viewer on whichever one they started from.
func submitCreateWorkspace(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			redirectTo(w, req, "/home")
			return
		}
		isAdmin, err := isAdminOfAnyWorkspace(ctx, store, email)
		if err != nil {
			serverError(w, err)
			return
		}
		if !isAdmin {
			redirectTo(w, req, "/switch-workspace")
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		workspaceName := strings.TrimSpace(req.FormValue("workspace_name"))
		if workspaceName == "" {
			render(ctx, w, rendering.CreateWorkspacePage("Workspace name is required."))
			return
		}

		// Nothing identity-shaped is collected or written here: the new Workspace's mch_user
		// record carries only Workspace-scoped values (metadata/user.yaml), and this identity's
		// name and email already exist on its credential.
		userMachine := machines[domain.UserMachineID]
		values := data.ValuesFromForm(userMachine, req.Form)
		data.ApplyDefaults(userMachine, values)
		if err := data.ValidateRecord(userMachine, values); err != nil {
			render(ctx, w, rendering.CreateWorkspacePage(err.Error()))
			return
		}

		ws, err := store.CreateWorkspace(ctx, workspaceName, slugify(workspaceName))
		if err != nil {
			serverError(w, err)
			return
		}
		user, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), domain.UserMachineID, values)
		if err != nil {
			serverError(w, err)
			return
		}
		if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
			serverError(w, err)
			return
		}
		if err := setSessionCookieFor(ctx, store, w, cfg, user.ID); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/home")
	}
}

// isAdminOfAnyWorkspace is showCreateWorkspace/submitCreateWorkspace's own gate. Unlike
// requireWorkspaceAdmin, which checks the *current* Workspace's role, creating a new Workspace has
// no current Workspace to check against -- so this reads across every membership email holds,
// the same list loadWorkspaceChoices (auth.go) already knows how to load.
func isAdminOfAnyWorkspace(ctx context.Context, store *data.Store, email string) (bool, error) {
	memberships, err := store.ListMemberships(ctx, email)
	if err != nil {
		return false, err
	}
	for _, m := range memberships {
		if m.WorkspaceRole == "admin" {
			return true, nil
		}
	}
	return false, nil
}
