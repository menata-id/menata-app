package rendering

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestHomeDraftApplicationRows_listFormIsARowNotACard: board 03 draws a draft as one more row of the
// Applications list; the stand-alone form keeps the dashed card for the page that draws no list.
func TestHomeDraftApplicationRows_listFormIsARowNotACard(t *testing.T) {
	rows := []HomeDraftApplicationRow{{Name: "Leave", ReviewHref: "/new-application/s1/review"}}
	render := func(inList bool) string {
		var buf bytes.Buffer
		ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Acme", false)
		if err := HomeDraftApplicationRows(rows, inList).Render(ctx, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return resolveColourRoles(t, ctx, buf.String())
	}
	if got := render(true); strings.Contains(got, "border-dashed border-indigo-200 bg-white px-4") || !strings.Contains(got, "border-t") {
		t.Errorf("list form should be a divided row, got %s", got)
	}
	if got := render(false); !strings.Contains(got, "rounded-lg border border-dashed border-indigo-200 bg-white px-4") {
		t.Errorf("stand-alone form should keep the dashed card, got %s", got)
	}
}

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
	home := render(WorkspaceHomePage("Acme", admin, "", nil, nil))
	for _, want := range []string{"Members, applications, security", "Describe it, Menata builds a draft"} {
		if !strings.Contains(home, want) {
			t.Errorf("launcher does not carry the hint %q", want)
		}
	}
	member := render(WorkspaceHomePage("Acme", Viewer{Initials: "AN", WorkspaceRole: "member"}, "", nil, nil))
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
