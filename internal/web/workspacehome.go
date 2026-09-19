package web

import (
	"errors"
	"net/http"
	"time"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showWorkspaceHome is the landing page after login (ROADMAP.md Phase 21 Step 5): the signed-in
// identity's Workspace, a single Application card (this slice's own scope -- no Application
// plurality yet) showing its real pending-approval count, and a "Your access" summary. A
// membership row is expected to be missing for the shared admin credential's placeholder identity
// (it predates real Workspace membership entirely); that degrades to a blank role rather than an
// error.
func showWorkspaceHome(machines map[string]*domain.Machine, store *data.Store, appName, homeRoute string, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		workspaceID, _ := data.WorkspaceScope(ctx)

		ws, err := store.GetWorkspace(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		membership, err := store.GetMembership(ctx, workspaceID, userID)
		if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
			serverError(w, err)
			return
		}
		if membership == nil {
			membership = &data.Membership{}
		}

		inbox, err := composition.ApprovalInbox(ctx, composition.NewLoader(store, machines), userID, time.Now(), machines[action.StepMachineID])
		if err != nil {
			serverError(w, err)
			return
		}

		// userName is best-effort: missing for the shared admin credential's placeholder identity
		// the same way membership itself is (see this function's own doc comment) -- Initials("")
		// degrades to "?" rather than erroring.
		userName := ""
		if userRecord, err := store.GetRecord(ctx, "mch_user", userID); err == nil {
			userName = composition.DisplayString(userRecord.Values["fld_name"])
		}

		switchHref := ""
		if membership.Email != "" {
			if choices, err := loadWorkspaceChoices(ctx, store, membership.Email); err == nil && len(choices) > 1 {
				switchHref = "/switch-workspace"
			}
		}

		// homeRoute is domain.Application.HomeRoute (Routes' own Deps.HomeRoute), resolved once at
		// startup from metadata's home_card: true item -- never a literal here or in
		// workspacehome.templ (internal/conformance's TestWorkspaceLevelPagesHaveNoHardcodedApplicationRoute
		// and TestWorkspaceLevelHandlersHaveNoHardcodedApplicationRoute hold this page and this
		// handler to that). Empty when metadata declares no home_card item: "/home" is always a
		// valid destination, never a guessed Application route.
		if homeRoute == "" {
			homeRoute = "/home"
		}

		render(ctx, w, rendering.WorkspaceHomePage(
			ws.Name, appName, membership.WorkspaceRole, membership.AppRole, len(inbox.Pending),
			composition.Initials(userName), composition.Initials(appName), switchHref, homeRoute,
		))
	}
}
