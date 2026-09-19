package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showWorkspaceMembers lists every member of the signed-in identity's Workspace (ROADMAP.md
// Phase 21 Step 6). Gated by requireWorkspaceAdmin.
func showWorkspaceMembers(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		members, err := store.ListMembers(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.WorkspaceMembersPage(members, appName))
	}
}

func showEditMember(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		m, err := store.GetMembership(ctx, workspaceID, chi.URLParam(req, "userRecordID"))
		if err != nil {
			recordError(w, err)
			return
		}
		render(ctx, w, rendering.EditMemberPage(*m, appName))
	}
}

func submitEditMember(store *data.Store) http.HandlerFunc {
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

		if err := store.UpdateMemberRole(ctx, workspaceID, userRecordID, workspaceRole, req.FormValue("app_role")); err != nil {
			recordError(w, err)
			return
		}
		redirectTo(w, req, "/workspace-members")
	}
}

// submitInviteMember pre-creates the invited email's mch_user record and membership with no
// credential yet -- their own first successful login activates the account (authenticateMember,
// ROADMAP.md Phase 21 Step 6's own design decision: no email-sending infrastructure exists here).
func submitInviteMember(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
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

		userMachine := machines[domain.UserMachineID]
		values := map[string]any{"fld_name": email, "fld_email": email}
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
		if err := store.AddMember(ctx, workspaceID, user.ID, email, "member", req.FormValue("app_role")); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/workspace-members")
	}
}
