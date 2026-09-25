package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestWorkspaceSettingsPage_realRowsAndPlaceholders is this page's own proof: a real destination
// renders as a link to its actual route, and a not-yet-built section renders as an honest,
// non-interactive "Not built yet" row rather than a link to nowhere. No fixture navigation is
// needed beyond WithCurrentWorkspace's zero-value Workspace -- every id this page resolves
// (nav_workspace_settings, nav_home, nav_workspace_members) is one of domain.RuntimeScreens,
// appended by declaredNavigation regardless of what the Workspace itself declares.
func TestWorkspaceSettingsPage_realRowsAndPlaceholders(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Dokter Kecil")

	var buf bytes.Buffer
	if err := WorkspaceSettingsPage("Dokter Kecil", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()

	for _, want := range []string{
		`href="/home"`, "Applications",
		`href="/workspace-members"`, "Workspace Members", "Invitations",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	for _, label := range []string{"General", "Authentication", "Audit log", "Danger zone"} {
		if !strings.Contains(html, label) {
			t.Errorf("rendered page missing placeholder row %q", label)
		}
	}
	if strings.Count(html, "Not built yet") != 4 {
		t.Errorf(`"Not built yet" appeared %d times, want 4 (General, Authentication, Audit log, Danger zone)`, strings.Count(html, "Not built yet"))
	}
}
