package expression

import "testing"

func TestComparison_Evaluate(t *testing.T) {
	cases := []struct {
		name string
		c    Comparison
		vals map[string]any
		want bool
	}{
		{"equals match", Comparison{Field: "fld_status", Op: OpEquals, Value: "done"}, map[string]any{"fld_status": "done"}, true},
		{"equals mismatch", Comparison{Field: "fld_status", Op: OpEquals, Value: "done"}, map[string]any{"fld_status": "todo"}, false},
		{"not_equals match", Comparison{Field: "fld_status", Op: OpNotEquals, Value: "done"}, map[string]any{"fld_status": "todo"}, true},
		{"not_equals mismatch", Comparison{Field: "fld_status", Op: OpNotEquals, Value: "done"}, map[string]any{"fld_status": "done"}, false},
		{"missing field treated as empty string", Comparison{Field: "fld_status", Op: OpNotEquals, Value: "done"}, map[string]any{}, true},
		{"unknown op is always false", Comparison{Field: "fld_status", Op: "greater_than", Value: "1"}, map[string]any{"fld_status": "2"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.c.Evaluate(c.vals); got != c.want {
				t.Errorf("Evaluate() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestOrdered pins what the four ordered operators mean. The two traps are the reason the function
// exists rather than a `<` on strings: "10" sorts before "9" as text, and "" sorts before every date.
func TestOrdered(t *testing.T) {
	cases := []struct {
		name          string
		op            Op
		actual, value string
		want          bool
	}{
		{"date before", OpLessThan, "2026-10-09", "2026-10-10", true},
		{"date same day is not before", OpLessThan, "2026-10-10", "2026-10-10", false},
		{"date same day is lte", OpLessOrEqual, "2026-10-10", "2026-10-10", true},
		{"date after", OpGreaterThan, "2026-10-11", "2026-10-10", true},
		{"date same day is gte", OpGreaterOrEqual, "2026-10-10", "2026-10-10", true},
		{"a number is not ordered as text", OpGreaterThan, "10", "9", true},
		{"a number below", OpLessThan, "10", "9", false},
		{"decimals", OpLessThan, "2.5", "10", true},
		{"a non-number against a number satisfies nothing", OpLessThan, "abc", "10", false},
		{"empty satisfies nothing", OpLessThan, "", "2026-10-10", false},
		{"empty satisfies not even lte", OpLessOrEqual, "", "2026-10-10", false},
		{"unknown op", Op("between"), "1", "2", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Ordered(c.op, c.actual, c.value); got != c.want {
				t.Errorf("Ordered(%s, %q, %q) = %v, want %v", c.op, c.actual, c.value, got, c.want)
			}
		})
	}
}

// A record that does not declare the Field has no date: it is not "before today". Equality keeps its
// own long-standing answer for an absent Field, which is why the ordered arm checks presence itself.
func TestComparison_OrderedOnAnAbsentFieldIsFalse(t *testing.T) {
	c := Comparison{Field: "fld_due", Op: OpLessThan, Value: "2026-10-10"}
	if c.Evaluate(map[string]any{}) {
		t.Error("an absent Field satisfied `lt`")
	}
	if c.Evaluate(map[string]any{"fld_due": nil}) {
		t.Error("a nil Field satisfied `lt`")
	}
	if !c.Evaluate(map[string]any{"fld_due": "2026-10-01"}) {
		t.Error("a past date did not satisfy `lt` today")
	}
	if !IsOrdered(OpGreaterOrEqual) || IsOrdered(OpEquals) {
		t.Error("IsOrdered misclassifies an operator")
	}
}
