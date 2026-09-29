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
