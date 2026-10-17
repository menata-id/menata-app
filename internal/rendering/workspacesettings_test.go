package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

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
	if err := WorkspaceSettingsPage("Dokter Kecil", Viewer{Initials: "AN"}, "", nil).Render(ctx, &buf); err != nil {
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

// TestWorkspaceSettingsPage_installationSection pins the reload button and the snapshot list: the reload form posts
// to the admin route, each snapshot is its own form carrying its id and the csrf token, the confirm names the undo,
// and with none saved the page says so instead of drawing an empty list.
func TestWorkspaceSettingsPage_installationSection(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Acme", false)
	render := func(rows []SnapshotRow) string {
		var buf bytes.Buffer
		if err := WorkspaceSettingsPage("Acme", Viewer{Initials: "AN"}, "", rows).Render(ctx, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}

	empty := render(nil)
	for _, want := range []string{`action="/reload-workspace"`, "Reload from disk", "No saved installations yet."} {
		if !strings.Contains(empty, want) {
			t.Errorf("page with no snapshots missing %q", want)
		}
	}
	if strings.Contains(empty, "/restore-workspace-snapshot") {
		t.Error("page with no snapshots drew a restore form")
	}

	rows := []SnapshotRow{
		{ID: "20261010T221500.000000001Z", TakenAt: time.Date(2026, 10, 10, 22, 15, 0, 1, time.UTC), Size: 2048},
		{ID: "20261009T080000.000000002Z", TakenAt: time.Date(2026, 10, 9, 8, 0, 0, 2, time.UTC), Size: 1},
	}
	html := render(rows)
	if got := strings.Count(html, `action="/restore-workspace-snapshot"`); got != 2 {
		t.Errorf("restore forms = %d, want one per snapshot (2)", got)
	}
	if got := strings.Count(html, `name="csrf_token"`); got < 3 {
		t.Errorf("csrf inputs = %d, want one in the reload form and one per restore form (3 at least)", got)
	}
	for _, want := range []string{
		`name="snapshot"`, `value="20261010T221500.000000001Z"`, `value="20261009T080000.000000002Z"`,
		"2026-10-10 22:15:00 UTC · 2 KB", "2026-10-09 08:00:00 UTC · 1 KB",
		"The current one is saved first.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("page with snapshots missing %q", want)
		}
	}
	if strings.Contains(html, "No saved installations yet.") {
		t.Error("page with snapshots still says there are none")
	}
}
