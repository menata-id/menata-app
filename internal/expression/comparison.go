package expression

import "fmt"

// Op is a bounded comparison operator. The set is closed and extended deliberately, not
// inferred (007 §14's static-registry seam applied to expressions).
type Op string

const (
	OpEquals    Op = "equals"
	OpNotEquals Op = "not_equals"
)

// KnownOps is the closed set of operators Comparison understands.
var KnownOps = map[Op]bool{
	OpEquals:    true,
	OpNotEquals: true,
}

// Comparison is the entire expression vocabulary Phase 4 (ROADMAP.md) needs: "field equals/
// not_equals value" against a record's own values. It is deterministic, side-effect free, and
// incapable of I/O or arbitrary code execution by construction (007 §9.1) -- there is no parser,
// no variables beyond the given values map, nothing to make unsafe.
//
// A general expression language (arithmetic, boolean combinators, cross-field references) is
// not built until a second, differently-shaped Constraint forces it. Building it now, for one
// rule, would be exactly the "design ahead of a real case" ROADMAP.md's own method rejects.
type Comparison struct {
	Field string
	Op    Op
	Value string
}

// Evaluate reports whether values satisfies the comparison. An unknown Op evaluates to false
// rather than panicking -- Comparison is meant to be validated (internal/metadata) before it is
// ever evaluated.
func (c Comparison) Evaluate(values map[string]any) bool {
	actual := fmt.Sprint(values[c.Field])
	switch c.Op {
	case OpEquals:
		return actual == c.Value
	case OpNotEquals:
		return actual != c.Value
	default:
		return false
	}
}
