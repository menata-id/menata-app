package web

import (
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// showWorkspaceSettings serves the Workspace-level Settings hub (Flow 2 gap study Tahap 5,
// 2026-09-25, M04a-Settings.dc.html) -- the Workspace-level counterpart of an Application's own
// Settings hub (internal/web/appsettings.go).
//
// Simpler than that one in every way that matters: registered under the same requireWorkspaceAdmin
// group as /workspace-members, /workspace-groups and /authorization-matrix
// (internal/web/router.go), so unlike showApplicationSettings it does not compute its own "is this
// viewer an admin" check -- the middleware already refused anyone else before this handler runs.
// It also composes nothing: every row is either a real link resolved from a RuntimeScreen id
// (domain.RuntimeScreens, routeByID/labelByID/titleByID/descriptionByID) or a static
// settingsPlaceholderRow, so there is no per-request derivation for a composition function to do --
// unlike ApplicationSettingsHub, whose Access section depends on the viewed Application's own
// declared roles.
func showWorkspaceSettings(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.WorkspaceSettingsPage(chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}
