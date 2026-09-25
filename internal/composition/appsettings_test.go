package composition

import (
	"testing"

	"menata.app/internal/domain"
)

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
			got := ApplicationSettingsHub(c.app, c.isWorkspaceAdmin)
			if got.ShowAccess != c.wantAccess {
				t.Errorf("ShowAccess = %v, want %v", got.ShowAccess, c.wantAccess)
			}
			if got.ShowMembersAndGroups != c.wantMembers {
				t.Errorf("ShowMembersAndGroups = %v, want %v", got.ShowMembersAndGroups, c.wantMembers)
			}
		})
	}
}

// An Application declaring no roles can never show Members & roles/Groups either, regardless of
// the viewer's own Workspace role -- there is no access to configure, so an admin sees the same
// nothing a plain member does.
func TestApplicationSettingsHub_noRolesHidesMembersEvenForAdmin(t *testing.T) {
	got := ApplicationSettingsHub(domain.Application{}, true)
	if got.ShowMembersAndGroups {
		t.Error("ShowMembersAndGroups = true, want false -- no roles means nothing to show regardless of who is looking")
	}
}
