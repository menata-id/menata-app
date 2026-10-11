package web

import (
	"context"
	"net/http"

	"menata.app/internal/authorization"
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
