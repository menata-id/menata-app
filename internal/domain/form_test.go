package domain

import "testing"

func TestEditFormInputsStartAsTheRecordAndLeaveOutWhatPatchCannotDo(t *testing.T) {
	m := &Machine{ID: "mch_x", Fields: []Field{
		{ID: "fld_name", Name: "Name", Type: FieldTypeText, Required: true},
		{ID: "fld_points", Name: "Points", Type: FieldTypeNumber},
		{ID: "fld_done", Name: "Done", Type: FieldTypeBoolean},
		{ID: "fld_status", Name: "Status", Type: FieldTypeStatus, Options: []string{"a", "b"}},
		{ID: "fld_color", Name: "Color", Type: FieldTypeStatus, Options: []string{"blue", "rose"}},
		{ID: "fld_file", Name: "File", Type: FieldTypeFile},
		{ID: "fld_by", Name: "By", Type: FieldTypeText, Stamp: "actor"},
	}, Transitions: []Transition{{Field: "fld_status", From: "a", To: "b"}}}
	got := m.EditFormInputs(map[string]any{"fld_name": "alpha", "fld_points": 3.5, "fld_done": true, "fld_status": "a", "fld_color": "rose"})
	if len(got) != 3 || got[0].FieldID != "fld_name" || got[1].FieldID != "fld_points" || got[2].FieldID != "fld_color" {
		t.Fatalf("inputs = %+v, want name, points and color (a boolean cannot be cleared by PATCH, a governed status moves along its edges, a file and a stamp are not typed; a status no Transition names is a plain choice)", got)
	}
	if c := got[2]; c.Kind != InputSelect || c.Default != "rose" || len(c.Options) != 2 {
		t.Errorf("an ungoverned status must be a select of its options starting at the record's value: %+v", c)
	}
	if got[0].Default != "alpha" || !got[0].Required || got[1].Default != "3.5" {
		t.Errorf("a control must start as the record's value: %+v", got)
	}
	if blank := m.EditFormInputs(nil); blank[0].Default != "" {
		t.Errorf("no record, no starting value: %+v", blank)
	}
}

func TestCreateFormInputsLeaveAGovernedStatusAtItsDefault(t *testing.T) {
	m := &Machine{ID: "mch_loan", Fields: []Field{
		{ID: "fld_purpose", Name: "Purpose", Type: FieldTypeText, Required: true},
		{ID: "fld_status", Name: "Status", Type: FieldTypeStatus, Options: []string{"applied", "disbursed"}, Default: "applied"},
		{ID: "fld_stage", Name: "Stage", Type: FieldTypeStatus, Options: []string{"x", "y"}},
		{ID: "fld_color", Name: "Color", Type: FieldTypeStatus, Options: []string{"blue", "rose"}, Default: "blue"},
	}, Transitions: []Transition{
		{Field: "fld_status", From: "applied", To: "disbursed"},
		{Field: "fld_stage", From: "x", To: "y"},
	}}
	got, _ := m.CreateFormInputs()
	var ids []string
	for _, in := range got {
		ids = append(ids, in.FieldID)
	}
	if len(ids) != 3 || ids[0] != "fld_purpose" || ids[1] != "fld_stage" || ids[2] != "fld_color" {
		t.Fatalf("inputs = %v, want purpose, stage and color: a governed status with a default starts there and moves only "+
			"along its edges; one with no default still needs a value; an ungoverned status is a plain choice", ids)
	}
}
