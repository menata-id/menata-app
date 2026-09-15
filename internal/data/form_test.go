package data

import (
	"net/url"
	"testing"

	"menata.app/internal/domain"
)

func TestValuesFromForm(t *testing.T) {
	m := testMachine()
	form := url.Values{
		"fld_title":    {"Write docs"},
		"fld_status":   {"todo"},
		"fld_priority": {"3"},
		// fld_assignee intentionally omitted, like a blank optional form field.
	}

	values := ValuesFromForm(m, form)

	if values["fld_title"] != "Write docs" {
		t.Errorf("fld_title = %v, want %q", values["fld_title"], "Write docs")
	}
	if _, ok := values["fld_assignee"]; ok {
		t.Errorf("fld_assignee = %v, want omitted", values["fld_assignee"])
	}
	if values["fld_priority"] != 3.0 {
		t.Errorf("fld_priority = %v (%T), want float64(3)", values["fld_priority"], values["fld_priority"])
	}
}

func TestValuesFromForm_booleanAlwaysPresent(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_task",
		Fields: []domain.Field{
			{ID: "fld_done", Type: domain.FieldTypeBoolean},
		},
	}

	values := ValuesFromForm(m, url.Values{})
	if values["fld_done"] != false {
		t.Errorf("fld_done = %v, want false for an unchecked/absent checkbox", values["fld_done"])
	}

	values = ValuesFromForm(m, url.Values{"fld_done": {"on"}})
	if values["fld_done"] != true {
		t.Errorf("fld_done = %v, want true when the form key is present", values["fld_done"])
	}
}

func TestValuesFromForm_unparsableNumberKeptAsString(t *testing.T) {
	m := testMachine()
	values := ValuesFromForm(m, url.Values{"fld_priority": {"not-a-number"}})
	if values["fld_priority"] != "not-a-number" {
		t.Errorf("fld_priority = %v, want the raw string so ValidateRecord can reject it", values["fld_priority"])
	}
}
