package web

import (
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// submitArchiveWorkspace is the Danger Zone's Archive action (Flow 2 gap study Tahap 7) -- an
// ordinary requireWorkspaceAdmin action on the *current* ctx-scoped Workspace, matching the
// mockup's own "Archive from Workspace settings" (WorkspaceArchived.dc.html). Redirects to
// /choose-workspace rather than /home: the Workspace this session was just sitting in is now
// archived, and showChooseWorkspace itself has no membership-count skip (only completeLogin's
// login-time fast path does), so this always renders the chooser with whatever else this identity
// belongs to.
func submitArchiveWorkspace(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		if err := store.ArchiveWorkspace(ctx, workspaceID); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/choose-workspace")
	}
}

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
