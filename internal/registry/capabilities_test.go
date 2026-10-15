package registry

import (
	"strings"
	"testing"
)

func TestCatalogueIsSelfConsistent(t *testing.T) {
	if p := CapabilityProblems(Capabilities); len(p) != 0 {
		t.Fatalf("the catalogue breaks its own rules: %v", p)
	}
}

// Each rule is fed a row that breaks only it, so a rule that stopped biting is seen.
func TestCapabilityProblemsNamesEachBrokenRule(t *testing.T) {
	ok := Capability{ID: "a.x", Struct: "a", Keys: []string{"x"}, Stage: StageSupported, AIWritable: true}
	prop := Capability{ID: "p", Struct: "a", Keys: []string{"y"}, Stage: StageProposed, Cases: []string{"c"}}
	cases := []struct {
		name string
		rows []Capability
		want string
	}{
		{"no id", []Capability{{Struct: "a", Keys: []string{"x"}}}, "no id"},
		{"duplicate id", []Capability{ok, ok}, "listed twice"},
		{"no key", []Capability{{ID: "q", Struct: "a", Stage: StageSupported, AIWritable: true}}, "no struct or no key"},
		{"key claimed twice", []Capability{ok, {ID: "b", Struct: "a", Keys: []string{"x"}, Stage: StageSupported, AIWritable: true}}, "claimed by both"},
		{"unwritable without reason", []Capability{{ID: "q", Struct: "a", Keys: []string{"z"}, Stage: StageSupported}}, "no reason is given"},
		{"writable with reason", []Capability{{ID: "q", Struct: "a", Keys: []string{"z"}, Stage: StageSupported, AIWritable: true, AIReason: "r"}}, "carries a reason"},
		{"supported with cases", []Capability{{ID: "q", Struct: "a", Keys: []string{"z"}, Stage: StageSupported, AIWritable: true, Cases: []string{"c"}}}, "still names corpus cases"},
		{"proposed writable", []Capability{{ID: "q", Struct: "a", Keys: []string{"z"}, Stage: StageProposed, AIWritable: true, Cases: []string{"c"}}}, "cannot already be writable"},
		{"proposed without case", []Capability{{ID: "q", Struct: "a", Keys: []string{"z"}, Stage: StageProposed}}, "names no case"},
		{"unknown stage", []Capability{{ID: "q", Struct: "a", Keys: []string{"z"}, Stage: "maybe"}}, "unknown stage"},
	}
	if p := CapabilityProblems([]Capability{ok, prop}); len(p) != 0 {
		t.Fatalf("a sound catalogue was refused: %v", p)
	}
	for _, c := range cases {
		got := strings.Join(CapabilityProblems(c.rows), "; ")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want a problem containing %q, got %q", c.name, c.want, got)
		}
	}
}
