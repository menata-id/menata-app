package composition

import (
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

var testGroup = domain.ViewGroup{ID: "vg", Scope: &domain.ViewScope{Field: "fld_project", Parameter: "project"}}

func TestCarryScope(t *testing.T) {
	for name, tc := range map[string]struct {
		group  domain.ViewGroup
		params map[string]string
		href   string
		want   string
	}{
		"a choice travels":             {testGroup, map[string]string{"project": "rec_1"}, "/pages/nav_table", "/pages/nav_table?project=rec_1"},
		"beside the week":              {testGroup, map[string]string{"project": "rec_1"}, "/pages/nav_cal?week=1", "/pages/nav_cal?project=rec_1&week=1"},
		"no choice leaves the address": {testGroup, map[string]string{}, "/pages/nav_table", "/pages/nav_table"},
		"a link that already chose":    {testGroup, map[string]string{"project": "rec_1"}, "/pages/nav_table?project=rec_2", "/pages/nav_table?project=rec_2"},
		"a group with no scope":        {domain.ViewGroup{ID: "vg"}, map[string]string{"project": "rec_1"}, "/pages/nav_table", "/pages/nav_table"},
		"an unusual value is escaped":  {testGroup, map[string]string{"project": "a&b=c"}, "/x", "/x?project=a%26b%3Dc"},
	} {
		if got := carryScope(tc.group, tc.params, tc.href); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

// TestViewCarry_isForTheGroupsOwnScreens: a link to another screen of the group keeps the choice; a link anywhere else does not take it.
func TestViewCarry_isForTheGroupsOwnScreens(t *testing.T) {
	app := domain.Application{AllNavigation: []domain.NavigationItem{{ID: "nav_a", ViewGroup: "vg"}, {ID: "nav_other"}}}
	carry := ViewCarry(app, testGroup, map[string]string{"project": "rec_1"})
	if got := carry("nav_a", "/a"); got != "/a?project=rec_1" {
		t.Errorf("to a screen of the group: %q", got)
	}
	if got := carry("nav_other", "/o"); got != "/o" {
		t.Errorf("to a screen outside it: %q", got)
	}
	if ViewCarry(app, domain.ViewGroup{ID: "vg"}, nil) != nil {
		t.Error("a group with no scope carries nothing, so it needs no hook")
	}
}

func TestScopeRecords(t *testing.T) {
	rec := func(id, project string) *data.Record {
		return &data.Record{ID: id, Values: map[string]any{"fld_project": project}}
	}
	all := []*data.Record{rec("a", "p1"), rec("b", "p2"), rec("c", ""), rec("d", "p1")}
	if got := ScopeRecords(testGroup, map[string]string{"project": "p1"}, all); len(got) != 2 || got[0].ID != "a" || got[1].ID != "d" {
		t.Errorf("a chosen project: %v", got)
	}
	if got := ScopeRecords(testGroup, map[string]string{}, all); len(got) != 4 {
		t.Errorf("no choice shows every task, those with no project too: %d", len(got))
	}
	if got := ScopeRecords(domain.ViewGroup{}, map[string]string{"project": "p1"}, all); len(got) != 4 {
		t.Errorf("a group with no scope narrows nothing: %d", len(got))
	}
}
