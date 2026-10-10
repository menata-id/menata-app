package rendering

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestWorkspaceSettingsPage_realRowsAndPlaceholders is this page's own proof: a real destination
// renders as a link to its actual route, and a not-yet-built section renders as an honest,
// non-interactive "Not built yet" row rather than a link to nowhere. No fixture navigation is
// needed beyond WithCurrentWorkspace's zero-value Workspace -- every id this page resolves
// (nav_workspace_settings, nav_home, nav_workspace_members) is one of domain.RuntimeScreens,
// appended by declaredNavigation regardless of what the Workspace itself declares.
func TestWorkspaceSettingsPage_realRowsAndPlaceholders(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Dokter Kecil", false)

	var buf bytes.Buffer
	if err := WorkspaceSettingsPage("Dokter Kecil", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()

	for _, want := range []string{
		`href="/home"`, "Applications",
		`href="/workspace-members"`, "Workspace Members", "Invitations",
		// Danger zone (Flow 2 gap study Tahap 7): Archive is real now, Transfer ownership stays a
		// placeholder -- both live under the same "Danger zone" heading.
		"Danger zone", `action="/workspace-settings/archive"`, "Archive workspace",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	for _, label := range []string{"General", "Authentication", "Audit log", "Transfer ownership"} {
		if !strings.Contains(html, label) {
			t.Errorf("rendered page missing placeholder row %q", label)
		}
	}
	if strings.Count(html, "Not built yet") != 4 {
		t.Errorf(`"Not built yet" appeared %d times, want 4 (General, Authentication, Audit log, Transfer ownership)`, strings.Count(html, "Not built yet"))
	}
}

// TestWorkspaceSettingsFrame_sidebarOnTheAdminScreens is S2.3's proof (boards 04/05): the four admin screens
// carry the settings sidebar, the section being viewed is marked current, the unbuilt sections are inert rows,
// and the sidebar is hidden below `sm` so the hub stays the phone's list. The hub itself is the index and does
// not carry it, and its Danger zone is the anchor the sidebar links to.
func TestWorkspaceSettingsFrame_sidebarOnTheAdminScreens(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Acme", false)
	viewer := Viewer{Initials: "AN", WorkspaceRole: "admin"}
	render := func(c interface {
		Render(context.Context, io.Writer) error
	}) string {
		var buf bytes.Buffer
		if err := c.Render(ctx, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	m := data.Membership{UserRecordID: "u1", Email: "rina@example.com", WorkspaceRole: "member"}
	pages := map[string]struct{ html, current string }{
		"members":      {render(WorkspaceMembersPage([]data.Membership{m}, nil, nil, "Acme", viewer, nil, "", "", 1)), "/workspace-members"},
		"edit member":  {render(EditMemberPage(m, "Rina", "Acme", viewer, nil, "")), "/workspace-members"},
		"groups":       {render(GroupsPage(nil, "Acme", viewer, nil, "")), "/workspace-groups"},
		"group detail": {render(GroupDetailPage(data.Group{ID: "g1", Name: "Finance"}, nil, "Acme", viewer, nil, "")), "/workspace-groups"},
	}
	for name, p := range pages {
		if !strings.Contains(p.html, `<aside class="hidden`) {
			t.Errorf("%s: no settings sidebar, or it is not hidden on a phone", name)
		}
		if n := strings.Count(p.html, `aria-current="page"`); n != 1 {
			t.Errorf("%s: %d current sidebar rows, want exactly 1", name, n)
		}
		if !strings.Contains(p.html, `href="`+p.current+`" aria-current="page"`) {
			t.Errorf("%s: the current row is not the %s one", name, p.current)
		}
		for _, want := range []string{`href="/workspace-settings"`, `href="/workspace-settings#danger-zone"`, `href="/home"`, "Authentication", "Audit log", "Not built yet"} {
			if !strings.Contains(p.html, want) {
				t.Errorf("%s: sidebar missing %q", name, want)
			}
		}
	}

	var hub bytes.Buffer
	if err := WorkspaceSettingsPage("Acme", viewer, "").Render(ctx, &hub); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(hub.String(), "<aside") {
		t.Error("the hub is the index and must not carry the sidebar")
	}
	if !strings.Contains(hub.String(), `id="danger-zone"`) {
		t.Error("the hub's Danger zone has no anchor for the sidebar to link to")
	}
}
