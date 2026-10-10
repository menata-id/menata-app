package composition

import (
	"reflect"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// taskWorkload is a synthetic Dataset exercising Aggregate's whole surface: count every record, count
// again only the ones a filter keeps, both grouped by one Dimension.
func taskWorkload() domain.Dataset {
	return domain.Dataset{
		ID:        "ds_test_workload",
		Dimension: "fld_assignee",
		Measures: []domain.Measure{
			{ID: "msr_total", Aggregate: domain.AggregateCount},
			{ID: "msr_total_open", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: []expression.Comparison{{
				Field: "fld_status", Op: expression.OpNotEquals, Value: "done",
			}}}},
		},
	}
}

func workloadRecords() []*data.Record {
	return []*data.Record{
		rec("tsk_1", map[string]any{"fld_assignee": "usr_ana", "fld_status": "todo"}),
		rec("tsk_2", map[string]any{"fld_assignee": "usr_ana", "fld_status": "done"}),
		rec("tsk_3", map[string]any{"fld_assignee": "usr_ana", "fld_status": "in_progress"}),
		rec("tsk_4", map[string]any{"fld_assignee": "usr_budi", "fld_status": "done"}),
	}
}

func TestAggregate_countGroupedByDimensionWithFilter(t *testing.T) {
	got := mustAggregate(t, taskWorkload(), workloadRecords(), expression.Context{})

	wantTotal := map[string]float64{"msr_total": 4, "msr_total_open": 2}
	if !reflect.DeepEqual(got.Total, wantTotal) {
		t.Errorf("Total = %v, want %v", got.Total, wantTotal)
	}

	wantByDimension := map[string]map[string]float64{
		"usr_ana":  {"msr_total": 3, "msr_total_open": 2},
		"usr_budi": {"msr_total": 1, "msr_total_open": 0},
	}
	if !reflect.DeepEqual(got.ByDimension, wantByDimension) {
		t.Errorf("ByDimension = %v, want %v", got.ByDimension, wantByDimension)
	}
}

// TestAggregate_groupSurvivesEveryMeasureFiltering is the case buildCapacity depends on and a
// naive implementation gets wrong: usr_budi's only Task is done, so the filtered Measure keeps
// nothing for that group. The group must still be present, reading zero -- "this assignee exists
// and has nothing open" is a real answer a capacity table renders, and a missing key would make it
// indistinguishable from an assignee who has no records at all.
func TestAggregate_groupSurvivesEveryMeasureFiltering(t *testing.T) {
	got := mustAggregate(t, taskWorkload(), workloadRecords(), expression.Context{})

	budi, ok := got.ByDimension["usr_budi"]
	if !ok {
		t.Fatal("ByDimension has no usr_budi group -- a group whose records are all filtered out of a measure must still appear, reading zero")
	}
	if budi["msr_total_open"] != 0 {
		t.Errorf("usr_budi msr_total_open = %v, want 0", budi["msr_total_open"])
	}
}

// TestAggregate_sumWithoutDimension is the no-breakdown case: a grand total with no dimension. ByDimension must stay empty rather than inventing a single group.
func TestAggregate_sumWithoutDimension(t *testing.T) {
	ds := domain.Dataset{
		ID:       "ds_test_total",
		Measures: []domain.Measure{{ID: "msr_total_capacity", Aggregate: domain.AggregateSum, Field: "fld_weekly_capacity"}},
	}
	records := []*data.Record{
		rec("usr_ana", map[string]any{"fld_weekly_capacity": float64(40)}),
		rec("usr_budi", map[string]any{"fld_weekly_capacity": float64(20)}),
		rec("usr_citra", map[string]any{}), // never filled in -- contributes nothing, not an error
	}

	got := mustAggregate(t, ds, records, expression.Context{})

	if got.Total["msr_total_capacity"] != 60 {
		t.Errorf("Total[msr_total_capacity] = %v, want 60", got.Total["msr_total_capacity"])
	}
	if len(got.ByDimension) != 0 {
		t.Errorf("ByDimension = %v, want empty for a Dataset declaring no dimension", got.ByDimension)
	}
}

// TestAggregate_declaredMeasureIsAlwaysPresent pins the "explicit zero, never absent" contract
// Aggregation's own doc comment states: a caller reading a Measure it declared must not have to
// distinguish "no record satisfied it" from "I misspelled the id".
func TestAggregate_declaredMeasureIsAlwaysPresent(t *testing.T) {
	got := mustAggregate(t, taskWorkload(), nil, expression.Context{})

	for _, id := range []string{"msr_total", "msr_total_open"} {
		if _, ok := got.Total[id]; !ok {
			t.Errorf("Total has no entry for declared measure %q over an empty record set -- want an explicit zero", id)
		}
	}
}

func mustAggregate(t *testing.T, ds domain.Dataset, records []*data.Record, ctx expression.Context) Aggregation {
	t.Helper()
	got, err := Aggregate(ds, records, ctx)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	return got
}

// A Measure's `where:` is a conjunction and may compare a date against `$today`: "overdue" is "not finished
// AND due before today", which neither comparison says alone. Three records separate the three ways to get it
// wrong -- ignoring the second comparison counts the finished-but-late one, ignoring the first counts the
// not-yet-due one, and resolving `$today` as text ("$today" sorts after every digit) counts nothing.
func TestAggregate_measureWhereIsAConjunctionResolvingToday(t *testing.T) {
	ds := domain.Dataset{ID: "ds_overdue", Measures: []domain.Measure{{
		ID: "msr_overdue", Aggregate: domain.AggregateCount,
		Where: &expression.Predicate{All: []expression.Comparison{
			{Field: "fld_status", Op: expression.OpNotEquals, Value: "done"},
			{Field: "fld_due", Op: expression.OpLessThan, Value: expression.SentinelToday},
		}},
	}}}
	records := []*data.Record{
		rec("a", map[string]any{"fld_status": "todo", "fld_due": "2026-10-01"}), // late and open: counts
		rec("b", map[string]any{"fld_status": "done", "fld_due": "2026-10-01"}), // late but finished
		rec("c", map[string]any{"fld_status": "todo", "fld_due": "2026-10-20"}), // open but not yet due
		rec("d", map[string]any{"fld_status": "todo"}),                          // no date is not "before today"
	}
	got := mustAggregate(t, ds, records, expression.Context{Today: "2026-10-10"})
	if got.Total["msr_overdue"] != 1 {
		t.Errorf("msr_overdue = %v, want 1", got.Total["msr_overdue"])
	}
	moved := mustAggregate(t, ds, records, expression.Context{Today: "2026-10-25"})
	if moved.Total["msr_overdue"] != 2 {
		t.Errorf("with today moved past c's date msr_overdue = %v, want 2 (the same records must answer to the injected date)", moved.Total["msr_overdue"])
	}
}

// With no date to resolve `$today` against, the answer is an error. A count that became 0 because the clock
// was missing would look exactly like "nothing is overdue".
func TestAggregate_failsClosedWhenTodayCannotBeResolved(t *testing.T) {
	ds := domain.Dataset{ID: "ds_overdue", Measures: []domain.Measure{{
		ID: "msr_overdue", Aggregate: domain.AggregateCount,
		Where: &expression.Predicate{All: []expression.Comparison{
			{Field: "fld_due", Op: expression.OpLessThan, Value: expression.SentinelToday},
		}},
	}}}
	if _, err := Aggregate(ds, nil, expression.Context{}); err == nil {
		t.Error("Aggregate answered a `$today` filter with no date to resolve it against")
	}
}
