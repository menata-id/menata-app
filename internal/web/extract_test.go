package web

import (
	"net/url"
	"testing"

	"menata.app/internal/composition"
	"menata.app/internal/domain"
)

// TestSubmittedRowsReadOnlyWhatTheRowKindDeclares: a tick that is absent leaves a row out, an index is read once however many
// of its controls arrive, and a Field the declaration does not name -- a boolean the Machine has, for one: an unticked box is
// always "present" as false -- is not in the row's values whatever the request holds.
func TestSubmittedRowsReadOnlyWhatTheRowKindDeclares(t *testing.T) {
	child := &domain.Machine{ID: "mch_t", Fields: []domain.Field{
		{ID: "fld_label", Type: domain.FieldTypeText}, {ID: "fld_gross", Type: domain.FieldTypeNumber},
		{ID: "fld_flag", Type: domain.FieldTypeBoolean}, {ID: "fld_secret", Type: domain.FieldTypeText}}}
	x := domain.FormExtract{Rows: []domain.ExtractRows{{Label: "Terms", Machine: "mch_t", Via: "fld_contract", Fields: []string{"fld_label", "fld_gross"}}}}
	loader := composition.NewLoader(nil, map[string]*domain.Machine{"mch_t": child})
	form := url.Values{
		"rows.0.0.include": {"on"}, "rows.0.0.fld_label": {"A"}, "rows.0.0.fld_gross": {"5"}, "rows.0.0.fld_secret": {"x"},
		"rows.0.1.fld_label": {"left out"},
		"rows.0.2.include":   {"on"}, "rows.0.2.fld_label": {"C"},
		"rows.1.0.include": {"on"}, "rows.1.0.fld_label": {"another kind"},
		"rows.0.9999.include": {"on"},
	}
	rows, err := submittedRows(x, loader, form)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows, want the two that were ticked (and no more than the index bound)", len(rows))
	}
	if rows[0].values["fld_label"] != "A" || rows[0].values["fld_gross"] != 5.0 || rows[0].via != "fld_contract" {
		t.Errorf("first row = %+v", rows[0])
	}
	if _, ok := rows[0].values["fld_secret"]; ok {
		t.Error("a Field the declaration does not name was read")
	}
	if _, ok := rows[0].values["fld_flag"]; ok {
		t.Error("an undeclared boolean was written as false")
	}
	if rows[1].values["fld_label"] != "C" {
		t.Errorf("second row = %+v", rows[1])
	}
}
