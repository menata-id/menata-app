package data

import (
	"net/url"
	"testing"

	"menata.app/internal/domain"
)

func commentMachine() *domain.Machine {
	return &domain.Machine{ID: "mch_comment", Fields: []domain.Field{
		{ID: "fld_body", Type: domain.FieldTypeLongText},
		{ID: "fld_author", Type: domain.FieldTypePerson, Stamp: domain.FieldStampCurrentUser},
	}}
}

func TestApplyStamps(t *testing.T) {
	cases := []struct {
		name    string
		actor   string
		values  map[string]any
		want    any
		present bool
	}{
		{"the acting user is written", "usr_ana", map[string]any{}, "usr_ana", true},
		{"a submitted author is overwritten", "usr_ana", map[string]any{"fld_author": "usr_budi"}, "usr_ana", true},
		{"no actor drops a submitted author rather than keeping it", "", map[string]any{"fld_author": "usr_budi"}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ApplyStamps(commentMachine(), tc.values, tc.actor)
			got, ok := tc.values["fld_author"]
			if ok != tc.present || got != tc.want && tc.present {
				t.Errorf("fld_author = %v (present %v), want %v (present %v)", got, ok, tc.want, tc.present)
			}
		})
	}
}

func TestApplyStamps_LeavesOrdinaryFieldsAlone(t *testing.T) {
	values := map[string]any{"fld_body": "hi"}
	ApplyStamps(commentMachine(), values, "usr_ana")
	if values["fld_body"] != "hi" {
		t.Errorf("an unstamped Field changed: %v", values)
	}
}

func TestValuesFromForm_ignoresStampedFields(t *testing.T) {
	values := ValuesFromForm(commentMachine(), url.Values{"fld_body": {"hi"}, "fld_author": {"usr_budi"}})
	if _, ok := values["fld_author"]; ok {
		t.Errorf("a submitted stamped value was read: %v", values)
	}
	if values["fld_body"] != "hi" {
		t.Errorf("an ordinary Field was dropped: %v", values)
	}
}
