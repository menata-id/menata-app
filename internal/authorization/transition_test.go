package authorization

import (
	"testing"

	"menata.app/internal/domain"
)

func TestAllowsTransition(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", ApplicationID: "app_x"}
	values := map[string]any{"fld_manager": "usr_sari"}
	holding := func(id string, roles ...string) domain.Actor {
		return domain.Actor{ID: id, Roles: map[string][]string{"app_x": roles}}
	}
	for _, tc := range []struct {
		name  string
		edge  domain.Transition
		actor domain.Actor
		want  bool
	}{
		{"an edge that says nothing admits anyone", domain.Transition{}, domain.Actor{}, true},
		{"a role the actor holds", domain.Transition{Roles: []string{"manajer"}}, holding("usr_a", "manajer"), true},
		{"any one of several roles", domain.Transition{Roles: []string{"manajer", "hr"}}, holding("usr_a", "hr"), true},
		{"a role the actor lacks", domain.Transition{Roles: []string{"manajer"}}, holding("usr_a", "karyawan"), false},
		{"a role held in another application", domain.Transition{Roles: []string{"manajer"}}, domain.Actor{ID: "usr_a", Roles: map[string][]string{"app_y": {"manajer"}}}, false},
		{"the person the record names", domain.Transition{ActorField: "fld_manager"}, holding("usr_sari"), true},
		{"someone else", domain.Transition{ActorField: "fld_manager"}, holding("usr_budi"), false},
		{"an empty field admits no one", domain.Transition{ActorField: "fld_none"}, holding("usr_sari"), false},
		{"both arms must hold: role missing", domain.Transition{Roles: []string{"manajer"}, ActorField: "fld_manager"}, holding("usr_sari", "karyawan"), false},
		{"both arms must hold: person missing", domain.Transition{Roles: []string{"manajer"}, ActorField: "fld_manager"}, holding("usr_budi", "manajer"), false},
		{"both arms hold", domain.Transition{Roles: []string{"manajer"}, ActorField: "fld_manager"}, holding("usr_sari", "manajer"), true},
		{"an unidentified actor on a gated edge", domain.Transition{Roles: []string{"manajer"}}, domain.Actor{}, false},
	} {
		if got := AllowsTransition(m, tc.edge, values, tc.actor); got != tc.want {
			t.Errorf("%s: AllowsTransition = %v, want %v", tc.name, got, tc.want)
		}
	}
}
