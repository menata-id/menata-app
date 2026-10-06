package registry

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestValidateComponentUse covers the validator the catalogue carries, including the arm that exists for
// §12.3's own sentence about arbitrary properties rather than for symmetry.
func TestValidateComponentUse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		typ    domain.ComponentType
		inputs map[string]string
		want   string // substring the issues must contain; "" means no issue at all
	}{
		{
			name:   "a complete use passes",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "OVERDUE", "tone": string(domain.ToneBad)},
		},
		{
			name:   "an unset size is allowed, because unset means regular",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "3", "tone": string(domain.ToneNeutral), "size": string(domain.BadgeCompact)},
		},
		{
			name:   "a size outside the closed set is refused",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "3", "tone": string(domain.ToneNeutral), "size": "jumbo"},
			want:   `size "jumbo" is not one of the declared sizes`,
		},
		{
			name:   "a missing required input is named",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"tone": string(domain.ToneGood)},
			want:   `requires input "label"`,
		},
		{
			name:   "a tone outside the closed set is refused",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "ok", "tone": "chartreuse"},
			want:   `tone "chartreuse" is not one of the declared tones`,
		},
		{
			// The arm that matters for §12.3: an undeclared input is how a bounded Component starts
			// becoming the unbounded GenericComponent that section forbids by name.
			name:   "an undeclared input is refused even when everything required is present",
			typ:    domain.ComponentStatusBadge,
			inputs: map[string]string{"label": "ok", "tone": string(domain.ToneGood), "icon": "clock"},
			want:   `no declared input "icon"`,
		},
		{
			name:   "an unregistered type is refused rather than resolved",
			typ:    domain.ComponentType("SparklineCard"),
			inputs: map[string]string{},
			want:   `"SparklineCard" is not one this runtime realizes`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := ValidateComponentUse(tc.typ, tc.inputs)
			joined := strings.Join(issues, " | ")
			if tc.want == "" {
				if len(issues) != 0 {
					t.Fatalf("ValidateComponentUse() = %v, want no issues", issues)
				}
				return
			}
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("ValidateComponentUse() = %v, want an issue containing %q", issues, tc.want)
			}
		})
	}
}

// TestStatusBadgeContractDeclaresNoDataRequirements asserts the one field the whole slice rests on, rather
// than leaving it to be read.
//
// A `StatusBadge` with a data requirement would be allowed to go and fetch something, which is what
// `rendering.slaBadgePill` did by parsing a date and calling `experience.EvaluateSLA(due, time.Now())`. The
// contract declaring none is what makes `conformance.TestRegisteredComponentsStayBounded` able to refuse a
// renderer signature that takes a record, an `any`, or the clock.
func TestStatusBadgeContractDeclaresNoDataRequirements(t *testing.T) {
	c := Components[domain.ComponentStatusBadge].Contract
	if len(c.DataRequirements) != 0 {
		t.Errorf("StatusBadge declares data requirements %v -- a badge is handed a resolved label and tone; resolution belongs in internal/composition", c.DataRequirements)
	}
	if len(c.Slots) != 0 || len(c.Actions) != 0 {
		t.Errorf("StatusBadge declares slots %v and actions %v -- it is a display-only leaf", c.Slots, c.Actions)
	}
	if c.Accessibility == "" {
		t.Error("StatusBadge declares no accessibility semantics -- §13 lists them, and \"the text is the name\" is an answer worth writing down so the next reader does not add a role it should not have")
	}
}

