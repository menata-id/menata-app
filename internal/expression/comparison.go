package expression

import (
	"fmt"
	"strconv"
)

// Op is a bounded comparison operator. The set is closed and extended deliberately, not
// inferred (007 §14's static-registry seam applied to expressions).
type Op string

const (
	OpEquals    Op = "equals"
	OpNotEquals Op = "not_equals"

	// OpIsEmpty holds when the Field has no value: never set, or set to nothing. It takes no value, so a
	// comparison declaring one is refused at load. It is the one way to say "undated" or "unassigned"; `equals`
	// with an empty value cannot, because an absent Field is not the empty string (SQL NULL, Go nil).
	OpIsEmpty Op = "is_empty"

	// The four ordered operators. They compare by what the declared value is, not by what the Field is
	// (see Ordered): a number against a number, anything else as text, which orders ISO dates correctly.
	OpLessThan       Op = "lt"
	OpLessOrEqual    Op = "lte"
	OpGreaterThan    Op = "gt"
	OpGreaterOrEqual Op = "gte"
)

// KnownOps is the closed set of operators Comparison understands.
var KnownOps = map[Op]bool{
	OpEquals:    true,
	OpNotEquals: true,
	OpIsEmpty:   true,

	OpLessThan:       true,
	OpLessOrEqual:    true,
	OpGreaterThan:    true,
	OpGreaterOrEqual: true,
}

// IsOrdered reports whether op compares by order rather than by identity.
func IsOrdered(op Op) bool {
	switch op {
	case OpLessThan, OpLessOrEqual, OpGreaterThan, OpGreaterOrEqual:
		return true
	}
	return false
}

// IsNumber reports whether a declared value is compared as a number. It is the one place that decides,
// and the SQL realisation of an ordered Comparison (internal/data) is built to agree with it: a value that
// is a number orders against numbers only, any other orders as text.
func IsNumber(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

// Ordered reports whether actual stands in the relation op to value. An absent or empty actual satisfies
// nothing: a record with no due date is not "before today", and treating "" as the smallest string would
// say it was. When value is a number, actual must be one too -- a non-numeric actual satisfies nothing
// rather than falling back to text, so "10" and "9" are never ordered as strings. Any other value orders
// as text, which is the right order for ISO dates (2026-10-09 < 2026-10-10).
func Ordered(op Op, actual, value string) bool {
	if actual == "" {
		return false
	}
	var cmp int
	if IsNumber(value) {
		a, err := strconv.ParseFloat(actual, 64)
		if err != nil {
			return false
		}
		v, _ := strconv.ParseFloat(value, 64)
		switch {
		case a < v:
			cmp = -1
		case a > v:
			cmp = 1
		}
	} else {
		switch {
		case actual < value:
			cmp = -1
		case actual > value:
			cmp = 1
		}
	}
	switch op {
	case OpLessThan:
		return cmp < 0
	case OpLessOrEqual:
		return cmp <= 0
	case OpGreaterThan:
		return cmp > 0
	case OpGreaterOrEqual:
		return cmp >= 0
	}
	return false
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
	if c.Op == OpIsEmpty {
		v := values[c.Field]
		return v == nil || fmt.Sprint(v) == ""
	}
	actual := fmt.Sprint(values[c.Field])
	switch c.Op {
	case OpEquals:
		return actual == c.Value
	case OpNotEquals:
		return actual != c.Value
	case OpLessThan, OpLessOrEqual, OpGreaterThan, OpGreaterOrEqual:
		if v := values[c.Field]; v == nil {
			return false
		}
		return Ordered(c.Op, actual, c.Value)
	default:
		return false
	}
}
