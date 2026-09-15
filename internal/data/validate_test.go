package data

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func testMachine() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Required: true, Options: []string{"todo", "done"}},
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson},
			{ID: "fld_priority", Name: "Priority", Type: domain.FieldTypeNumber},
			{ID: "fld_project", Name: "Project", Type: domain.FieldTypeRelation, RelatedMachine: "mch_project"},
		},
	}
}

func TestValidateRecord_valid(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":  "Write docs",
		"fld_status": "todo",
	})
	if err != nil {
		t.Fatalf("ValidateRecord() error = %v, want nil", err)
	}
}

func TestValidateRecord_missingRequired(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_status": "todo",
	})
	assertContains(t, err, `"fld_title" is required`)
}

func TestValidateRecord_unknownField(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":  "Write docs",
		"fld_status": "todo",
		"fld_ghost":  "x",
	})
	assertContains(t, err, `unknown field "fld_ghost"`)
}

func TestValidateRecord_badStatusOption(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":  "Write docs",
		"fld_status": "archived",
	})
	assertContains(t, err, `not one of`)
}

func TestValidateRecord_optionalFieldOmitted(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":  "Write docs",
		"fld_status": "todo",
	})
	if err != nil {
		t.Fatalf("ValidateRecord() error = %v, want nil (fld_assignee is optional)", err)
	}
}

func TestValidateRecord_numberField_valid(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":    "Write docs",
		"fld_status":   "todo",
		"fld_priority": float64(2),
	})
	if err != nil {
		t.Fatalf("ValidateRecord() error = %v, want nil", err)
	}
}

func TestValidateRecord_numberField_notANumber(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":    "Write docs",
		"fld_status":   "todo",
		"fld_priority": "high",
	})
	assertContains(t, err, `"fld_priority": value high is not a number`)
}

func TestValidateRecord_relationField_valid(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":   "Write docs",
		"fld_status":  "todo",
		"fld_project": "rec_abc123",
	})
	if err != nil {
		t.Fatalf("ValidateRecord() error = %v, want nil", err)
	}
}

func TestValidateRecord_relationField_notAString(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":   "Write docs",
		"fld_status":  "todo",
		"fld_project": 42,
	})
	assertContains(t, err, `"fld_project": value must be a record id`)
}

func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want error containing %q", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), substr)
	}
}
