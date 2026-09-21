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

// showRoleMatrix serves the Authorization Matrix (ui-sample/case-03-flow1/
// 06b-authorization-matrix.html): who can do what here, in two sections -- the Workspace, then one
// block per Application.
//
// Gated by requireWorkspaceAdmin, like the Members and Groups routes it sits beside: it renders
// the whole Workspace's access model, which is administration. The nav item is hidden from a plain
// member by the same membersHiddenFor list, so nobody is offered a link that only ever 403s.
//
// Every Application renders at once. It used to take ?app= and show one, which was the mockup's
// own selector -- dropped 2026-09-21 with the restructure, because with the sections split by
// scope the selector hid exactly the comparison the page exists to make, and an Application
// declaring no roles had no tab to be found under at all.
//
// It reads metadata only. There is no store call for the matrix itself, which is the honest shape
// of this screen: every fact it draws is declared, so nothing about it can differ between two
// people looking at it, and composition.RoleMatrix is a pure function over the loaded Workspace.
func showRoleMatrix(machineList []*domain.Machine, store *data.Store, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		workspaceRole, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.RoleMatrixPage(
			composition.RoleMatrix(ws.Applications, machineList),
			chrome.WorkspaceName, chrome.UserInitials, workspaceRole, switchHref,
		))
	}
}
