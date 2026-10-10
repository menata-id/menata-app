package composition

import (
	"testing"

	"menata.app/internal/domain"
)

var testHub = domain.NavigationItem{ID: "nav_x_settings", Route: "/settings/nav_x_settings", Heading: "Tally settings", Description: "Applies to Tally only.", SettingsHub: true}

func TestApplicationSettingsHub(t *testing.T) {
	withRoles := domain.Application{Roles: []string{"approver", "submitter", "reviewer"}}
	noRoles := domain.Application{}

	cases := []struct {
		name                    string
		app                     domain.Application
		isWorkspaceAdmin        bool
		wantAccess, wantMembers bool
	}{
		{"no roles at all, gates nothing to show", noRoles, true, false, false},
		{"roles declared, plain member", withRoles, false, true, false},
		{"roles declared, workspace admin", withRoles, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ApplicationSettingsHub(c.app, testHub, c.isWorkspaceAdmin)
			if got.ShowAccess != c.wantAccess {
				t.Errorf("ShowAccess = %v, want %v", got.ShowAccess, c.wantAccess)
			}
			if got.ShowMembersAndGroups != c.wantMembers {
				t.Errorf("ShowMembersAndGroups = %v, want %v", got.ShowMembersAndGroups, c.wantMembers)
			}
		})
	}
}

// A fictional Application with no roles and one declared settings_hub_member: no Access section, one
// Configuration row read whole from the item, and an About that carries its own name -- the proof the
// hub names no Application (S2.1's "done means").
func TestApplicationSettingsHub_readsEverythingFromTheApplication(t *testing.T) {
	app := domain.Application{
		Name: "Tally", Description: "Counts things.", Machines: []string{"mch_a", "mch_b"},
		AllNavigation: []domain.NavigationItem{
			testHub,
			{ID: "nav_other", Route: "/other", Label: "Other"},
			{ID: "nav_units", Route: "/machines/mch_unit", Label: "Units", Description: "What a count is measured in.", SettingsHubMember: true},
		},
	}
	got := ApplicationSettingsHub(app, testHub, true)
	if got.ShowAccess || got.ShowMembersAndGroups {
		t.Errorf("an Application with no roles shows Access = %v / members = %v, want neither", got.ShowAccess, got.ShowMembersAndGroups)
	}
	if len(got.Members) != 1 || got.Members[0].Href != "/machines/mch_unit" || got.Members[0].Label != "Units" || got.Members[0].Description != "What a count is measured in." {
		t.Errorf("Members = %+v, want exactly the one settings_hub_member item read whole", got.Members)
	}
	if got.Title != "Tally settings" || got.HubRoute != "/settings/nav_x_settings" || got.AppName != "Tally" || got.MachineCount != 2 || got.RoleCount != 0 {
		t.Errorf("hub identity not read from the Application: %+v", got)
	}
}

func TestSettingsHubOf_findsOnlyAHubWithThatId(t *testing.T) {
	app := domain.Application{AllNavigation: []domain.NavigationItem{testHub, {ID: "nav_plain", Route: "/p"}}}
	if _, ok := SettingsHubOf(app, "nav_x_settings"); !ok {
		t.Error("the hub item was not found by its id")
	}
	if _, ok := SettingsHubOf(app, "nav_plain"); ok {
		t.Error("an item that is not marked settings_hub was found as a hub")
	}
	if _, ok := SettingsHubOf(app, "nav_missing"); ok {
		t.Error("an id naming nothing was found")
	}
}
