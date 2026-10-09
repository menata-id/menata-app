package domain

import "testing"

func TestEditFormInputsStartAsTheRecordAndLeaveOutWhatPatchCannotDo(t *testing.T) {
	m := &Machine{ID: "mch_x", Fields: []Field{
		{ID: "fld_name", Name: "Name", Type: FieldTypeText, Required: true},
		{ID: "fld_points", Name: "Points", Type: FieldTypeNumber},
		{ID: "fld_done", Name: "Done", Type: FieldTypeBoolean},
		{ID: "fld_status", Name: "Status", Type: FieldTypeStatus, Options: []string{"a", "b"}},
		{ID: "fld_file", Name: "File", Type: FieldTypeFile},
		{ID: "fld_by", Name: "By", Type: FieldTypeText, Stamp: "actor"},
	}}
	got := m.EditFormInputs(map[string]any{"fld_name": "alpha", "fld_points": 3.5, "fld_done": true, "fld_status": "a"})
	if len(got) != 2 || got[0].FieldID != "fld_name" || got[1].FieldID != "fld_points" {
		t.Fatalf("inputs = %+v, want only name and points (a boolean cannot be cleared by PATCH, a status moves along its edges, a file and a stamp are not typed)", got)
	}
	if got[0].Default != "alpha" || !got[0].Required || got[1].Default != "3.5" {
		t.Errorf("a control must start as the record's value: %+v", got)
	}
	if blank := m.EditFormInputs(nil); blank[0].Default != "" {
		t.Errorf("no record, no starting value: %+v", blank)
	}
}
