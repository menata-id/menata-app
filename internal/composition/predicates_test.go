package composition

import (
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// predicatesFor keeps the conjunction and the OR-group apart: the store joins Alternative predicates with OR and
// ANDs the result with the rest, so a predicate on the wrong side changes which records a Dataset lists.
func TestPredicatesFor_marksOnlyAnyAsAlternativeAndMapsIsEmpty(t *testing.T) {
	ds := domain.Dataset{ID: "ds_x", Where: &expression.Predicate{
		All: []expression.Comparison{
			{Field: "fld_assignee", Op: expression.OpEquals, Value: expression.SentinelCurrentUser},
			{Field: "fld_status", Op: expression.OpNotEquals, Value: "done"},
		},
		Any: []expression.Comparison{
			{Field: "fld_due", Op: expression.OpIsEmpty},
			{Field: "fld_due", Op: expression.OpGreaterThan, Value: "$today+7"},
		},
	}}
	got, err := predicatesFor(ds, expression.Context{CurrentUser: "usr_1", Today: "2026-10-10"})
	if err != nil {
		t.Fatal(err)
	}
	want := []data.FieldPredicate{
		{Field: "fld_assignee", Value: "usr_1"},
		{Field: "fld_status", Negate: true, Value: "done"},
		{Field: "fld_due", Empty: true, Alternative: true},
		{Field: "fld_due", Order: orderSQL[expression.OpGreaterThan], Value: "2026-10-17", Alternative: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d predicates, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("predicate %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// An alternative whose value cannot be resolved fails the Dataset closed, exactly as a conjunct does; dropping it
// would widen the OR-group to match nothing it should.
func TestPredicatesFor_anUnresolvableAlternativeFailsClosed(t *testing.T) {
	ds := domain.Dataset{ID: "ds_x", Where: &expression.Predicate{
		Any: []expression.Comparison{{Field: "fld_due", Op: expression.OpGreaterThan, Value: "$parameters.until"}},
	}}
	if _, err := predicatesFor(ds, expression.Context{}); err == nil {
		t.Error("an unresolvable `any:` value was accepted")
	}
}

// A Measure's filter keeps its OR-group when its context values are resolved; dropping `any:` would count every
// record the conjunction matches, a larger number that looks right.
func TestResolveMeasureFilters_keepsAndResolvesTheOrGroup(t *testing.T) {
	ds := domain.Dataset{ID: "ds_x", Measures: []domain.Measure{{ID: "msr_n", Where: &expression.Predicate{
		All: []expression.Comparison{{Field: "fld_status", Op: expression.OpNotEquals, Value: "done"}},
		Any: []expression.Comparison{{Field: "fld_due", Op: expression.OpGreaterThan, Value: "$today+7"}, {Field: "fld_due", Op: expression.OpIsEmpty}},
	}}}}
	got, err := resolveMeasureFilters(ds, expression.Context{Today: "2026-10-10"})
	if err != nil {
		t.Fatal(err)
	}
	p := got["msr_n"]
	if p == nil || len(p.All) != 1 || len(p.Any) != 2 || p.Any[0].Value != "2026-10-17" {
		t.Fatalf("resolved filter = %+v, want one conjunct and two alternatives with $today+7 resolved", p)
	}
}
