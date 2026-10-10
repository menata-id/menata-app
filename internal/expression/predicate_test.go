package expression

import "testing"

func TestPredicate_requiresEveryComparison(t *testing.T) {
	p := &Predicate{All: []Comparison{
		{Field: "fld_assignee", Op: OpEquals, Value: "usr_1"},
		{Field: "fld_status", Op: OpNotEquals, Value: "done"},
	}}

	cases := []struct {
		name   string
		values map[string]any
		want   bool
	}{
		{"both hold", map[string]any{"fld_assignee": "usr_1", "fld_status": "todo"}, true},
		{"first fails", map[string]any{"fld_assignee": "usr_2", "fld_status": "todo"}, false},
		{"second fails", map[string]any{"fld_assignee": "usr_1", "fld_status": "done"}, false},
		{"neither holds", map[string]any{"fld_assignee": "usr_2", "fld_status": "done"}, false},
	}
	for _, c := range cases {
		if got := p.Evaluate(c.values); got != c.want {
			t.Errorf("%s: Evaluate = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPredicate_emptyAndNilSelectEverything: "no filter" has to mean "everything", and a nil
// receiver has to behave like an empty one so a caller holding *Predicate need not branch.
func TestPredicate_emptyAndNilSelectEverything(t *testing.T) {
	var nilPredicate *Predicate
	if !nilPredicate.Evaluate(map[string]any{"fld_x": "y"}) {
		t.Error("a nil Predicate rejected a record; no filter must select everything")
	}
	if !(&Predicate{}).Evaluate(map[string]any{"fld_x": "y"}) {
		t.Error("an empty Predicate rejected a record")
	}
	if got := nilPredicate.Comparisons(); got != nil {
		t.Errorf("nil Predicate.Comparisons() = %v, want nil", got)
	}
}

// TestPredicate_oneComparisonBehavesExactlyLikeTheLeaf is the compatibility claim 007 §7.7 makes:
// the existing form stays valid, so wrapping a single comparison must not change its answer.
func TestPredicate_oneComparisonBehavesExactlyLikeTheLeaf(t *testing.T) {
	leaf := Comparison{Field: "fld_status", Op: OpNotEquals, Value: "done"}
	wrapped := &Predicate{All: []Comparison{leaf}}

	for _, values := range []map[string]any{
		{"fld_status": "done"}, {"fld_status": "todo"}, {},
	} {
		if leaf.Evaluate(values) != wrapped.Evaluate(values) {
			t.Errorf("values %v: wrapping one comparison changed its answer", values)
		}
	}
}

// TestPredicate_anyNeedsOneAlternativeBesideEveryConjunct is My Tasks' "Later" bucket in miniature: open, and
// either undated or due after the window. The undated cases are the ones worth pinning -- a Field that was
// never set (nil) and one stored as nothing ("") are both empty, and an ordered comparison is false for both.
func TestPredicate_anyNeedsOneAlternativeBesideEveryConjunct(t *testing.T) {
	p := &Predicate{
		All: []Comparison{{Field: "fld_status", Op: OpNotEquals, Value: "done"}},
		Any: []Comparison{
			{Field: "fld_due", Op: OpIsEmpty},
			{Field: "fld_due", Op: OpGreaterThan, Value: "2026-10-17"},
		},
	}
	cases := []struct {
		name   string
		values map[string]any
		want   bool
	}{
		{"late and open", map[string]any{"fld_status": "todo", "fld_due": "2026-10-20"}, true},
		{"never dated and open", map[string]any{"fld_status": "todo"}, true},
		{"dated nothing and open", map[string]any{"fld_status": "todo", "fld_due": ""}, true},
		{"soon and open", map[string]any{"fld_status": "todo", "fld_due": "2026-10-12"}, false},
		{"undated but done", map[string]any{"fld_status": "done"}, false},
		{"late but done", map[string]any{"fld_status": "done", "fld_due": "2026-10-20"}, false},
	}
	for _, c := range cases {
		if got := p.Evaluate(c.values); got != c.want {
			t.Errorf("%s: Evaluate = %v, want %v", c.name, got, c.want)
		}
	}
	if got := len(p.Comparisons()); got != 3 {
		t.Errorf("Comparisons() = %d leaves, want 3 (All then Any)", got)
	}
}

func TestPredicate_anyAloneNeedsOne(t *testing.T) {
	p := &Predicate{Any: []Comparison{
		{Field: "fld_a", Op: OpEquals, Value: "x"},
		{Field: "fld_b", Op: OpEquals, Value: "y"},
	}}
	if !p.Evaluate(map[string]any{"fld_b": "y"}) {
		t.Error("the second alternative holds, so the predicate must")
	}
	if p.Evaluate(map[string]any{"fld_a": "n", "fld_b": "n"}) {
		t.Error("no alternative holds, so the predicate must not")
	}
}

func TestComparison_isEmptyIsTrueOnlyWithoutAValue(t *testing.T) {
	c := Comparison{Field: "fld_x", Op: OpIsEmpty}
	for name, v := range map[string]map[string]any{"absent": {}, "nil": {"fld_x": nil}, "empty": {"fld_x": ""}} {
		if !c.Evaluate(v) {
			t.Errorf("%s: want empty", name)
		}
	}
	for name, v := range map[string]any{"text": "a", "zero": 0, "false": false} {
		if c.Evaluate(map[string]any{"fld_x": v}) {
			t.Errorf("%s: a stored %v is a value, not empty", name, v)
		}
	}
}
