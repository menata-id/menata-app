package rendering

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestAdministrationHintsAndNewApplicationSteps holds S1c: board 14's hint under each launcher
// Administration row, and boards 15/16's back link and step markers.
func TestAdministrationHintsAndNewApplicationSteps(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Acme", false)
	render := func(c interface {
		Render(context.Context, io.Writer) error
	}) string {
		var buf bytes.Buffer
		if err := c.Render(ctx, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	admin := Viewer{Initials: "AN", WorkspaceRole: "admin"}
	home := render(appShell("T", "Acme", admin, ""))
	for _, want := range []string{"Members, applications, security", "Describe it, Menata builds a draft"} {
		if !strings.Contains(home, want) {
			t.Errorf("launcher does not carry the hint %q", want)
		}
	}
	member := render(appShell("T", "Acme", Viewer{Initials: "AN", WorkspaceRole: "member"}, ""))
	if strings.Contains(member, "Members, applications, security") {
		t.Errorf("a plain member's launcher offers the Administration rows")
	}
	page := render(NewApplicationPage(ConversationView{}, "Acme", admin, ""))
	for _, want := range []string{"Step 1 of 2", "← Applications", `href="/home"`} {
		if !strings.Contains(page, want) {
			t.Errorf("conversation page does not contain %q", want)
		}
	}
}
