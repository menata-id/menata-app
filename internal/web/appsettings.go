package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showApplicationSettings serves any Application's Settings hub at /settings/{navID} (ROADMAP.md S2.1),
// the way showDeclaredPage serves /pages/{navID}: the id names a navigation item carrying
// `settings_hub: true`, found in the request's own Application. An id that names no such item -- or
// one belonging to an Application this Workspace has not installed -- is a 404, never a guess.
// `?section=permissions` selects the Permissions view, which only changes what a phone shows; the
// desktop layout draws the same content either way.
//
// Gated by requireApplicationAccess alone, not requireWorkspaceAdmin: the page is read-only
// information for any member, and the two rows that manage state stay pointed at their already-
// admin-gated destinations and are hidden rather than offered-then-403.
func showApplicationSettings(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		app, ok := rendering.CurrentApplication(ctx)
		if !ok {
			http.NotFound(w, req)
			return
		}
		hubItem, ok := composition.SettingsHubOf(app, chi.URLParam(req, "navID"))
		if !ok {
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
			composition.ApplicationSettingsHub(app, hubItem, isAdmin),
			composition.RoleMatrixForApplication(app, installedMachines(ctx)),
			req.URL.Query().Get("section"), chrome.WorkspaceName, chrome.Viewer(), switchHref,
		))
	}
}
