package data

import (
	"net/url"
	"testing"

	"menata.app/internal/domain"
)

// A switch sends "false" before its box, so an unticked one still names its Field; a plain checkbox sends nothing.
func TestValuesFromFormReadsASwitchAndACheckbox(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Fields: []domain.Field{{ID: "fld_vat", Name: "VAT applies", Type: domain.FieldTypeBoolean}}}
	for _, tc := range []struct {
		name string
		form url.Values
		want bool
	}{
		{"switch unticked", url.Values{"fld_vat": {"false"}}, false},
		{"switch ticked", url.Values{"fld_vat": {"false", "on"}}, true},
		{"checkbox ticked", url.Values{"fld_vat": {"on"}}, true},
		{"checkbox unticked", url.Values{}, false},
	} {
		if got := ValuesFromForm(m, tc.form)["fld_vat"]; got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}
