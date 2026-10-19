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
			{ID: "fld_attachment", Name: "Attachment", Type: domain.FieldTypeFile},
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

// A required text that is only spaces is as missing as an empty one: a list renamed to "   " would be a column with no name, and the
// browser's own `required` passes it.
func TestValidateRecord_requiredTextOfOnlySpacesIsMissing(t *testing.T) {
	for _, blank := range []string{"   ", "\t", " \n "} {
		err := ValidateRecord(testMachine(), map[string]any{"fld_title": blank, "fld_status": "todo"})
		if err == nil || !strings.Contains(err.Error(), "fld_title") {
			t.Errorf("title %q: error = %v, want fld_title required", blank, err)
		}
	}
	// Not required, a blank value is left alone, and a status never gets to be blank by this rule.
	if err := ValidateRecord(testMachine(), map[string]any{"fld_title": "x", "fld_status": "todo", "fld_priority": 3}); err != nil {
		t.Errorf("valid record: %v", err)
	}
	if err := ValidateRecord(testMachine(), map[string]any{"fld_title": "x", "fld_status": " "}); err == nil {
		t.Error("a status of one space was accepted")
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

func TestValidateRecord_fileField_valid(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":      "Write docs",
		"fld_status":     "todo",
		"fld_attachment": "mch_task/fld_attachment/a1b2c3d4__contract.pdf",
	})
	if err != nil {
		t.Fatalf("ValidateRecord() error = %v, want nil", err)
	}
}

func TestValidateRecord_fileField_notAString(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":      "Write docs",
		"fld_status":     "todo",
		"fld_attachment": 42,
	})
	assertContains(t, err, `"fld_attachment": value must be a storage key`)
}

func TestValidateRecord_fileField_optionalOmitted(t *testing.T) {
	err := ValidateRecord(testMachine(), map[string]any{
		"fld_title":  "Write docs",
		"fld_status": "todo",
	})
	if err != nil {
		t.Fatalf("ValidateRecord() error = %v, want nil (fld_attachment is optional)", err)
	}
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

// A date Field holds a day, YYYY-MM-DD: a value that is not one ("2027-01-05x") would sit in storage and fall into no day of
// any calendar window that happens to include it.
func TestValidateRecord_dateField(t *testing.T) {
	m := testMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_due", Name: "Due", Type: domain.FieldTypeDate})
	for _, ok := range []string{"2027-01-05", "2024-02-29"} {
		if err := ValidateRecord(m, map[string]any{"fld_title": "x", "fld_status": "todo", "fld_due": ok}); err != nil {
			t.Errorf("date %q: error = %v, want nil", ok, err)
		}
	}
	for _, bad := range []any{"2027-01-05x", "05/01/2027", "2027-13-01", "2023-02-29", "tomorrow", 20270105, true} {
		err := ValidateRecord(m, map[string]any{"fld_title": "x", "fld_status": "todo", "fld_due": bad})
		if err == nil || !strings.Contains(err.Error(), "fld_due") {
			t.Errorf("date %v: error = %v, want a refusal naming fld_due", bad, err)
		}
	}
	if err := ValidateRecord(m, map[string]any{"fld_title": "x", "fld_status": "todo", "fld_due": ""}); err != nil {
		t.Errorf("an empty date is no date, not a bad one: %v", err)
	}
}
