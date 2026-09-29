package expression

// Predicate is a conjunction of Comparisons: every one must hold.
//
// **It wraps Comparison rather than replacing it**, which is why the four existing users --
// Constraint's block_if.condition, a Measure's where, a Schedule's guard and a member-removal block
// -- change by not one line. 007 §7.7 states the reason as a rule: "Existing field/operator filter
// forms remain valid as syntax sugar and may compile to the common expression representation." A
// single comparison stays exactly what it was; this is the representation it compiles into when an
// author writes more than one.
//
// 007 §9 is the argument against the alternative. Expressions are one shared primitive across
// "computed values; constraints; event conditions; view filters; Dataset filters; conditional
// visibility; action values; derived measures", and "a single bounded expression model should
// replace proliferation of independent mini-languages". A second filter syntax for record selection
// would be that proliferation.
//
// **Conjunction only, and that is a measured boundary rather than a first instalment.** Disjunction
// has no case: the site that forced this one needs `assignee == me AND status != done`, and nothing
// in the codebase filters on an OR. Adding `any:` now would be the premature declaration B5 refuses
// -- the same rule that kept KnownActions at one entry and left `now` out of KnownWriteSources.
type Predicate struct {
	All []Comparison
}

// Evaluate reports whether values satisfies every Comparison.
//
// An empty Predicate is true: a Dataset that declares no filter selects everything, which is what
// "no filter" has to mean. The nil receiver behaves the same, so a caller holding *Predicate need
// not branch before asking.
func (p *Predicate) Evaluate(values map[string]any) bool {
	if p == nil {
		return true
	}
	for _, c := range p.All {
		if !c.Evaluate(values) {
			return false
		}
	}
	return true
}

// Comparisons returns the leaves, so a caller lowering this into another representation -- SQL, for
// instance -- walks one list rather than reaching into the struct.
func (p *Predicate) Comparisons() []Comparison {
	if p == nil {
		return nil
	}
	return p.All
}
