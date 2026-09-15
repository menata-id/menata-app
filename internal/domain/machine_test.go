package domain

import "testing"

func TestField_IsReference(t *testing.T) {
	cases := []struct {
		name string
		f    Field
		want bool
	}{
		{"relation with target", Field{Type: FieldTypeRelation, RelatedMachine: "mch_project"}, true},
		{"person (normalized to mch_user by Parse)", Field{Type: FieldTypePerson, RelatedMachine: UserMachineID}, true},
		{"text", Field{Type: FieldTypeText}, false},
		{"relation with empty target (invalid metadata, not yet caught)", Field{Type: FieldTypeRelation}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.f.IsReference(); got != c.want {
				t.Errorf("IsReference() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestMachine_FieldByID(t *testing.T) {
	m := &Machine{Fields: []Field{{ID: "fld_title", Name: "Title"}}}

	if f, ok := m.FieldByID("fld_title"); !ok || f.Name != "Title" {
		t.Errorf("FieldByID(fld_title) = (%+v, %v), want (Title, true)", f, ok)
	}
	if _, ok := m.FieldByID("fld_ghost"); ok {
		t.Error("FieldByID(fld_ghost) ok = true, want false")
	}
}
