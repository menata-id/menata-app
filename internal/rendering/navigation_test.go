package rendering

import (
	"testing"

	"menata.app/internal/domain"
)

func TestNavSections_splitsUngroupedPrimaryAndRest(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_dashboard", Label: "Dashboard", Route: "/dashboard", Group: "Document Approval", Priority: 1},
		{ID: "nav_my_tasks", Label: "My Tasks", Route: "/my-tasks", Group: "Project Management", Priority: 1},
	}

	ungrouped, primary, rest := navSections(items)

	if len(ungrouped) != 1 || ungrouped[0].ID != "nav_home" {
		t.Errorf("ungrouped = %+v, want [nav_home]", ungrouped)
	}
	if primary.Label != "Document Approval" || len(primary.Items) != 1 || primary.Items[0].ID != "nav_dashboard" {
		t.Errorf("primary = %+v, want Document Approval containing nav_dashboard", primary)
	}
	if len(rest) != 1 || rest[0].Label != "Project Management" {
		t.Errorf("rest = %+v, want [Project Management]", rest)
	}
}

func TestNavSections_noNamedGroupsLeavesPrimaryAndRestEmpty(t *testing.T) {
	items := []domain.NavigationItem{{ID: "nav_home", Label: "Home", Route: "/home"}}

	ungrouped, primary, rest := navSections(items)

	if len(ungrouped) != 1 {
		t.Errorf("ungrouped = %+v, want 1 item", ungrouped)
	}
	if primary.Label != "" || len(primary.Items) != 0 {
		t.Errorf("primary = %+v, want zero value", primary)
	}
	if len(rest) != 0 {
		t.Errorf("rest = %+v, want empty", rest)
	}
}
