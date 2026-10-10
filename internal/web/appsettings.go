package web

import (
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showApplicationSettings serves any Application's Settings hub at /settings/{navID} (ROADMAP.md S2.1),
// the way showDeclaredPage serves /pages/{navID}: the id names a navigation item carrying
// `settings_hub: true`, found in the request's own Application. An id that names no such item -- or
// one belonging to an Application this Workspace has not installed -- is a 404, never a guess.
// `?section=permissions` selects the Permissions view, which only changes what a phone shows; the
// desktop layout draws the same content either way.
//
// Gated by requireApplicationAccess alone, not requireWorkspaceAdmin: the page is read-only
// information for any member, and the two rows that manage state stay pointed at their already-
// admin-gated destinations and are hidden rather than offered-then-403.
func showApplicationSettings(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		app, ok := rendering.CurrentApplication(ctx)
		if !ok {
			http.NotFound(w, req)
			return
		}
		hubItem, ok := composition.SettingsHubOf(app, chi.URLParam(req, "navID"))
		if !ok {
			http.NotFound(w, req)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		isAdmin := chrome.WorkspaceRole != domain.WorkspaceRoleMember

		render(ctx, w, rendering.ApplicationSettingsPage(
			composition.ApplicationSettingsHub(app, hubItem, isAdmin),
			composition.RoleMatrixForApplication(app, installedMachines(ctx)),
			req.URL.Query().Get("section"), chrome.WorkspaceName, chrome.Viewer(), switchHref,
		))
	}
}

// memberRoleFormField is the one field the scoped role form posts. The Application is in the address,
// so unlike the Workspace pages' namespaced `app_role[<id>]` there is nothing to disambiguate.
const memberRoleFormField = "role"

// settingsHubApplication resolves the Application and hub item a /settings/{navID}/members request is
// about, answering 404 itself when the id names no hub of the request's Application or the Application
// declares no roles (a vocabulary-less Application has no roles to assign). The Application is found
// from the address by applicationForPath, which is how a request under /settings/ reaches this far.
func settingsHubApplication(w http.ResponseWriter, req *http.Request) (domain.Application, domain.NavigationItem, bool) {
	app, ok := rendering.CurrentApplication(req.Context())
	if !ok || len(app.Roles) == 0 {
		http.NotFound(w, req)
		return domain.Application{}, domain.NavigationItem{}, false
	}
	hub, ok := composition.SettingsHubOf(app, chi.URLParam(req, "navID"))
	if !ok {
		http.NotFound(w, req)
		return domain.Application{}, domain.NavigationItem{}, false
	}
	return app, hub, true
}

// showApplicationMembers serves /settings/{navID}/members (ROADMAP.md S2.2): who holds which role in
// this one Application, the role editable in place. Registered in the Workspace-admin group -- the
// authority is the same one that edits a member's roles today; there is no "application admin".
func showApplicationMembers(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		app, hub, ok := settingsHubApplication(w, req)
		if !ok {
			return
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		members, err := store.ListMembers(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		names, err := store.MemberNames(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.ApplicationMembersPage(
			composition.ApplicationMembers(app, hub, members, names), chrome.WorkspaceName, chrome.Viewer(), switchHref,
		))
	}
}

// submitApplicationMemberRole sets one member's direct role in this Application and nothing else:
// not their Workspace role, not their roles elsewhere. The value is checked against the Application's
// own declared vocabulary, an empty one clears the row, and a member who is not in this Workspace is a
// 404 -- SetMemberAppRole alone would insert a row for any user id.
//
// It deliberately leaves the legacy workspace_members.app_role column alone: nothing reads it (the
// Edit member form still writes it for the Document Approval id, see legacyAppRoleApplicationID), and
// keeping a second write here would be a second place to remember it exists.
func submitApplicationMemberRole(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		app, hub, ok := settingsHubApplication(w, req)
		if !ok {
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		role := req.FormValue(memberRoleFormField)
		if role != "" && !slices.Contains(app.Roles, role) {
			http.Error(w, "this application does not declare that role", http.StatusUnprocessableEntity)
			return
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		userRecordID := chi.URLParam(req, "userRecordID")
		if _, err := store.GetMembership(ctx, workspaceID, userRecordID); err != nil {
			recordError(w, err)
			return
		}
		if err := store.SetMemberAppRole(ctx, workspaceID, userRecordID, app.ID, role); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, hub.Route+"/members")
	}
}
