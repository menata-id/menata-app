package web

import (
	"net/http"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// showAccountMenuWorkspaces serves the Account menu's own "Workspaces" section (AccountMenu.dc.html,
// Flow 2 canvas re-audit -- ROADMAP.md, 2026-09-27) as an htmx fragment, fetched lazily
// (accountMenu's own hx-trigger="toggle once ...") rather than threaded through appShell's
// parameters -- appShell renders on almost every authenticated page, and loadWorkspaceChoices'
// single query is a cost most page loads would never spend on a menu that stays closed.
//
// Reuses loadWorkspaceChoices/currentUserEmail (auth.go), the identical query the Choose/Switch
// Workspace pages already run, rather than a third copy of either. Archived Workspaces are
// dropped before rendering -- the mockup's own account menu draws none, unlike the full Choose
// Workspace page's own expandable Archived section.
func showAccountMenuWorkspaces(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			render(ctx, w, rendering.AccountMenuWorkspaceRows(nil, ""))
			return
		}
		choices, err := loadWorkspaceChoices(ctx, store, email)
		if err != nil {
			serverError(w, err)
			return
		}
		active := make([]rendering.WorkspaceChoice, 0, len(choices))
		for _, c := range choices {
			if !c.Archived {
				active = append(active, c)
			}
		}
		currentID, _ := data.WorkspaceScope(ctx)
		render(ctx, w, rendering.AccountMenuWorkspaceRows(active, currentID))
	}
}
