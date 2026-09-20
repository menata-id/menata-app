package composition

import (
	"reflect"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

func TestProjectCardFields(t *testing.T) {
	m := &domain.Machine{
		ID: "mch_widget",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		},
		CardFields: []domain.CardField{
			{Field: "fld_title", Role: domain.CardFieldRoleTitle},
			{Field: "fld_owner", Role: domain.CardFieldRolePerson},
		},
	}
	r := rec("wdg_1", map[string]any{
		"fld_title": "Widget One",
		"fld_owner": "usr_ana",
	})
	relations := rendering.RelationOptions{
		"mch_user": {{ID: "usr_ana", Label: "Ana Putri"}},
	}

	got := ProjectCardFields(m, r, relations)
	want := []rendering.ProjectedField{
		{Label: "Title", Role: "title", Display: "Widget One"},
		{Label: "Owner", Role: "person", Display: "Ana Putri"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProjectCardFields() = %+v, want %+v", got, want)
	}
}

// TestProjectCardFields_skipsMissingField is the defensive case: card_fields: naming a Field
// id that no longer exists on m (stale metadata after a rename/removal that internal/metadata.
// Validate would already reject at load time) must not panic a live render -- it's skipped.
func TestProjectCardFields_skipsMissingField(t *testing.T) {
	m := &domain.Machine{
		ID:     "mch_widget",
		Fields: []domain.Field{{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText}},
		CardFields: []domain.CardField{
			{Field: "fld_ghost", Role: domain.CardFieldRoleTitle},
		},
	}
	r := rec("wdg_1", map[string]any{"fld_title": "Widget One"})

	got := ProjectCardFields(m, r, nil)
	if len(got) != 0 {
		t.Errorf("ProjectCardFields() = %+v, want empty (stale field id skipped)", got)
	}
}

func TestProjectCardFields_dateRole(t *testing.T) {
	m := &domain.Machine{
		ID:     "mch_widget",
		Fields: []domain.Field{{ID: "fld_due", Name: "Due", Type: domain.FieldTypeDate}},
		CardFields: []domain.CardField{
			{Field: "fld_due", Role: domain.CardFieldRoleDate},
		},
	}
	r := rec("wdg_1", map[string]any{"fld_due": "2026-09-19"})

	got := ProjectCardFields(m, r, nil)
	want := []rendering.ProjectedField{{Label: "Due", Role: "date", Display: "19 Sep 2026"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProjectCardFields() = %+v, want %+v", got, want)
	}
}
