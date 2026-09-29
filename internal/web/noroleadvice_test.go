package web

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestNoRoleMessage_tellsAnAdminHowToFixIt exists because the same install gap produced this 403
// three times -- for the AI publish path, then dokter-kecil, then hanomerch -- and each time the
// person hitting it could not tell a broken install from a deliberate exclusion.
//
// internal/installer may not reach the database at all (boundary_test.go), so the role grant is
// necessarily a separate step and an install that skips POST /install-application skips it. That
// cannot be prevented here; what can be fixed is the dead end.
func TestNoRoleMessage_tellsAnAdminHowToFixIt(t *testing.T) {
	app := domain.Application{ID: "app_document_approval", Name: "Document Approval"}
	members := domain.RuntimeScreenRoute("nav_workspace_members")
	if members == "" {
		t.Fatal("nav_workspace_members resolves to no route -- the message would point nowhere")
	}

	admin := noRoleMessage(app, domain.Actor{ID: "usr_1", WorkspaceRole: "admin"})
	if !strings.Contains(admin, members) {
		t.Errorf("the admin message does not name %s, so it says what is wrong without saying where to fix it:\n  %s", members, admin)
	}
	if !strings.Contains(admin, "No one") {
		t.Errorf("the admin message blames the viewer rather than the missing grant:\n  %s", admin)
	}
	if !strings.Contains(admin, app.Name) {
		t.Errorf("the message does not name the Application:\n  %s", admin)
	}

	member := noRoleMessage(app, domain.Actor{ID: "usr_2", WorkspaceRole: "member"})
	if !strings.Contains(member, "Ask a workspace admin") {
		t.Errorf("a plain member is not told who can fix it:\n  %s", member)
	}
	if admin == member {
		t.Error("an admin and a member get the same advice, but only one of them can act on it")
	}
}
