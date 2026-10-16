package behavior

import (
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

func projectMachine() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_project",
		Name: "Project",
		// The Field the Constraint below gates on. Absent until 2026-09-29 (metadata.Validate).
		Fields: []domain.Field{{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"active", "done"}}},
		Constraints: []domain.Constraint{
			{
				ID:         "cst_project_done_no_open_tasks",
				On:         "fld_status",
				WhenEquals: "done",
				BlockIf: domain.RelationBlock{
					RelatedMachine: "mch_task",
					RelatedField:   "fld_project",
					Condition:      expression.Comparison{Field: "fld_status", Op: expression.OpNotEquals, Value: "done"},
				},
			},
		},
	}
}

func TestCheckConstraints_blockedByOpenTask(t *testing.T) {
	related := map[string][]*data.Record{
		"mch_task": {
			{ID: "rec_task1", Values: map[string]any{"fld_project": "rec_proj1", "fld_status": "todo"}},
		},
	}

	err := CheckConstraints(projectMachine(), "rec_proj1", map[string]any{"fld_status": "done"}, related)
	if err == nil {
		t.Fatal("CheckConstraints() error = nil, want a violation: an open task still references this project")
	}
	if !strings.Contains(err.Error(), "cst_project_done_no_open_tasks") {
		t.Errorf("error = %q, want it to name the violated constraint", err.Error())
	}
}

func TestCheckConstraints_allowedWhenTasksDone(t *testing.T) {
	related := map[string][]*data.Record{
		"mch_task": {
			{ID: "rec_task1", Values: map[string]any{"fld_project": "rec_proj1", "fld_status": "done"}},
		},
	}

	err := CheckConstraints(projectMachine(), "rec_proj1", map[string]any{"fld_status": "done"}, related)
	if err != nil {
		t.Fatalf("CheckConstraints() error = %v, want nil: the only task referencing this project is done", err)
	}
}

func TestCheckConstraints_allowedWhenNoRelatedRecords(t *testing.T) {
	err := CheckConstraints(projectMachine(), "rec_proj1", map[string]any{"fld_status": "done"}, nil)
	if err != nil {
		t.Fatalf("CheckConstraints() error = %v, want nil: no tasks reference this project at all", err)
	}
}

func TestCheckConstraints_unrelatedTaskIgnored(t *testing.T) {
	related := map[string][]*data.Record{
		"mch_task": {
			// belongs to a different project -- must not block this one.
			{ID: "rec_task1", Values: map[string]any{"fld_project": "rec_other_project", "fld_status": "todo"}},
		},
	}

	err := CheckConstraints(projectMachine(), "rec_proj1", map[string]any{"fld_status": "done"}, related)
	if err != nil {
		t.Fatalf("CheckConstraints() error = %v, want nil: the open task belongs to a different project", err)
	}
}

func TestCheckConstraints_notFiredWhenTransitionDoesNotMatch(t *testing.T) {
	related := map[string][]*data.Record{
		"mch_task": {
			{ID: "rec_task1", Values: map[string]any{"fld_project": "rec_proj1", "fld_status": "todo"}},
		},
	}

	// Setting status to "active", not "done" -- the constraint only fires on the done transition.
	err := CheckConstraints(projectMachine(), "rec_proj1", map[string]any{"fld_status": "active"}, related)
	if err != nil {
		t.Fatalf("CheckConstraints() error = %v, want nil: this constraint only fires when fld_status becomes done", err)
	}
}

func TestCheckBounds(t *testing.T) {
	zero, cap := 0.0, 100.0
	floor := &domain.Machine{Bounds: []domain.AggregateBound{{ID: "ens_floor", SumOf: "fld_n", Per: "fld_item", AtLeast: &zero, Reason: "stok negatif"}}}
	ceil := &domain.Machine{Bounds: []domain.AggregateBound{{ID: "ens_cap", SumOf: "fld_n", Per: "fld_item", AtMost: &cap}}}
	rec := func(id, item string, n float64) *data.Record {
		return &data.Record{ID: id, Values: map[string]any{"fld_item": item, "fld_n": n}}
	}
	stored := []*data.Record{rec("r1", "a", 100), rec("r2", "a", -30), rec("r3", "b", 5)}
	for _, tc := range []struct {
		name    string
		m       *domain.Machine
		id      string
		values  map[string]any
		refused string
	}{
		{"a floor kept", floor, "", map[string]any{"fld_item": "a", "fld_n": -70.0}, ""},
		{"a floor broken by one more", floor, "", map[string]any{"fld_item": "a", "fld_n": -71.0}, "stok negatif"},
		{"another group has its own total", floor, "", map[string]any{"fld_item": "b", "fld_n": -6.0}, "it would be -1 for b"},
		{"a group with nothing stored", floor, "", map[string]any{"fld_item": "new", "fld_n": -1.0}, "ens_floor"},
		{"an edit replaces its stored self", floor, "r1", map[string]any{"fld_item": "a", "fld_n": 30.0}, ""},
		{"an edit that leaves too little", floor, "r1", map[string]any{"fld_item": "a", "fld_n": 29.0}, "ens_floor"},
		{"a cap kept at its edge", ceil, "", map[string]any{"fld_item": "b", "fld_n": 95.0}, ""},
		{"a cap broken", ceil, "", map[string]any{"fld_item": "b", "fld_n": 96.0}, "may not exceed 100"},
		{"no group value is outside every bound", floor, "", map[string]any{"fld_n": -999.0}, ""},
		{"a missing number counts as nothing", floor, "", map[string]any{"fld_item": "a"}, ""},
	} {
		err := CheckBounds(tc.m, tc.id, tc.values, stored)
		switch {
		case tc.refused == "" && err != nil:
			t.Errorf("%s: refused: %v", tc.name, err)
		case tc.refused != "" && (err == nil || !strings.Contains(err.Error(), tc.refused)):
			t.Errorf("%s: got %v, want an error containing %q", tc.name, err, tc.refused)
		}
	}
}
