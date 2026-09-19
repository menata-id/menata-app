package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// renderWorkspaceHomeForRole renders WorkspaceHomePage with the given workspaceRole and every
// other parameter fixed to a minimal valid fixture, returning the Members link's presence.
func workspaceHomeHasMembersLink(t *testing.T, workspaceRole string) bool {
	t.Helper()
	var buf bytes.Buffer
	c := WorkspaceHomePage("Acme", "Task Tracker", workspaceRole, "member", 0, "AN", "TT", "", "/home")
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return strings.Contains(buf.String(), `href="/workspace-members"`)
}

// TestWorkspaceHomePage_membersLinkVisibility is the regression test for a code-review finding
// (2026-09-19): requireWorkspaceAdmin (internal/web/middleware.go) fails *open* for an identity
// with no real membership row at all (the shared admin credential predating per-user accounts),
// letting it reach /workspace-members regardless of role -- showWorkspaceHome degrades that same
// case to workspaceRole == "" (data.Membership{}). Gating the Members link on
// workspaceRole == "admin" would hide a link that identity can still open directly; gating on
// != "member" (what workspacehome.templ actually does) shows it for both a real admin and a
// missing membership row, hiding it only for a real member -- the same three-way split
// requireWorkspaceAdmin already makes.
func TestWorkspaceHomePage_membersLinkVisibility(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{role: "admin", want: true},
		{role: "", want: true}, // no real membership row -- requireWorkspaceAdmin's fail-open case
		{role: "member", want: false},
	}
	for _, tt := range tests {
		if got := workspaceHomeHasMembersLink(t, tt.role); got != tt.want {
			t.Errorf("workspaceRole = %q: Members link present = %v, want %v", tt.role, got, tt.want)
		}
	}
}
