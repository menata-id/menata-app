package aiassist

import (
	"strings"
	"testing"
)

func TestGapSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Board View!":                 "board-view",
		"  rekap persen per cabang  ": "rekap-persen-per-cabang",
		"multi__person -- voting":     "multi-person-voting",
		"already-a-slug":              "already-a-slug",
		"!!!":                         "",
		"":                            "",
		strings.Repeat("ab-", 40):     strings.TrimRight(strings.Repeat("ab-", 40)[:64], "-"),
	} {
		if got := GapSlug(in); got != want {
			t.Errorf("GapSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

// Gaps repairs a key, a class and a kind the model got wrong instead of losing the gap, and never
// invents one: a missing key falls back to the name, an unusable name to nothing.
func TestReply_gapsRepairsKeyAndClass(t *testing.T) {
	r := Reply{CapabilityGaps: []CapabilityGap{
		{Requested: "A Board", Kind: GapRuntime, Class: "ui_component", CapabilityKey: "Board View"},
		{Requested: "Rekap Cabang", Kind: GapRuntime, Class: "made-up"},
		{Requested: "???", Kind: GapRuntime},
	}}
	got := r.Gaps()
	if got[0].CapabilityKey != "board-view" || got[0].Class != ClassUIComponent {
		t.Errorf("gap 0 = %+v", got[0])
	}
	if got[1].CapabilityKey != "rekap-cabang" || got[1].Class != ClassUnclassified {
		t.Errorf("gap 1 = %+v", got[1])
	}
	if got[2].CapabilityKey != "" {
		t.Errorf("gap 2 key = %q, want none invented", got[2].CapabilityKey)
	}
}

// The prompt shows every gap not yet built, by key, and no built one; the response schema makes the
// ideal capability and the class required, with the classes the register uses.
func TestPromptShowsOpenGapsAndTheSchemaRequiresTheIdealCapability(t *testing.T) {
	prompt := SystemPromptFor(nil, nil)
	for _, g := range KnownGaps {
		has := strings.Contains(prompt, "- "+g.Key+" (")
		if g.Status == GapBuilt && has {
			t.Errorf("the prompt still lists built gap %q as missing", g.Key)
		}
		if g.Status != GapBuilt && !has {
			t.Errorf("the prompt does not list open gap %q, so the model cannot reuse its key", g.Key)
		}
	}
	required := strings.Join(capabilityGapSchema.Required, ",")
	for _, f := range []string{"capability_key", "ideal_capability", "class"} {
		if !strings.Contains(required, f) {
			t.Errorf("capabilityGapSchema does not require %q", f)
		}
	}
	if got := capabilityGapSchema.Properties["class"].Enum; strings.Join(got, ",") != strings.Join(GapClasses, ",") {
		t.Errorf("class enum = %v, want GapClasses %v", got, GapClasses)
	}
}

func TestValidateKnownGaps(t *testing.T) {
	cap := func(id string) (bool, bool) {
		switch id {
		case "writable":
			return true, true
		case "unwritable":
			return true, false
		}
		return false, false
	}
	ok := KnownGap{Key: "a-b", Class: ClassDataModel, Summary: "s", Status: GapOpen}
	bad := map[string]KnownGap{
		"not a slug":            {Key: "A B", Class: ClassDataModel, Summary: "s", Status: GapOpen},
		"closed set":            {Key: "a-b", Class: "nope", Summary: "s", Status: GapOpen},
		"no summary":            {Key: "a-b", Class: ClassDataModel, Status: GapOpen},
		"not open, planned or":  {Key: "a-b", Class: ClassDataModel, Summary: "s", Status: "done"},
		"only a built gap does": {Key: "a-b", Class: ClassDataModel, Summary: "s", Status: GapOpen, Capability: "writable"},
		"built but names no":    {Key: "a-b", Class: ClassDataModel, Summary: "s", Status: GapBuilt},
		"does not list":         {Key: "a-b", Class: ClassDataModel, Summary: "s", Status: GapBuilt, Capability: "missing"},
		"is not AIWritable":     {Key: "a-b", Class: ClassDataModel, Summary: "s", Status: GapBuilt, Capability: "unwritable"},
	}
	for want, g := range bad {
		found := false
		for _, p := range ValidateKnownGaps([]KnownGap{g}, cap) {
			found = found || strings.Contains(p, want)
		}
		if !found {
			t.Errorf("a gap with %q was not reported: %+v", want, g)
		}
	}
	if p := ValidateKnownGaps([]KnownGap{ok, {Key: "c", Class: ClassUIComponent, Summary: "s", Status: GapBuilt, Capability: "writable"}}, cap); len(p) != 0 {
		t.Errorf("a well-formed register reported %v", p)
	}
	if p := ValidateKnownGaps([]KnownGap{ok, ok}, cap); len(p) != 1 || !strings.Contains(p[0], "twice") {
		t.Errorf("a repeated key reported %v, want one 'twice'", p)
	}
}
