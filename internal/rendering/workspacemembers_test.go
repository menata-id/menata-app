package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func renderMembersPage(t *testing.T, members []data.Membership, pending []data.PendingInvite) string {
	t.Helper()
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Acme", false)
	var buf bytes.Buffer
	c := WorkspaceMembersPage(members, map[string]string{"u1": "Rina"}, pending, "Acme", Viewer{Initials: "AN", WorkspaceRole: "admin"}, nil, "", "", len(members))
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// TestWorkspaceMembersPage_statusColumnAndHeading holds board 04's deltas: the heading and subtitle
// are the runtime screen's declared Title/Description, each of Active/Deactivated/Invited is a Status
// cell of its own, and an invitation shows the role it will become rather than "Invited" in the role column.
func TestWorkspaceMembersPage_statusColumnAndHeading(t *testing.T) {
	body := renderMembersPage(t,
		[]data.Membership{
			{UserRecordID: "u1", Email: "rina@example.com", WorkspaceRole: "admin"},
			{UserRecordID: "u2", Email: "budi@example.com", WorkspaceRole: "member", Deactivated: true},
		},
		[]data.PendingInvite{{Email: "sari@example.com", WorkspaceRole: "member"}},
	)
	for _, want := range []string{">Members<", "Everyone in this workspace", ">Status<", ">Active<", ">Deactivated<", ">Invited<", "managed in that application’s Settings."} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	if strings.Contains(body, "Manage workspace membership and application access.") {
		t.Errorf("page still carries the pre-board subtitle")
	}
}

// TestHomeDraftApplicationRows_listFormIsARowNotACard: board 03 draws a draft as one more row of the
// Applications list; the stand-alone form keeps the dashed card for the page that draws no list.
func TestHomeDraftApplicationRows_listFormIsARowNotACard(t *testing.T) {
	rows := []HomeDraftApplicationRow{{Name: "Leave", ReviewHref: "/new-application/s1/review"}}
	render := func(inList bool) string {
		var buf bytes.Buffer
		if err := HomeDraftApplicationRows(rows, inList).Render(WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Acme", false), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	if got := render(true); strings.Contains(got, "border-dashed border-indigo-200 bg-white px-4") || !strings.Contains(got, "border-t") {
		t.Errorf("list form should be a divided row, got %s", got)
	}
	if got := render(false); !strings.Contains(got, "rounded-lg border border-dashed border-indigo-200 bg-white px-4") {
		t.Errorf("stand-alone form should keep the dashed card, got %s", got)
	}
}
