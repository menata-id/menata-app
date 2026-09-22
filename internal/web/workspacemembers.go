package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
	"menata.app/internal/rendering"
)

// legacyAppRoleApplicationID names the Application whose role still mirrors into
// workspace_members.app_role, the pre-Fase-3b single-Application column migration 008
// deliberately keeps so a rollback loses nothing. Document Approval, because that column's stored
// values (approver/submitter/reviewer) are exactly its declared vocabulary and it was the only
// Application in existence when they were written.
//
// This constant disappears with the column, in the migration that drops it.
const legacyAppRoleApplicationID = "app_document_approval"

// roleApplications is the Applications a member can actually be given a role in: those that
// declare a vocabulary. An Application declaring none (project-management.yaml today) offers no
// select and no access row, rather than an empty dropdown or a borrowed vocabulary.
func roleApplications(ws domain.Workspace) []rendering.RoleApplication {
	out := make([]rendering.RoleApplication, 0, len(ws.Applications))
	for _, app := range ws.Applications {
		if len(app.Roles) == 0 {
			continue
		}
		out = append(out, rendering.RoleApplication{
			ID:    app.ID,
			Name:  app.Name,
			Field: appRoleField(app.ID),
			Roles: app.Roles,
		})
	}
	return out
}

// showWorkspaceMembers lists every member of the signed-in identity's Workspace (ROADMAP.md
// Phase 21 Step 6). Gated by requireWorkspaceAdmin.
func showWorkspaceMembers(store *data.Store, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		members, err := store.ListMembers(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		// Two separate lists on purpose: members, and invitations nobody has accepted yet. The
		// second set holds no membership and no role, which is why it cannot simply be a filtered
		// view of the first (migrations/011_pending_invites.sql).
		names, err := store.MemberNames(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		pending, err := store.ListPendingInvites(ctx, workspaceID)
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
		render(ctx, w, rendering.WorkspaceMembersPage(members, names, pending, chrome.WorkspaceName, chrome.Viewer(), roleApplications(ws), switchHref))
	}
}

func showEditMember(store *data.Store, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		m, err := store.GetMembership(ctx, workspaceID, chi.URLParam(req, "userRecordID"))
		if err != nil {
			recordError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		viewerID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, viewerID)
		names, err := store.MemberNames(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.EditMemberPage(*m, names[m.UserRecordID], chrome.WorkspaceName, chrome.Viewer(), roleApplications(ws), switchHref))
	}
}

func submitEditMember(store *data.Store, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		userRecordID := chi.URLParam(req, "userRecordID")

		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		workspaceRole := req.FormValue("workspace_role")
		if workspaceRole != "admin" && workspaceRole != "member" {
			http.Error(w, "invalid workspace role", http.StatusUnprocessableEntity)
			return
		}

		appRoles, err := submittedAppRoles(req, ws.Applications)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		// UpdateMemberRole still writes the legacy single app_role column alongside the new rows
		// (migration 008 keeps it so a rollback loses nothing). It is fed Document Approval's
		// role, which is what that column has always held.
		if err := store.UpdateMemberRole(ctx, workspaceID, userRecordID, workspaceRole, appRoles[legacyAppRoleApplicationID]); err != nil {
			recordError(w, err)
			return
		}
		for appID, role := range appRoles {
			if err := store.SetMemberAppRole(ctx, workspaceID, userRecordID, appID, role); err != nil {
				serverError(w, err)
				return
			}
		}
		redirectTo(w, req, "/workspace-members")
	}
}

// submitInviteMember records an invitation and emails its workspace-bound token. It creates no
// membership and no mch_user record: those come into existence when the invitation is *accepted*
// (submitAcceptInvite, invite.go), which is the owner's rule from 2026-09-22 -- someone who has
// confirmed nothing is not a member, so they must not appear in a member list, in an approver
// picker, or in any authorization lookup.
//
// It also no longer asks for the invitee's full name, and that is the same rule seen from the
// other side: a name belongs to whoever owns the email. The old form had the inviting admin type
// a stranger's name into a Field on a record created for them; now the invitee states it once,
// when they accept, onto their own identity (migration 010). An admin supplies the email and the
// roles -- the two things that genuinely are the Workspace's decision.
func submitInviteMember(store *data.Store, mailer mail.Mailer, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)

		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		email := normalizeEmail(req.FormValue("email"))
		if email == "" {
			http.Error(w, "email is required", http.StatusUnprocessableEntity)
			return
		}
		appRoles, err := submittedAppRoles(req, ws.Applications)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		// Already a member of this Workspace: the invitation would have nothing to grant, and
		// accepting it would try to create a second membership for the same identity.
		if _, ok, err := resolveWorkspaceMembership(ctx, store, email, workspaceID); err != nil {
			serverError(w, err)
			return
		} else if ok {
			http.Error(w, "that email is already a member of this workspace", http.StatusUnprocessableEntity)
			return
		}

		invite := data.PendingInvite{WorkspaceID: workspaceID, Email: email, WorkspaceRole: "member", AppRoles: appRoles}
		if err := store.CreatePendingInvite(ctx, invite); err != nil {
			serverError(w, err)
			return
		}
		sendInviteEmail(ctx, mailer, cfg, email, workspaceID)
		redirectTo(w, req, "/workspace-members")
	}
}

// submitRevokeInvite withdraws an invitation that has not been accepted. There is no membership to
// remove and no record to delete -- that is exactly what an invitation living in its own table
// buys -- so this is one delete, and the emailed link stops working immediately because
// submitAcceptInvite re-checks that the invitation still exists at the moment it is used.
func submitRevokeInvite(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		email := normalizeEmail(req.FormValue("email"))
		if email == "" {
			http.Error(w, "email is required", http.StatusUnprocessableEntity)
			return
		}
		if err := store.DeletePendingInvite(ctx, workspaceID, email); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/workspace-members")
	}
}
