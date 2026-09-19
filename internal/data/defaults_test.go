package data

import (
	"testing"

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
