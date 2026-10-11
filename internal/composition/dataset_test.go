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

// count_distinct counts the distinct non-empty values a Field holds, overall and per group; a percentage divides
// two final Measures, rounds down, and is 0 over a zero denominator -- never a division by zero.
func TestAggregate_countDistinctAndPercentage(t *testing.T) {
	ds := domain.Dataset{
		ID: "ds_t", Dimension: "fld_assignee",
		Measures: []domain.Measure{
			{ID: "msr_all", Aggregate: domain.AggregateCount},
			{ID: "msr_done", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: []expression.Comparison{{Field: "fld_status", Op: expression.OpEquals, Value: "done"}}}},
			{ID: "msr_projects", Aggregate: domain.AggregateCountDistinct, Field: "fld_project"},
			{ID: "msr_share", Aggregate: domain.AggregatePercentage, Numerator: "msr_done", Denominator: "msr_all"},
		},
	}
	records := []*data.Record{
		rec("t1", map[string]any{"fld_assignee": "ana", "fld_project": "p1", "fld_status": "done"}),
		rec("t2", map[string]any{"fld_assignee": "ana", "fld_project": "p1", "fld_status": "todo"}),
		rec("t3", map[string]any{"fld_assignee": "ana", "fld_project": "p2", "fld_status": "todo"}),
		rec("t4", map[string]any{"fld_assignee": "budi", "fld_project": "", "fld_status": "todo"}),
		rec("t5", map[string]any{"fld_assignee": "budi", "fld_status": "todo"}),
	}
	got := mustAggregate(t, ds, records, expression.Context{})
	if want := map[string]float64{"msr_all": 5, "msr_done": 1, "msr_projects": 2, "msr_share": 20}; !reflect.DeepEqual(got.Total, want) {
		t.Errorf("Total = %v, want %v", got.Total, want)
	}
	if want := map[string]float64{"msr_all": 3, "msr_done": 1, "msr_projects": 2, "msr_share": 33}; !reflect.DeepEqual(got.ByDimension["ana"], want) {
		t.Errorf("ana = %v, want %v (33%% rounded down from 33.3)", got.ByDimension["ana"], want)
	}
	if want := map[string]float64{"msr_all": 2, "msr_done": 0, "msr_projects": 0, "msr_share": 0}; !reflect.DeepEqual(got.ByDimension["budi"], want) {
		t.Errorf("budi = %v, want %v (an empty project is no project)", got.ByDimension["budi"], want)
	}
	empty := mustAggregate(t, ds, nil, expression.Context{})
	if empty.Total["msr_share"] != 0 || empty.Total["msr_projects"] != 0 {
		t.Errorf("no records: %v, want every figure 0", empty.Total)
	}
}

// A percentage rounds down (2 of 3 is 66, not 67), is computed after a count_distinct it divides, keeps a decimal
// share that is whole in decimal whole (0.29 of 1 is 29, not 28), and a filtered count_distinct honours its filter.
func TestAggregate_percentageRoundsDownAfterDistinctAndDistinctHonoursWhere(t *testing.T) {
	done := &expression.Predicate{All: []expression.Comparison{{Field: "fld_status", Op: expression.OpEquals, Value: "done"}}}
	ds := domain.Dataset{ID: "ds_t", Measures: []domain.Measure{
		{ID: "msr_all", Aggregate: domain.AggregateCount},
		{ID: "msr_done", Aggregate: domain.AggregateCount, Where: done},
		{ID: "msr_projects", Aggregate: domain.AggregateCountDistinct, Field: "fld_project"},
		{ID: "msr_done_projects", Aggregate: domain.AggregateCountDistinct, Field: "fld_project", Where: done},
		{ID: "msr_two_thirds", Aggregate: domain.AggregatePercentage, Numerator: "msr_done", Denominator: "msr_all"},
		{ID: "msr_project_share", Aggregate: domain.AggregatePercentage, Numerator: "msr_done_projects", Denominator: "msr_projects"},
		{ID: "msr_spent", Aggregate: domain.AggregateSum, Field: "fld_spent"},
		{ID: "msr_budget", Aggregate: domain.AggregateSum, Field: "fld_budget"},
		{ID: "msr_spent_share", Aggregate: domain.AggregatePercentage, Numerator: "msr_spent", Denominator: "msr_budget"},
	}}
	records := []*data.Record{
		rec("t1", map[string]any{"fld_project": "p1", "fld_status": "done", "fld_spent": 0.29, "fld_budget": 1.0}),
		rec("t2", map[string]any{"fld_project": "p2", "fld_status": "done"}),
		rec("t3", map[string]any{"fld_project": "p2", "fld_status": "todo"}),
		rec("t4", map[string]any{"fld_project": "p3", "fld_status": "todo"}),
	}
	got := mustAggregate(t, ds, records, expression.Context{})
	for id, want := range map[string]float64{"msr_done_projects": 2, "msr_projects": 3, "msr_project_share": 66, "msr_spent_share": 29} {
		if got.Total[id] != want {
			t.Errorf("%s = %v, want %v", id, got.Total[id], want)
		}
	}
	three := mustAggregate(t, ds, records[1:], expression.Context{})
	if three.Total["msr_two_thirds"] != 33 {
		t.Errorf("1 of 3 = %v, want 33", three.Total["msr_two_thirds"])
	}
	two := mustAggregate(t, ds, append(records[:2:2], records[3]), expression.Context{})
	if two.Total["msr_two_thirds"] != 66 {
		t.Errorf("2 of 3 = %v, want 66 (rounded down, not 67)", two.Total["msr_two_thirds"])
	}
}
