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

// showRoleMatrix serves board 06, the Approval Role Matrix (ROADMAP.md Case 03 Fase 7):
// which role may perform which declared transition, for one Application.
//
// Gated by requireWorkspaceAdmin, like the Members and Groups routes it sits beside -- it renders
// the whole Workspace's access model, which is administration, and the nav item is hidden from a
// plain member by the same membersHiddenFor list so nobody is offered a link that only ever 403s
// (the gap Fase 2's launcher first exposed).
//
// ?app= selects the Application; the first one declaring roles is the default. An unknown or
// role-less id falls back to that default rather than 404ing, because the selector is a GET form
// and a stale bookmark should land on a real table rather than an error page.
//
// It reads metadata only. There is no store call here at all, which is the honest shape of this
// screen: every fact it draws is declared, so nothing about it can differ between two people
// looking at it, and composition.RoleMatrix is a pure function over the loaded Workspace.
func showRoleMatrix(machineList []*domain.Machine, store *data.Store, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		apps := roleApplications(ws)
		if len(apps) == 0 {
			http.Error(w, "no application in this workspace declares roles", http.StatusNotFound)
			return
		}

		selected := apps[0].ID
		if requested := req.URL.Query().Get("app"); requested != "" {
			for _, a := range apps {
				if a.ID == requested {
					selected = requested
					break
				}
			}
		}
		application, ok := ws.ApplicationByID(selected)
		if !ok {
			serverError(w, errNoSuchApplication)
			return
		}

		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		workspaceRole, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.RoleMatrixPage(
			composition.RoleMatrix(application, machineList), apps,
			chrome.WorkspaceName, chrome.UserInitials, workspaceRole, switchHref,
		))
	}
}

// errNoSuchApplication cannot happen through the branch above -- roleApplications only ever names
// Applications this same Workspace declares -- so it is a programming error rather than a request
// error, and is reported as one instead of being silently smoothed over.
var errNoSuchApplication = errNoSuchApplicationType{}

type errNoSuchApplicationType struct{}

func (errNoSuchApplicationType) Error() string {
	return "role matrix: selected application is not declared by this workspace"
}
