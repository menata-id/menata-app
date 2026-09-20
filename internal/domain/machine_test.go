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

// TestFieldTypeGroup_isNotAReference pins the invariant the whole group Field type rests on.
//
// IsReference() means exactly one thing to its callers: "relations[f.RelatedMachine] holds this
// field's options" (internal/composition.Loader.RelationOptions, rendering's fieldInput and
// RelationLabel). A Group has no Machine, so if this ever returned true the picker would look up a
// Machine that does not exist, find nothing, and render an empty <select> -- working markup,
// silently offering no choices. That is a failure nobody would see, which is why it is pinned here
// rather than left to the type's doc comment.
func TestFieldTypeGroup_isNotAReference(t *testing.T) {
	f := Field{ID: "fld_approver_group", Name: "Approver Group", Type: FieldTypeGroup}
	if f.IsReference() {
		t.Error("a group Field must not be a reference: it has no target Machine, so relation option lookup would silently find nothing")
	}
	if f.RelatedMachine != "" {
		t.Errorf("RelatedMachine = %q, want empty -- a Group is a platform record, not a Machine", f.RelatedMachine)
	}
	if !KnownFieldTypes[FieldTypeGroup] {
		t.Error("group must be in KnownFieldTypes, or metadata declaring one is rejected at load time")
	}
}
