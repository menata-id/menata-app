package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func validMachine() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}},
		},
	}
}

func TestValidate_valid(t *testing.T) {
	if err := Validate(validMachine()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_badMachineID(t *testing.T) {
	m := validMachine()
	m.ID = "task"
	assertIssue(t, m, "machine id")
}

func TestValidate_missingName(t *testing.T) {
	m := validMachine()
	m.Name = ""
	assertIssue(t, m, "name is required")
}

func TestValidate_badFieldID(t *testing.T) {
	m := validMachine()
	m.Fields[0].ID = "title"
	assertIssue(t, m, "field id")
}

func TestValidate_duplicateFieldID(t *testing.T) {
	m := validMachine()
	m.Fields[1].ID = m.Fields[0].ID
	assertIssue(t, m, "declared more than once")
}

func TestValidate_unknownType(t *testing.T) {
	m := validMachine()
	m.Fields[0].Type = "widget"
	assertIssue(t, m, "unknown type")
}

func TestValidate_statusWithoutOptions(t *testing.T) {
	m := validMachine()
	m.Fields[1].Options = nil
	assertIssue(t, m, "requires at least one option")
}

func assertIssue(t *testing.T, m *domain.Machine, substr string) {
	t.Helper()
	err := Validate(m)
	if err == nil {
		t.Fatalf("Validate() error = nil, want error containing %q", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), substr)
	}
}
