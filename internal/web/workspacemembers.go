package web

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

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
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.WorkspaceMembersPage(members, chrome.WorkspaceName, chrome.UserInitials, roleApplications(ws)))
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
		render(ctx, w, rendering.EditMemberPage(*m, chrome.WorkspaceName, chrome.UserInitials, roleApplications(ws)))
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

// submitInviteMember pre-creates the invited email's mch_user record and membership with no
// credential yet -- the credential itself is only ever created by submitAcceptInvite (invite.go),
// gated on the emailed, workspace-bound token this handler now sends (security audit 2026-09-19,
// H1; ROADMAP.md Phase 21 Step 6's original design let the first successful login activate the
// account instead, which let anyone who knew/guessed the invited email claim it first).
//
// fld_name comes from the inviting admin, not the email address -- it used to default to email,
// which then surfaced everywhere a person's name is displayed (the approver picker's own
// ApproverRow, approvalStepper's assignee label, SummaryCard/PendingApprovalCard's submitter),
// showing an email address instead of a real name until the invitee later edited their own
// record. Required here (mirrors RegistrationPage's own "Your name") so it's never blank at
// creation instead.
func submitInviteMember(machines map[string]*domain.Machine, store *data.Store, mailer mail.Mailer, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
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
		name := strings.TrimSpace(req.FormValue("fld_name"))
		if name == "" {
			http.Error(w, "full name is required", http.StatusUnprocessableEntity)
			return
		}

		userMachine := machines[domain.UserMachineID]
		values := map[string]any{"fld_name": name, "fld_email": email}
		data.ApplyDefaults(userMachine, values)
		if err := data.ValidateRecord(userMachine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		user, err := store.CreateRecord(ctx, domain.UserMachineID, values)
		if err != nil {
			serverError(w, err)
			return
		}
		appRoles, err := submittedAppRoles(req, ws.Applications)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := store.AddMember(ctx, workspaceID, user.ID, email, "member", appRoles[legacyAppRoleApplicationID]); err != nil {
			serverError(w, err)
			return
		}
		for appID, role := range appRoles {
			if err := store.SetMemberAppRole(ctx, workspaceID, user.ID, appID, role); err != nil {
				serverError(w, err)
				return
			}
		}
		sendInviteEmail(ctx, mailer, cfg, email, workspaceID)
		redirectTo(w, req, "/workspace-members")
	}
}
