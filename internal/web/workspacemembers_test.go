package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
)

func inviteTestUserMachine() *domain.Machine {
	return &domain.Machine{
		ID: domain.UserMachineID,
		Fields: []domain.Field{
			{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_email", Name: "Email", Type: domain.FieldTypeText, Required: true},
		},
	}
}

// TestSubmitInviteMember_usesSuppliedFullName is the regression test for the gap the owner found
// manually (2026-09-19): submitInviteMember used to default fld_name to the invited email address
// itself, which then surfaced everywhere a person's name is displayed -- the approver picker
// (ApproverRow), approvalStepper's assignee label, SummaryCard/PendingApprovalCard's submitter --
// showing an email instead of a real name until the invitee separately edited their own record.
// The invite form now collects the name up front (workspacemembers.templ), same as
// RegistrationPage's own "Your name" -- this pins that the handler actually uses it.
func TestSubmitInviteMember_usesSuppliedFullName(t *testing.T) {
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

	machines := map[string]*domain.Machine{domain.UserMachineID: inviteTestUserMachine()}
	handler := submitInviteMember(machines, store, mail.LogMailer{}, cfg)

	form := strings.NewReader("email=" + email + "&fld_name=Budi+Santoso&app_role=approver")
	req := httptest.NewRequest(http.MethodPost, "/workspace-members/invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(data.WithWorkspaceScope(ctx, ws.ID))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	members, err := store.ListMembers(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("want one member, got %d", len(members))
	}

	user, err := store.GetRecord(data.WithWorkspaceScope(ctx, ws.ID), domain.UserMachineID, members[0].UserRecordID)
	if err != nil {
		t.Fatalf("GetRecord(invited user): %v", err)
	}
	if got := user.Values["fld_name"]; got != "Budi Santoso" {
		t.Errorf("fld_name = %q, want %q -- must not fall back to the email address", got, "Budi Santoso")
	}
}

// TestSubmitInviteMember_missingNameIsRejected pins the other half: an invite with no name at all
// is rejected outright (422), not silently defaulted back to the email address.
func TestSubmitInviteMember_missingNameIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: "invite-member-test-secret", SecureCookies: false}
	ctx := context.Background()
	const email = "invite_flow_test_noname@example.com"

	ws, err := store.CreateWorkspace(ctx, "Invite Test Workspace No Name", "invite-test-workspace-no-name")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	machines := map[string]*domain.Machine{domain.UserMachineID: inviteTestUserMachine()}
	handler := submitInviteMember(machines, store, mail.LogMailer{}, cfg)

	form := strings.NewReader("email=" + email)
	req := httptest.NewRequest(http.MethodPost, "/workspace-members/invite", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(data.WithWorkspaceScope(ctx, ws.ID))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	members, err := store.ListMembers(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("want no member created when name is missing, got %d", len(members))
	}
}
