package web

import (
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showApplicationSettings serves an Application's Settings hub (ROADMAP.md "In progress",
// "Application Settings hub") and its Permissions sub-page from one handler factory, activeSection
// naming which: "" for the hub itself (/document-approval/settings), "permissions" for
// /document-approval/settings/permissions. Both routes render the identical
// rendering.ApplicationSettingsPage; activeSection is only ever a rendering decision (which the
// mobile view shows), never a different data pipeline, since the desktop layout renders the same
// content either way (see that function's own doc comment).
//
// Gated by requireApplicationAccess alone, like every other Document Approval route -- not
// requireWorkspaceAdmin -- because this page is read-only information for any member; the two rows
// that actually manage state (Members & roles, Groups) stay pointed at their already-admin-gated
// destinations and are hidden rather than offered-then-403 (composition.ApplicationSettingsHub's
// own ShowMembersAndGroups).
func showApplicationSettings(activeSection string, machineList []*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		app, ok := rendering.CurrentApplication(ctx)
		if !ok {
			// Declared in this Application's own navigation (metadata/applications/document-
			// approval.yaml), so currentApplication resolves it for every real request; this is
			// only reachable if the route were somehow hit outside that middleware chain.
			http.NotFound(w, req)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		isAdmin := chrome.WorkspaceRole != domain.WorkspaceRoleMember

		render(ctx, w, rendering.ApplicationSettingsPage(
			composition.ApplicationSettingsHub(app, isAdmin),
			composition.RoleMatrixForApplication(app, machineList),
			activeSection, chrome.WorkspaceName, chrome.Viewer(), switchHref,
		))
	}
}
