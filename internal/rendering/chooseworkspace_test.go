package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestSplitWorkspaceChoices is splitWorkspaceChoices' own proof of the rule its doc comment
// states: live rows pass through untouched, an archived admin membership goes to the archived
// slice, and an archived non-admin membership is dropped entirely -- "hidden from members"
// (WorkspaceArchived.dc.html) enforced by omission, not by a flag the template has to check.
func TestSplitWorkspaceChoices(t *testing.T) {
	choices := []WorkspaceChoice{
		{ID: "ws_live", Name: "Live Co", Role: "member"},
		{ID: "ws_archived_admin", Name: "Archived Admin Co", Role: "admin", Archived: true, ArchivedAt: "12 Jul 2026"},
		{ID: "ws_archived_member", Name: "Archived Member Co", Role: "member", Archived: true, ArchivedAt: "1 Jan 2026"},
	}
	live, archived := splitWorkspaceChoices(choices)

	if len(live) != 1 || live[0].ID != "ws_live" {
		t.Errorf("live = %+v, want exactly ws_live", live)
	}
	if len(archived) != 1 || archived[0].ID != "ws_archived_admin" {
		t.Errorf("archived = %+v, want exactly ws_archived_admin (an archived membership held as member must be dropped, not just unlisted)", archived)
	}
}

// TestChooseWorkspacePage_archivedSectionOnlyForAdmin is the rendered-output half of the same
// rule: an admin sees a "Archived workspaces (1)" disclosure with a Restore form posting to
// action + "/restore", and a plain member's identical archived membership renders no trace of it
// at all -- not a hidden row, no row.
func TestChooseWorkspacePage_archivedSectionOnlyForAdmin(t *testing.T) {
	adminChoices := []WorkspaceChoice{
		{ID: "ws_live", Name: "Live Co", Role: "member"},
		{ID: "ws_old", Name: "Old Co", Role: "admin", Archived: true, ArchivedAt: "12 Jul 2026"},
	}
	var buf bytes.Buffer
	if err := ChooseWorkspacePage(adminChoices, "", "/switch-workspace", "/home", "Back to Menata", false, "silvia@menata.id").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		"Archived workspaces (1)", "Old Co", "archived 12 Jul 2026",
		`action="/switch-workspace/restore"`, "Restore",
		"Live Co",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("admin's rendered page missing %q", want)
		}
	}

	memberChoices := []WorkspaceChoice{
		{ID: "ws_live", Name: "Live Co", Role: "member"},
		{ID: "ws_old", Name: "Old Co", Role: "member", Archived: true, ArchivedAt: "12 Jul 2026"},
	}
	buf.Reset()
	if err := ChooseWorkspacePage(memberChoices, "", "/switch-workspace", "/home", "Back to Menata", false, "silvia@menata.id").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	memberHTML := buf.String()
	for _, unwanted := range []string{"Archived workspaces", "Old Co", "Restore"} {
		if strings.Contains(memberHTML, unwanted) {
			t.Errorf("member's rendered page unexpectedly contains %q -- an archived workspace must be hidden from a plain member entirely", unwanted)
		}
	}
}

// TestChooseWorkspacePage_chooseWorkspaceMockupGaps covers the four Choose Workspace gaps closed
// 2026-09-27 (ROADMAP.md, Flow 2 canvas re-audit): a live row's slug/member-count line, the
// capitalized role badge, the footer's "Signed in as ..." identity line, and the archived
// section's disclosure chevron.
func TestChooseWorkspacePage_chooseWorkspaceMockupGaps(t *testing.T) {
	choices := []WorkspaceChoice{
		{ID: "ws_live", Name: "Dokter Kecil", Slug: "dokter-kecil", MemberCount: 24, Role: "admin"},
		{ID: "ws_old", Name: "Old Co", Role: "admin", Archived: true, ArchivedAt: "12 Jul 2026"},
	}
	var buf bytes.Buffer
	if err := ChooseWorkspacePage(choices, "", "/switch-workspace", "/home", "Back to Menata", false, "silvia@menata.id").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		"dokter-kecil · 24 members",     // 2a: slug + member count line
		"Admin",                         // 2b: capitalized role badge, not lower-case "admin"
		"Signed in as silvia@menata.id", // 2c: footer identity line
		"M9 6l6 6-6 6",                  // 2d: archived-section chevron-right icon's own path data
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page missing %q; html=%s", want, html)
		}
	}
}