// TestAvatarContractRequiresAnAccessibleName asserts the clause the second Component existed to find.
//
// `StatusBadge`'s accessibility clause describes an absence -- its text is its name. An avatar's content is
// initials, which are not a name, so `label` is a **required input** rather than an optional nicety. The four
// hand-written sites it replaced carried no accessible name at all.
func TestAvatarContractRequiresAnAccessibleName(t *testing.T) {
	var label *domain.ComponentInput
	for i, in := range Components[domain.ComponentAvatar].Contract.Inputs {
		if in.Name == "label" {
			label = &Components[domain.ComponentAvatar].Contract.Inputs[i]
		}
	}
	if label == nil {
		t.Fatal("Avatar declares no `label` input -- the initials are not an accessible name, so a Component that cannot be given one renders a circle a screen reader announces as two letters")
	}
	if !label.Required {
		t.Error("Avatar's `label` is optional -- an accessibility requirement a caller may skip is not a requirement")
	}
	if issues := ValidateComponentUse(domain.ComponentAvatar, map[string]string{
		"initials": "AP", "size": string(domain.AvatarInline), "presence": string(domain.AvatarPresent),
	}); len(issues) == 0 {
		t.Error("an Avatar use with no label passed validation")
	}
}

// TestBothComponentsShareOneValidatorShape is the answer to the Stage 2 plan's own question -- does the
// contract have to change to hold a second member?
//
// **Structurally it did not**, and that is worth asserting rather than implying: `Inputs` is a slice, so four
// fit where two did, and both validators derive their required/undeclared checks from the declaration through
// one shared helper. A second validator that invented its own conventions would make the catalogue two
// catalogues, which is the failure this checks for.
func TestBothComponentsShareOneValidatorShape(t *testing.T) {
	for typ := range Components {
		undeclared := ValidateComponentUse(typ, map[string]string{"notAThing": "x"})
		var found bool
		for _, s := range undeclared {
			if strings.Contains(s, `no declared input "notAThing"`) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not refuse an undeclared input -- every Component must, or boundedness is per-Component taste (007 §12.3)", typ)
		}
	}
	if len(Components) < 2 {
		t.Fatalf("this gate compares Components against each other and the catalogue holds %d", len(Components))
	}
}

// A Metric's optional hint and tone are declared in its contract, and a tone outside the closed set is
// refused -- the same posture StatusBadge takes, since both hand a tone to the Theme's palette lookup.
func TestMetricAcceptsHintAndToneButOnlyDeclaredTones(t *testing.T) {
	if issues := ValidateComponentUse(domain.ComponentMetric, map[string]string{
		"label": "Overdue", "value": "1", "hint": "Past its due date", "tone": string(domain.ToneBad),
	}); len(issues) != 0 {
		t.Errorf("a Metric with hint and a declared tone was refused: %v", issues)
	}
	if issues := ValidateComponentUse(domain.ComponentMetric, map[string]string{
		"label": "Overdue", "value": "1", "tone": "chartreuse",
	}); len(issues) == 0 {
		t.Error("a Metric with an undeclared tone passed validation")
	}
}

// TestValidateButton holds the contract's one cross-input rule and its closed set. `name` and `value` are the
// native pair a submit button posts, and half a pair is a defect with a different symptom each way: a name with
// no value posts an empty string the handler cannot tell from "not clicked", and a value with no name is
// dropped by the browser. A validator that only checked required inputs would pass both.
func TestValidateButton(t *testing.T) {
	ok := map[string]string{"label": "Save", "variant": "primary"}
	if issues := ValidateComponentUse(domain.ComponentButton, ok); len(issues) != 0 {
		t.Errorf("a label and a variant is a valid Button, got %v", issues)
	}
	pair := map[string]string{"label": "Reject", "variant": "danger", "name": "decision", "value": "rejected"}
	if issues := ValidateComponentUse(domain.ComponentButton, pair); len(issues) != 0 {
		t.Errorf("a complete name/value pair is valid, got %v", issues)
	}
	for name, in := range map[string]map[string]string{
		"name without value": {"label": "x", "variant": "primary", "name": "intent"},
		"value without name": {"label": "x", "variant": "primary", "value": "draft"},
		"unknown variant":    {"label": "x", "variant": "ghost"},
		"no label":           {"variant": "primary"},
		"no variant":         {"label": "x"},
		"hx attribute":       {"label": "x", "variant": "primary", "hx-get": "/x"},
		"class":              {"label": "x", "variant": "primary", "class": "mr-1"},
	} {
		if issues := ValidateComponentUse(domain.ComponentButton, in); len(issues) == 0 {
			t.Errorf("%s must be refused, but validated clean", name)
		}
	}
}
