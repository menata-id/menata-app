package expression

// Predicate is a conjunction of Comparisons -- every one of All must hold -- and, when Any is declared, at
// least one of Any must hold as well: "all of these, and some of those".
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
// **Disjunction arrived with its first case (2026-10-10, K15).** Until then the boundary was that nothing
// filtered on an OR; My Tasks' "Later" bucket is "open, and either undated or due after next week", which no
// conjunction says. It is one group beside All rather than a nested tree, because that is the shape the case
// has and a recursive boolean language would be the proliferation 007 §9 warns about; a second case that
// needs nesting is the trigger to generalise.
type Predicate struct {
	All []Comparison
	Any []Comparison
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
	if len(p.Any) == 0 {
		return true
	}
	for _, c := range p.Any {
		if c.Evaluate(values) {
			return true
		}
	}
	return false
}

// Comparisons returns every leaf, All then Any, for a caller that asks about the comparisons regardless of how
// they combine -- validation, "does this name $current_user". A caller lowering the predicate into another
// representation (SQL) must keep the two groups apart and reads All and Any itself.
func (p *Predicate) Comparisons() []Comparison {
	if p == nil {
		return nil
	}
	if len(p.Any) == 0 {
		return p.All
	}
	out := make([]Comparison, 0, len(p.All)+len(p.Any))
	out = append(out, p.All...)
	return append(out, p.Any...)
}
