package web

import (
	"context"
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
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
	// WorkspaceRole is the viewer's role in *this* Workspace ("admin"/"member"), read from the
	// membership row resolveIdentity already loaded. appShell's Workspace menu needs it: its
	// Workspace-settings row points at a requireWorkspaceAdmin-gated route, and offering a plain
	// member a link that only ever 403s is the same code-review finding that deleted
	// rendering.membersHiddenFor (appshell.templ's own history). Empty for the shared admin
	// credential's placeholder identity, which holds no membership row -- so it sees no admin row
	// either, which is correct rather than unfortunate: cfg.AdminUserID is a break-glass login,
	// not a Workspace member.
	WorkspaceRole string
}

// resolveChrome reads both values for one request. The three Workspace-level handlers
// (showWorkspaceHome, showWorkspaceMembers, showEditMember) each need the identical pair, which
// is what makes this a function rather than three copies -- and keeps each of them inside
// internal/conformance's own handler-size budget (TestHandlersStaySmall).
//
// **It reads what resolveIdentity already resolved, as of 2026-09-22.** This comment used to
// argue the opposite at length -- that carrying chrome on ctx "would add a Workspace lookup and a
// user-record read to every authenticated request... Three explicit call sites cost less and say
// more" -- and the measurement contradicted it. There are fifteen call sites, not three; the
// Workspace lookup was already happening on every request in currentWorkspace, so this one was
// the *second*; and GetMembership is four queries, not one, so calling it here and again in
// viewerWorkspaceContext cost eight. /home issued seventeen queries in total. The argument was
// sound about what it was avoiding and wrong about what was already there.
//
// Both halves are best-effort in the same way showWorkspaceHome's own userName already was: the
// shared admin credential's placeholder identity has no mch_user record and no membership row at
// all, so this degrades to an empty name and Initials("") == "?" rather than failing a page that
// would otherwise render. A real error reaching the database is still returned.
func resolveChrome(ctx context.Context, req *http.Request, store *data.Store, cfg config.Config) (shellChrome, error) {
	if id, ok := identityFrom(ctx); ok {
		name, email, role := id.ViewerName(ctx), "", ""
		if m := id.Membership(ctx); m != nil {
			email, role = m.Email, m.WorkspaceRole
		}
		workspaceName := ""
		if id.workspace != nil {
			workspaceName = id.workspace.Name
		}
		return shellChrome{WorkspaceName: workspaceName, Name: name, Email: email, UserInitials: composition.Initials(name), WorkspaceRole: role}, nil
	}

	// Fallback for a handler mounted without resolveIdentity -- in this repo, a test mounting one
	// handler on a bare chi router. Kept rather than made fatal so those tests keep exercising the
	// handler they are about instead of the middleware chain they deliberately skip.
	workspaceID, _ := data.WorkspaceScope(ctx)
	ws, err := store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return shellChrome{}, err
	}

	// The viewer's name and email come from their identity, not from their mch_user record: both
	// stopped being Fields on 2026-09-22 (metadata/user.yaml, migration 010) because they belong
	// to whoever owns the login rather than to one Workspace. The membership row is what ties this
	// session's record id to that identity, so it is the first hop.
	userName, userEmail, userRole := "", "", ""
	userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	if membership, err := store.GetMembership(ctx, workspaceID, userID); err == nil && membership != nil {
		userEmail, userRole = membership.Email, membership.WorkspaceRole
		userName = viewerNameFor(ctx, store, membership)
	}

	return shellChrome{WorkspaceName: ws.Name, Name: userName, Email: userEmail, UserInitials: composition.Initials(userName), WorkspaceRole: userRole}, nil
}

// Viewer is shellChrome's three identity fields as rendering.appShell's own parameter type, so
// every call site builds it the same way instead of repeating the field-by-field literal.
func (c shellChrome) Viewer() rendering.Viewer {
	return rendering.Viewer{Name: c.Name, Email: c.Email, Initials: c.UserInitials, WorkspaceRole: c.WorkspaceRole}
}

// viewerWorkspaceContext resolves two per-viewer facts. switchWorkspaceHref (the launcher's "All
// Workspaces" link target, rendering.appShell's own parameter) is the one every appShell screen
// still threads down; workspaceRole is returned alongside it for the one caller that still needs
// it (Workspace Home's own "Manage members" link, workspacehome.templ's inline `workspaceRole !=
// "member"` check) -- most callers, including pageChrome (pages.go), discard it with `_`.
// rendering.membersHiddenFor, which this comment used to say workspaceRole fed, was deleted
// 2026-09-21 along with appShell's own hiddenNavIDs parameter (capabilities.md, "Navigation: one
// filter now, not two") -- the launcher stopped listing individual destinations at all, so there
// was nothing left for a viewer-level filter to hide.
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
	// resolveIdentity read this membership once for the whole request; before 2026-09-22 this was
	// a fourth read of it (see resolveChrome above). The fallback is the same as everywhere else
	// here: a handler mounted without the middleware, which means a test.
	if id, ok := identityFrom(ctx); ok {
		m := id.Membership(ctx)
		if m == nil {
			return "", ""
		}
		if m.Email != "" {
			switchHref = "/switch-workspace"
		}
		return m.WorkspaceRole, switchHref
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
