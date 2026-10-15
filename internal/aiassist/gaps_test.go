package aiassist

import (
	"encoding/json"
	"strings"
	"testing"
)

// A stored turn from before WG holds one gap under "capability_gap"; it must still decode, as an
// unclassified gap, never be dropped and never be given a kind nobody assigned.
func TestReply_gapsDecodesTheLegacySingleGap(t *testing.T) {
	var r Reply
	if err := json.Unmarshal([]byte(`{"message":"m","capability_gap":{"requested":"voting","note":"n"}}`), &r); err != nil {
		t.Fatal(err)
	}
	g := r.Gaps()
	if len(g) != 1 || g[0].Requested != "voting" || g[0].Kind != GapUnclassified {
		t.Errorf("legacy gap = %+v, want one unclassified gap named voting", g)
	}
}

// A kind the model got wrong must not lose the gap (the table's CHECK would refuse the insert and
// the whole turn with it): it is stored as unclassified.
func TestReply_gapsNormalisesAnUnknownKind(t *testing.T) {
	r := Reply{CapabilityGaps: []CapabilityGap{
		{Requested: "a", Kind: GapRuntime}, {Requested: "b", Kind: GapAssistant}, {Requested: "c", Kind: "oops"}, {Requested: "d"},
	}}
	want := []string{GapRuntime, GapAssistant, GapUnclassified, GapUnclassified}
	for i, g := range r.Gaps() {
		if g.Kind != want[i] {
			t.Errorf("gap %q kind = %q, want %q", g.Requested, g.Kind, want[i])
		}
	}
}

// The response schema must let a reply carry a change and several gaps, and demand a kind.
func TestReplySchema_carriesManyGapsEachWithAKind(t *testing.T) {
	gaps, ok := replySchema.Properties["capability_gaps"]
	if !ok || gaps.Type != "ARRAY" || gaps.Items == nil {
		t.Fatalf("capability_gaps = %+v, want an array of gap objects", gaps)
	}
	if _, old := replySchema.Properties["capability_gap"]; old {
		t.Error("the single capability_gap key is still in the response schema")
	}
	kind := gaps.Items.Properties["kind"]
	if len(kind.Enum) != 2 || kind.Enum[0] != GapRuntime || kind.Enum[1] != GapAssistant {
		t.Errorf("kind enum = %v, want runtime and assistant", kind.Enum)
	}
	required := strings.Join(gaps.Items.Required, ",")
	if !strings.Contains(required, "kind") {
		t.Errorf("required = %q, want kind among them", required)
	}
}

// The prompt is the only thing that makes the model record a gap, so the triggers it states are
// held: every decline or substitution (not only the NEVER list), alongside a change, with a kind.
// This checks the instruction is present; it cannot check the model obeys it (the log shows that).
func TestPrompt_instructsEveryDeclineToBeRecordedAlongsideAChange(t *testing.T) {
	p := composableSurface()
	full := SystemPromptFor(nil, nil)
	for _, want := range []string{
		"capability_gaps",
		"EVERY part of the request you decline, replace with a different approach",
		"A reply may carry a change and capability_gaps together",
		`"runtime"`, `"assistant"`,
	} {
		if !strings.Contains(p+full, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
	if strings.Contains(p+full, "mutually exclusive") {
		t.Error("the prompt still calls a change and a gap mutually exclusive")
	}
}
