package data

import (
	"testing"
	"time"

	"menata.app/internal/domain"
)

func machineWithDefault() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
		Fields: []domain.Field{
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Required: true,
				Options: []string{"todo", "in_progress", "done"}, Default: "todo"},
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
		},
	}
}

func TestApplyDefaults_fillsAbsentField(t *testing.T) {
	values := map[string]any{"fld_title": "Ship it"}
	ApplyDefaults(machineWithDefault(), values)
	if values["fld_status"] != "todo" {
		t.Errorf(`values["fld_status"] = %#v, want "todo"`, values["fld_status"])
	}
}

func TestApplyDefaults_fillsExplicitEmptyString(t *testing.T) {
	values := map[string]any{"fld_title": "Ship it", "fld_status": ""}
	ApplyDefaults(machineWithDefault(), values)
	if values["fld_status"] != "todo" {
		t.Errorf(`values["fld_status"] = %#v, want "todo"`, values["fld_status"])
	}
}

func TestApplyDefaults_neverOverwritesARealValue(t *testing.T) {
	values := map[string]any{"fld_title": "Ship it", "fld_status": "done"}
	ApplyDefaults(machineWithDefault(), values)
	if values["fld_status"] != "done" {
		t.Errorf(`values["fld_status"] = %#v, want "done" (must not overwrite a real value)`, values["fld_status"])
	}
}

func TestApplyDefaults_noOpWhenFieldDeclaresNoDefault(t *testing.T) {
	values := map[string]any{"fld_status": "todo"}
	ApplyDefaults(machineWithDefault(), values)
	if _, present := values["fld_title"]; present {
		t.Errorf(`values["fld_title"] = %#v, want absent -- fld_title declares no Default`, values["fld_title"])
	}
}

func TestApplyDefaultsAtDerivesADateFromAStatus(t *testing.T) {
	m := &domain.Machine{Fields: []domain.Field{
		// Declared after the date on purpose: the derived date must still read the status a default filled.
		{ID: "fld_due", Type: domain.FieldTypeDate, DefaultFrom: &domain.FieldDefaultFrom{Field: "fld_priority", Days: map[string]int{"tinggi": 1, "normal": 3}}},
		{ID: "fld_priority", Type: domain.FieldTypeStatus, Options: []string{"tinggi", "normal", "rendah"}, Default: "normal"},
	}}
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		in   map[string]any
		want any
	}{
		"a priority the map names":           {map[string]any{"fld_priority": "tinggi"}, "2026-10-31"},
		"the default priority":               {map[string]any{}, "2026-11-02"},
		"a priority the map leaves out":      {map[string]any{"fld_priority": "rendah"}, nil},
		"a date the submitter already chose": {map[string]any{"fld_priority": "tinggi", "fld_due": "2027-01-01"}, "2027-01-01"},
		"an empty date counts as not chosen": {map[string]any{"fld_priority": "tinggi", "fld_due": ""}, "2026-10-31"},
	} {
		ApplyDefaultsAt(m, tc.in, now)
		if got := tc.in["fld_due"]; got != tc.want {
			t.Errorf("%s: fld_due = %v, want %v", name, got, tc.want)
		}
	}
}

func TestValidateRecordRequiresAFieldOnlyWhileTheConditionHolds(t *testing.T) {
	m := &domain.Machine{Fields: []domain.Field{
		{ID: "fld_format", Type: domain.FieldTypeStatus, Options: []string{"poster", "banner"}},
		{ID: "fld_file", Type: domain.FieldTypeText, RequiredWhen: &domain.FieldCondition{Field: "fld_format", Equals: "banner"}},
	}}
	for name, tc := range map[string]struct {
		in      map[string]any
		refused bool
	}{
		"the condition holds and the field is empty":        {map[string]any{"fld_format": "banner"}, true},
		"the condition holds and the field is empty string": {map[string]any{"fld_format": "banner", "fld_file": ""}, true},
		"the condition holds and the field is filled":       {map[string]any{"fld_format": "banner", "fld_file": "x"}, false},
		"another value":               {map[string]any{"fld_format": "poster"}, false},
		"the watched field is absent": {map[string]any{}, false},
	} {
		if err := ValidateRecord(m, tc.in); (err != nil) != tc.refused {
			t.Errorf("%s: ValidateRecord = %v, want refused=%v", name, err, tc.refused)
		}
	}
}
