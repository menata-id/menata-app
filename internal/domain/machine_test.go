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
	if _, ok := KnownFieldTypes[FieldTypeGroup]; !ok {
		t.Error("group must be in KnownFieldTypes, or metadata declaring one is rejected at load time")
	}
}

func TestReopenValue(t *testing.T) {
	m := &Machine{Fields: []Field{{ID: "f", Type: FieldTypeStatus, Options: []string{"a", "b", "z"}}}}
	if got := m.ReopenValue(); got != "" {
		t.Errorf("no completion declared: ReopenValue() = %q, want empty", got)
	}
	m.Completion = &Completion{Field: "f", Done: "z"}
	if got := m.ReopenValue(); got != "a" {
		t.Errorf("no default: ReopenValue() = %q, want the first option", got)
	}
	m.Fields[0].Default = "b"
	if got := m.ReopenValue(); got != "b" {
		t.Errorf("default b: ReopenValue() = %q, want the Field's default", got)
	}
}

func TestCopyTitleFieldIsDerivedFromWhatTheMachineAlreadyDeclares(t *testing.T) {
	title := []CardField{{Field: "fld_title", Role: CardFieldRoleTitle}}
	for _, tc := range []struct {
		name string
		m    Machine
		want string
	}{
		{"an ordinary Machine with a title", Machine{ID: "mch_task", CardFields: title}, "fld_title"},
		{"no title role: nothing to name a copy by", Machine{ID: "mch_task"}, ""},
		{"append-only: an audit trail is not copied", Machine{ID: "mch_comment", CardFields: title, AppendOnly: true}, ""},
		{"a workflow engine's Machine: its own screens write it", Machine{ID: "mch_document", CardFields: title, WorkflowRole: "document"}, ""},
		{"a runtime-level Machine", Machine{ID: UserMachineID, CardFields: title}, ""},
	} {
		if got := tc.m.CopyTitleField(); got != tc.want {
			t.Errorf("%s: CopyTitleField = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFieldTypeOrderIsExactlyKnownFieldTypes holds the ordered list against the map in both directions.
// Everything generated from the catalogue (the AI prompt's sentence, its schema Enum) ranges over the list,
// so a type in the map and missing here would reach the loader and never the model -- the drift K02 found.
func TestFieldTypeOrderIsExactlyKnownFieldTypes(t *testing.T) {
	seen := map[FieldType]bool{}
	for _, ft := range FieldTypes() {
		if seen[ft] {
			t.Errorf("FieldTypes lists %q twice", ft)
		}
		seen[ft] = true
		if _, ok := KnownFieldTypes[ft]; !ok {
			t.Errorf("FieldTypes lists %q, which KnownFieldTypes does not declare", ft)
		}
	}
	for ft := range KnownFieldTypes {
		if !seen[ft] {
			t.Errorf("KnownFieldTypes declares %q and FieldTypes does not list it -- the AI's prompt and schema would never offer it", ft)
		}
	}
}
