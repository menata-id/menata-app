package composition

import (
	"reflect"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// taskWorkload is the shape metadata/task.yaml actually declares: count every record, count again
// only the ones a filter keeps, both grouped by one Dimension. Using the real shape rather than a
// synthetic one keeps this test honest about what Team Capacity depends on.
func taskWorkload() domain.Dataset {
	return domain.Dataset{
		ID:        "ds_task_workload",
		Dimension: "fld_assignee",
		Measures: []domain.Measure{
			{ID: "msr_total", Aggregate: domain.AggregateCount},
			{ID: "msr_active", Aggregate: domain.AggregateCount, Where: &expression.Comparison{
				Field: "fld_status", Op: expression.OpNotEquals, Value: "done",
			}},
		},
	}
}

// taskByProject and taskByStatus mirror metadata/task.yaml's other two declarations, and
// documentByStatus metadata/document.yaml's. internal/conformance.TestComposedScreenDatasetsAreDeclared
// is what keeps these fixtures honest against the real files.
func taskByProject() domain.Dataset {
	return domain.Dataset{
		ID:        "ds_task_by_project",
		Dimension: "fld_project",
		Measures: []domain.Measure{
			{ID: "msr_total", Aggregate: domain.AggregateCount},
			{ID: "msr_active", Aggregate: domain.AggregateCount, Where: &expression.Comparison{
				Field: "fld_status", Op: expression.OpNotEquals, Value: "done",
			}},
		},
	}
}

func taskByStatus() domain.Dataset {
	return domain.Dataset{
		ID:        "ds_task_by_status",
		Dimension: "fld_status",
		Measures:  []domain.Measure{{ID: "msr_total", Aggregate: domain.AggregateCount}},
	}
}

func documentByStatus() domain.Dataset {
	return domain.Dataset{
		ID:        "ds_document_by_status",
		Dimension: "fld_status",
		Measures:  []domain.Measure{{ID: "msr_total", Aggregate: domain.AggregateCount}},
	}
}

// userCapacity mirrors metadata/user.yaml's own declaration: a grand total, no dimension.
func userCapacity() domain.Dataset {
	return domain.Dataset{
		ID:       "ds_user_capacity",
		Measures: []domain.Measure{{ID: "msr_total_capacity", Aggregate: domain.AggregateSum, Field: "fld_weekly_capacity"}},
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
	got := Aggregate(taskWorkload(), workloadRecords())

	wantTotal := map[string]float64{"msr_total": 4, "msr_active": 2}
	if !reflect.DeepEqual(got.Total, wantTotal) {
		t.Errorf("Total = %v, want %v", got.Total, wantTotal)
	}

	wantByDimension := map[string]map[string]float64{
		"usr_ana":  {"msr_total": 3, "msr_active": 2},
		"usr_budi": {"msr_total": 1, "msr_active": 0},
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
	got := Aggregate(taskWorkload(), workloadRecords())

	budi, ok := got.ByDimension["usr_budi"]
	if !ok {
		t.Fatal("ByDimension has no usr_budi group -- a group whose records are all filtered out of a measure must still appear, reading zero")
	}
	if budi["msr_active"] != 0 {
		t.Errorf("usr_budi msr_active = %v, want 0", budi["msr_active"])
	}
}

// TestAggregate_sumWithoutDimension is the other real declaration (metadata/user.yaml): a grand
// total with no breakdown. ByDimension must stay empty rather than inventing a single group.
func TestAggregate_sumWithoutDimension(t *testing.T) {
	ds := domain.Dataset{
		ID:       "ds_user_capacity",
		Measures: []domain.Measure{{ID: "msr_total_capacity", Aggregate: domain.AggregateSum, Field: "fld_weekly_capacity"}},
	}
	records := []*data.Record{
		rec("usr_ana", map[string]any{"fld_weekly_capacity": float64(40)}),
		rec("usr_budi", map[string]any{"fld_weekly_capacity": float64(20)}),
		rec("usr_citra", map[string]any{}), // never filled in -- contributes nothing, not an error
	}

	got := Aggregate(ds, records)

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
	got := Aggregate(taskWorkload(), nil)

	for _, id := range []string{"msr_total", "msr_active"} {
		if _, ok := got.Total[id]; !ok {
			t.Errorf("Total has no entry for declared measure %q over an empty record set -- want an explicit zero", id)
		}
	}
}
