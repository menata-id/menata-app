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
