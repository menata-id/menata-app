package data

import (
	"reflect"
	"testing"
)

// TestEffectiveRoles covers the cases CAP-O07 and board 05 between them name. It needs no
// database: the union is a read-time merge over two single-valued sources, which is the whole
// point of the function existing separately from either table.
func TestEffectiveRoles(t *testing.T) {
	group := func(name string, grants map[string]string) Group {
		return Group{ID: "grp_" + name, Name: name, Grants: grants}
	}

	tests := []struct {
		name   string
		direct map[string]string
		groups []Group
		want   map[string][]string
	}{
		{
			name:   "direct only",
			direct: map[string]string{"app_a": "approver"},
			want:   map[string][]string{"app_a": {"approver"}},
		},
		{
			name:   "group only",
			groups: []Group{group("Reviewers", map[string]string{"app_a": "reviewer"})},
			want:   map[string][]string{"app_a": {"reviewer"}},
		},
		{
			// "Holding a role through either path grants it identically" (CAP-O07), so the same
			// role from both sources is one role, not two.
			name:   "both grant the same role",
			direct: map[string]string{"app_a": "approver"},
			groups: []Group{group("Reviewers", map[string]string{"app_a": "approver"})},
			want:   map[string][]string{"app_a": {"approver"}},
		},
		{
			// Board 05's own case: direct Procurement=Buyer while the Reviewers group grants
			// Procurement=Reviewer. The set holds both -- no precedence rule, because neither
			// CAP-O07 nor the board states one.
			name:   "both grant different roles",
			direct: map[string]string{"app_proc": "buyer"},
			groups: []Group{group("Reviewers", map[string]string{"app_proc": "reviewer"})},
			want:   map[string][]string{"app_proc": {"buyer", "reviewer"}},
		},
		{
			// Upstream's schema comment names this explicitly as how a person ends up with
			// several roles in one Application.
			name: "two groups granting different roles",
			groups: []Group{
				group("Reviewers", map[string]string{"app_a": "reviewer"}),
				group("Approvers", map[string]string{"app_a": "approver"}),
			},
			want: map[string][]string{"app_a": {"reviewer", "approver"}},
		},
		{
			name:   "empty role is not a role",
			direct: map[string]string{"app_a": ""},
			groups: []Group{group("Reviewers", map[string]string{"app_b": ""})},
			want:   map[string][]string{},
		},
		{
			name:   "roles in different applications stay separate",
			direct: map[string]string{"app_a": "approver"},
			groups: []Group{group("Reviewers", map[string]string{"app_b": "reviewer"})},
			want:   map[string][]string{"app_a": {"approver"}, "app_b": {"reviewer"}},
		},
		{
			name: "nothing at all",
			want: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveRoles(tt.direct, tt.groups)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EffectiveRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
