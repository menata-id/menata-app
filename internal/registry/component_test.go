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
