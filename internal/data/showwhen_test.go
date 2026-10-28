package data

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// A required Field that is not shown (`show_when:`) is not missing: a record of another kind is valid without it, a record of the kind that shows it is not.
func TestAFieldNotShownIsNotRequired(t *testing.T) {
	m := &domain.Machine{ID: "mch_paper", Fields: []domain.Field{
		{ID: "fld_kind", Name: "Kind", Type: domain.FieldTypeStatus, Options: []string{"Contract", "Invoice"}, Required: true},
		{ID: "fld_ref", Name: "Contract number", Type: domain.FieldTypeText, Required: true, ShowWhen: &domain.FieldCondition{Field: "fld_kind", Equals: "Contract"}},
	}}
	if err := ValidateRecord(m, map[string]any{"fld_kind": "Invoice"}); err != nil {
		t.Errorf("an Invoice without a contract number: %v", err)
	}
	if err := ValidateRecord(m, map[string]any{"fld_kind": "Contract"}); err == nil || !strings.Contains(err.Error(), "fld_ref") {
		t.Errorf("a Contract without its number was accepted: %v", err)
	}
	if err := ValidateRecord(m, map[string]any{"fld_kind": "Contract", "fld_ref": "C-1"}); err != nil {
		t.Errorf("a Contract with its number: %v", err)
	}
	if err := ValidateRecord(m, map[string]any{"fld_kind": "Invoice", "fld_ref": "C-1"}); err != nil {
		t.Errorf("a value kept on a Field that is not shown is not an error: %v", err)
	}
}
