package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// The screens in this file follow one shape: resolve whatever the request carries, ask
// internal/composition for the page's content, render it. The joins, rollups and SLA bucketing
// they used to perform inline now live beside their own tests (development-history.md Phase 19 Step 3).

// pageChrome resolves the three values every appShell-based screen in this file threads down to
// its own rendering.XxxPage call -- workspace name, viewer, and the launcher's switch-workspace
// href -- the same resolution showApprovalInbox already does inline (internal/web/approval.go),
// pulled out here because these handlers all need exactly it and nothing more (no
// workspaceRole: none of these screens has a role-gated link the way Workspace Home's "Manage
// members" does).
func pageChrome(ctx context.Context, req *http.Request, store *data.Store, cfg config.Config) (workspaceName string, viewer rendering.Viewer, switchWorkspaceHref string, err error) {
	chrome, err := resolveChrome(ctx, req, store, cfg)
	if err != nil {
		return "", rendering.Viewer{}, "", err
	}
	userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	_, switchHref := viewerWorkspaceContext(ctx, store, userID)
	return chrome.WorkspaceName, chrome.Viewer(), switchHref, nil
}

// showCalendar is Case 19's week-grid Layout (Case 19 PM05): every mch_task due in one Monday-Sunday week,
// one column per day. `?week=N` steps N weeks from the current one; anything that is not a number is this week.
func showCalendar(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		machines := machinesFor(ctx)

		weekOffset, _ := strconv.Atoi(req.URL.Query().Get("week"))
		c, err := composition.CalendarWeek(ctx, composition.NewLoader(store, machines), time.Now(), weekOffset)
		if err != nil {
			serverError(w, err)
			return
		}
		workspaceName, viewer, switchHref, err := pageChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.CalendarPage(c, workspaceName, viewer, switchHref))
	}
}
