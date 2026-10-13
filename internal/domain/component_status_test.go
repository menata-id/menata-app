package domain

import (
	"strings"
	"testing"
)

// TestComponentStatusProblems states the contract's own status rules: a status is required and known, and a
// successor belongs to exactly the deprecated state.
func TestComponentStatusProblems(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    ComponentContract
		want string // "" means no problem; otherwise a substring of the single problem
	}{
		{"stable", ComponentContract{Type: "X", Status: ComponentStatusStable}, ""},
		{"experimental", ComponentContract{Type: "X", Status: ComponentStatusExperimental}, ""},
		{"deprecated with a successor", ComponentContract{Type: "X", Status: ComponentStatusDeprecated, ReplacedBy: "Y"}, ""},
		{"no status", ComponentContract{Type: "X"}, "declares no Status"},
		{"unknown status", ComponentContract{Type: "X", Status: "beta"}, `Status "beta"`},
		{"deprecated without a successor", ComponentContract{Type: "X", Status: ComponentStatusDeprecated}, "names no ReplacedBy"},
		{"successor on a stable one", ComponentContract{Type: "X", Status: ComponentStatusStable, ReplacedBy: "Y"}, "only a deprecated Component has a successor"},
		{"successor on an experimental one", ComponentContract{Type: "X", Status: ComponentStatusExperimental, ReplacedBy: "Y"}, "only a deprecated Component has a successor"},
		{"replaced by itself", ComponentContract{Type: "X", Status: ComponentStatusDeprecated, ReplacedBy: "X"}, "its own ReplacedBy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.c.StatusProblems()
			switch {
			case tc.want == "" && len(got) != 0:
				t.Errorf("want no problem, got %v", got)
			case tc.want != "" && (len(got) != 1 || !strings.Contains(got[0], tc.want)):
				t.Errorf("want one problem containing %q, got %v", tc.want, got)
			}
		})
	}
}
