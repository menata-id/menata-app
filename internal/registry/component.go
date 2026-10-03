package registry

import (
	"fmt"

	"menata.app/internal/domain"
)

// Component is one member of the Component Registry seam 007 §14 describes: an identity, the contract it
// declares (§13), and the validator that checks a use of it.
//
// It mirrors `Service` exactly, including what it leaves out -- the **renderer** stays in
// `internal/rendering`, because this package may import `internal/domain` and nothing else. §14's chain
// (`type → contract → validator → resolver → renderer`) is held across three planes with a conformance gate
// asserting they agree, not in one package. Read `Service`'s own comment for why that is a plane boundary
// rather than a preference.
//
// **What a registry buys that a `templ` function does not**, since a reader is entitled to ask: a closed
// catalogue is the only place a *boundedness* rule can be enforced. §12.3 forbids a Component acquiring
// business data its contract does not represent, and that is unenforceable against an ordinary function --
// any signature is legal, so `slaBadgePill(v any)` parsing a date and calling `EvaluateSLA(due, time.Now())`
// looked exactly like a renderer. Against a contract it is a mismatch a test can state.
type Component struct {
	// Contract is what this Component declares (§13).
	Contract domain.ComponentContract
	// Validate checks one *use* of this Component: the inputs a caller supplies against the ones declared.
	// It returns issues rather than an error, the posture every validator in this tree already has.
	//
	// It is not the boundedness check. Boundedness is structural -- it is about the renderer's own
	// signature, not about one call -- and lives in `conformance.TestRegisteredComponentsStayBounded`.
	Validate func(inputs map[string]string) []string
}

// Components is the closed catalogue. A type absent from it is refused rather than resolved, which is what
// makes this a static seam and not a plugin loader (see doc.go).
//
// **One member, and that is the honest starting size.** §12.3 lists nine example Components and this
// registers the one with a measured population: `statusPill` had 8 call sites plus two more renderers of the
// same idea under different names, one of which was unbounded. The other eight examples are either already
// expressible (a `Metric` is `statusBadge`'s sibling, next) or have no site at all -- and 007 §34 plus this
// repo's own three zero-caller layout primitives are why none of them is registered in advance.
var Components = map[domain.ComponentType]Component{
	domain.ComponentStatusBadge: {Contract: statusBadgeContract, Validate: validateStatusBadge},
}

// statusBadgeContract is declared separately from the catalogue, not for tidiness: `validateStatusBadge`
// reads its own declared inputs rather than retyping them, and a validator reaching back into `Components`
// to find them is an initialization cycle Go rejects outright. Naming the contract is what lets the
// validator be derived from it instead of agreeing with it by hand -- the same reason
// `conformance.TestEveryDerivationIsOwedBySomeRole` reads constants out of the source.
var statusBadgeContract = domain.ComponentContract{
	Type: domain.ComponentStatusBadge,
	Inputs: []domain.ComponentInput{
		{Name: "label", Kind: "string", Required: true},
		{Name: "tone", Kind: "BadgeTone", Required: true},
	},
	// Empty, and this is the load-bearing line of the whole slice. See
	// domain.ComponentContract.DataRequirements.
	DataRequirements: nil,
	Slots:            nil,
	Actions:          nil,
	// Measured rather than invented: no pill in this corpus carries an ARIA attribute, and the text inside
	// the span is the state. Saying so is what stops the next reader adding `role="status"` to a label that
	// is not a live region.
	Accessibility: "the badge's own text is its accessible name; it is not a live region, so it carries no role",
	Renderer:      "statusBadge",
}

// ValidateComponentUse is the entry point: it resolves the type and runs its contract, or reports that the
// runtime realizes no such Component.
func ValidateComponentUse(t domain.ComponentType, inputs map[string]string) []string {
	c, known := Components[t]
	if !known {
		return []string{fmt.Sprintf("component type %q is not one this runtime realizes", t)}
	}
	return c.Validate(inputs)
}

// validateStatusBadge checks a use against the declared inputs: every required one present, no undeclared
// one supplied, and `tone` naming a member of the closed set.
//
// The undeclared-input half is not symmetry for its own sake. An extra input is how a bounded Component
// starts becoming the unbounded `GenericComponent` §12.3 forbids by name -- one caller passes something the
// contract does not mention, the renderer grows a parameter for it, and the contract is now a comment.
func validateStatusBadge(inputs map[string]string) []string {
	var issues []string
	declared := map[string]bool{}
	for _, in := range statusBadgeContract.Inputs {
		declared[in.Name] = true
		if in.Required && inputs[in.Name] == "" {
			issues = append(issues, fmt.Sprintf("StatusBadge requires input %q", in.Name))
		}
	}
	for name := range inputs {
		if !declared[name] {
			issues = append(issues, fmt.Sprintf("StatusBadge has no declared input %q -- a Component does not grow a property to suit one caller (007 §12.3)", name))
		}
	}
	if tone := inputs["tone"]; tone != "" && !domain.KnownBadgeTones[domain.BadgeTone(tone)] {
		issues = append(issues, fmt.Sprintf("StatusBadge tone %q is not one of the declared tones", tone))
	}
	return issues
}
