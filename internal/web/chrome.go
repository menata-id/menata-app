package web

import (
	"context"
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// shellChrome is what rendering.appShell needs beyond the page's own content: the Workspace name
// for its breadcrumb, and the viewer's initials for its avatar (Fase 2 of the
// ui-sample/case-03-flow1 port).
type shellChrome struct {
	WorkspaceName string
	UserInitials  string
}

// resolveChrome reads both values for one request. The three Workspace-level handlers
// (showWorkspaceHome, showWorkspaceMembers, showEditMember) each need the identical pair, which
// is what makes this a function rather than three copies -- and keeps each of them inside
// internal/conformance's own handler-size budget (TestHandlersStaySmall).
//
// Deliberately *not* resolved in requireAuth and carried on ctx, which was the first design:
// that would add a Workspace lookup and a user-record read to every authenticated request,
// including every /api/* call and every HTMX partial, to serve chrome that only full-page
// Workspace-level screens render. Three explicit call sites cost less and say more.
//
// Both halves are best-effort in the same way showWorkspaceHome's own userName already was: the
// shared admin credential's placeholder identity has no mch_user record and no membership row at
// all, so this degrades to an empty name and Initials("") == "?" rather than failing a page that
// would otherwise render. A real error reaching the database is still returned.
func resolveChrome(ctx context.Context, req *http.Request, store *data.Store, cfg config.Config) (shellChrome, error) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	ws, err := store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return shellChrome{}, err
	}

	// domain.UserMachineID is the runtime's own declared identity Machine -- the same constant
	// FieldTypePerson resolves against -- so this is a reference, not a hardcoded Machine id.
	// "fld_name" still is one: there is no declared "which Field is a record's display name"
	// pointer yet, which is why composition.DisplayString callers all name it. Forward-checkable
	// pointer: ROADMAP.md's Case 03 Fase 3b, where membership and identity metadata are reworked.
	userName := ""
	userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	if userRecord, err := store.GetRecord(ctx, domain.UserMachineID, userID); err == nil {
		userName = composition.DisplayString(userRecord.Values["fld_name"])
	}

	return shellChrome{WorkspaceName: ws.Name, UserInitials: composition.Initials(userName)}, nil
}
