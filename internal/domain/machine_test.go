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
