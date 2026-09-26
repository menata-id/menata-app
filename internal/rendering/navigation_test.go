package rendering

import (
	"context"
	"testing"

	"menata.app/internal/domain"
)

func TestRouteByID_survivesHiddenNavGroup(t *testing.T) {
	all := []domain.NavigationItem{
		{ID: "nav_approval_inbox", Label: "Approval Inbox", Route: "/approval-inbox"},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{
		Navigation: []domain.NavigationItem{{ID: "nav_home", Label: "Home", Route: "/home"}},
		Applications: []domain.Application{{
			ID: "app_document_approval", Name: "Document Approval",
			ShowNav:    false,
			Navigation: nil, // show_nav: false -- no menu chrome anywhere
			// ...but every declared item is still resolvable by id:
			AllNavigation: all,
		}},
	}, "Test Workspace", false)

	if got := routeByID(ctx, "nav_approval_inbox"); got != "/approval-inbox" {
		t.Errorf("routeByID(ctx, nav_approval_inbox) with show_nav: false = %q, want /approval-inbox still resolvable", got)
	}
	if got := labelByID(ctx, "nav_approval_inbox"); got != "Approval Inbox" {
		t.Errorf("labelByID(ctx, nav_approval_inbox) with show_nav: false = %q, want the declared label", got)
	}
	// The Workspace's own navigation resolves through the same lookup.
	if got := routeByID(ctx, "nav_home"); got != "/home" {
		t.Errorf("routeByID(ctx, nav_home) = %q, want /home", got)
	}
}

func TestRouteByID_unknownIDPanics(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Test Workspace", false)

	defer func() {
		if recover() == nil {
			t.Error("routeByID(ctx, unknown id) did not panic, want a loud failure on a programmer error, not a silently broken link")
		}
	}()
	routeByID(ctx, "nav_does_not_exist")
}
