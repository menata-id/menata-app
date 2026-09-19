package web

import (
	"errors"
	"net/http"
	"time"

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
func showWorkspaceHome(machines map[string]*domain.Machine, store *data.Store, appName string, cfg config.Config) http.HandlerFunc {
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

		inbox, err := composition.ApprovalInbox(ctx, composition.NewLoader(store, machines), userID, time.Now())
		if err != nil {
			serverError(w, err)
			return
		}

		render(ctx, w, rendering.WorkspaceHomePage(ws.Name, appName, membership.WorkspaceRole, membership.AppRole, len(inbox.Pending)))
	}
}
