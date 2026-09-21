package web

import (
	"context"
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// shellChrome is what rendering.appShell needs beyond the page's own content: the Workspace name
// for its breadcrumb (Fase 2 of the ui-sample/case-03-flow1 port), and the viewer's own Name/
// Email/Initials for the Account menu (added Account menu port, 2026-09-21) -- all three read
// from the one mch_user record this function already loads, so Email costs nothing new and Name
// is exposed rather than only its derived Initials.
type shellChrome struct {
	WorkspaceName string
	Name          string
	Email         string
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
	userName, userEmail := "", ""
	userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	if userRecord, err := store.GetRecord(ctx, domain.UserMachineID, userID); err == nil {
		userName = composition.DisplayString(userRecord.Values["fld_name"])
		userEmail = composition.DisplayString(userRecord.Values["fld_email"])
	}

	return shellChrome{WorkspaceName: ws.Name, Name: userName, Email: userEmail, UserInitials: composition.Initials(userName)}, nil
}

// Viewer is shellChrome's three identity fields as rendering.appShell's own parameter type, so
// every call site builds it the same way instead of repeating the field-by-field literal.
func (c shellChrome) Viewer() rendering.Viewer {
	return rendering.Viewer{Name: c.Name, Email: c.Email, Initials: c.UserInitials}
}

// viewerWorkspaceContext resolves the two per-viewer facts every appShell screen threads down to
// rendering: the Workspace role held here (feeds rendering.membersHiddenFor, so a plain member
// isn't offered the two admin-gated launcher destinations) and the launcher's own "All
// Workspaces" link target (rendering.appShell's switchWorkspaceHref parameter).
//
// Both come from one GetMembership call rather than two separate ones (workspaceRoleOf used to be
// its own function; approval.go's own handler duplicated the same query inline) -- every
// appShell-based handler needs both values now that the launcher carries the switch-workspace
// link, so the query is worth sharing.
//
// switchHref is "" exactly when this identity has no membership row with a real email -- the
// shared admin credential's placeholder identity (predates per-user accounts, ROADMAP.md), which
// has nothing a Workspace list could be keyed on. Every other identity gets "/switch-workspace"
// unconditionally (owner request, 2026-09-20): that screen is where an "add workspace" entry
// point is meant to land next, so it stays offered even to someone who belongs to only this one
// Workspace today.
func viewerWorkspaceContext(ctx context.Context, store *data.Store, userID string) (workspaceRole, switchHref string) {
	if userID == "" {
		return "", ""
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	m, err := store.GetMembership(ctx, workspaceID, userID)
	if err != nil {
		return "", ""
	}
	if m.Email != "" {
		switchHref = "/switch-workspace"
	}
	return m.WorkspaceRole, switchHref
}
