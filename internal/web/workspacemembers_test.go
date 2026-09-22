package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
	"menata.app/internal/rendering"
)

// inviteRoleWorkspace is a Workspace with one Application declaring the role these invites assign.
// submittedAppRoles validates against the Application's own roles: vocabulary since Fase 3b, so an
// invite naming a role no Application declares is rejected rather than stored.
func inviteRoleWorkspace() domain.Workspace {
	return domain.Workspace{Applications: []domain.Application{{
		ID: "app_document_approval", Name: "Document Approval", Roles: []string{"approver", "submitter"},
	}}}
}

// TestSubmitInviteMember_recordsInvitationWithoutMembership pins the owner's rule from 2026-09-22:
// inviting someone records an *invitation*, not a membership. Until they accept, they hold no
// membership row, no mch_user record and no role -- so they cannot appear in a member list, be
// picked as an approver, or pass any authorization check.
//
// It replaces TestSubmitInviteMember_usesSuppliedFullName, which pinned the opposite arrangement:
// that the inviting admin's typed name landed on a record created for the invitee. The form no
// longer collects a name at all, because a name belongs to whoever owns the email.
func TestSubmitInviteMember_recordsInvitationWithoutMembership(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "invite-member-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "invite_flow_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Invite Test Workspace", "invite-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	handler := submitInviteMember(store, mail.LogMailer{}, cfg)
	form := strings.NewReader("email=" + email + "&" +
		url.QueryEscape("app_role[app_document_approval]") + "=approver")
	req := httptest.NewRequest(http.MethodPost, "/workspace-members/invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(ctx, ws.ID), inviteRoleWorkspace(), "Test Workspace"))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	members, err := store.ListMembers(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("inviting created %d membership row(s); an invitation is not a membership", len(members))
	}
	records, err := store.ListRecords(data.WithWorkspaceScope(ctx, ws.ID), domain.UserMachineID)
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("inviting created %d mch_user record(s); the record is created on acceptance", len(records))
	}

	invites, err := store.ListPendingInvites(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("want one pending invitation, got %d", len(invites))
	}
	if invites[0].Email != email {
		t.Errorf("invitation email = %q, want %q", invites[0].Email, email)
	}
	// The roles the admin chose wait on the invitation until there is a member to attach them to.
	if got := invites[0].AppRoles["app_document_approval"]; got != "approver" {
		t.Errorf("invitation AppRoles[app_document_approval] = %q, want %q", got, "approver")
	}
}

// TestSubmitInviteMember_missingEmailIsRejected pins the one field the form still requires. An
// email is the Workspace's own decision to make; the invitee's name is not.
func TestSubmitInviteMember_missingEmailIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "invite-member-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "invite_flow_test_noemail@example.com"

	ws, err := store.CreateWorkspace(ctx, "Invite Test Workspace No Email", "invite-test-workspace-no-email")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	handler := submitInviteMember(store, mail.LogMailer{}, cfg)
	req := httptest.NewRequest(http.MethodPost, "/workspace-members/invite", strings.NewReader("email="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(ctx, ws.ID), inviteRoleWorkspace(), "Test Workspace"))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	invites, err := store.ListPendingInvites(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("want no invitation recorded when the email is missing, got %d", len(invites))
	}
}

// TestSubmitInviteMember_existingMemberIsRejected guards the case that would otherwise try to
// create a second membership for one identity: accepting such an invitation would violate
// workspace_members' own (workspace_id, user_record_id) shape, and the invitation grants nothing
// they do not already hold.
func TestSubmitInviteMember_existingMemberIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "invite-member-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "invite_flow_test_existing@example.com"

	ws, err := store.CreateWorkspace(ctx, "Invite Test Workspace Existing", "invite-test-workspace-existing")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	user, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), domain.UserMachineID, map[string]any{})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	handler := submitInviteMember(store, mail.LogMailer{}, cfg)
	req := httptest.NewRequest(http.MethodPost, "/workspace-members/invite", strings.NewReader("email="+email))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(ctx, ws.ID), inviteRoleWorkspace(), "Test Workspace"))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	invites, err := store.ListPendingInvites(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListPendingInvites: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("want no invitation recorded for an existing member, got %d", len(invites))
	}
}
