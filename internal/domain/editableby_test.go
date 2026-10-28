package domain

import "testing"

// A Field that names who may change it (`editable_by:`) is kept as it is for everyone else: an actor holding one of the roles in the Machine's own
// Application, or a Workspace admin, may change it; a member holding another role, no role, or nothing at all may not. The rule joins the state locks
// (LockedField) rather than replacing them, and a Field that names no one is open as it always was.
func TestLockedFieldForKeepsAFieldFromWhoMayNotChangeIt(t *testing.T) {
	m := &Machine{ID: "mch_x", ApplicationID: "app_x"}
	f := Field{ID: "fld_amount", Name: "Amount", EditableBy: []string{"reviewer"}}
	member := func(roles map[string][]string) Actor { return Actor{WorkspaceRole: WorkspaceRoleMember, Roles: roles} }
	for name, tc := range map[string]struct {
		actor  Actor
		locked bool
	}{
		"the named role":                  {member(map[string][]string{"app_x": {"reviewer"}}), false},
		"one of several roles":            {member(map[string][]string{"app_x": {"author", "reviewer"}}), false},
		"another role":                    {member(map[string][]string{"app_x": {"author"}}), true},
		"the role in another Application": {member(map[string][]string{"app_y": {"reviewer"}}), true},
		"no roles":                        {member(nil), true},
		"nobody identified":               {Actor{}, true},
		"a Workspace admin":               {Actor{WorkspaceRole: WorkspaceRoleAdmin}, false},
	} {
		if got, _ := m.LockedFieldFor(f, nil, tc.actor); got != tc.locked {
			t.Errorf("%s: locked = %v, want %v", name, got, tc.locked)
		}
	}
	open := Field{ID: "fld_note", Name: "Note"}
	if got, _ := m.LockedFieldFor(open, nil, Actor{}); got {
		t.Error("a Field that names no one is locked")
	}
	once := Field{ID: "fld_no", Name: "No", SetOnce: true, EditableBy: []string{"reviewer"}}
	if got, _ := m.LockedFieldFor(once, map[string]any{"fld_no": "7"}, member(map[string][]string{"app_x": {"reviewer"}})); !got {
		t.Error("editable_by lifted a set_once lock")
	}
	if _, why := m.LockedFieldFor(f, nil, member(nil)); why == "" {
		t.Error("a locked Field says nothing about why")
	}
}

// A list of values is a status Field no Transition names (the plan's Value List, measured composable): an edit form asks for it as a select of its
// options, so an ordinary choice such as a document's kind is editable without becoming a lifecycle; once a Transition names it, it is the lifecycle's
// and the form leaves it out.
func TestAStatusNoTransitionNamesIsAChoiceListTheEditFormAsks(t *testing.T) {
	m := &Machine{ID: "mch_x", Fields: []Field{
		{ID: "fld_kind", Name: "Kind", Type: FieldTypeStatus, Options: []string{"Contract", "Invoice"}},
		{ID: "fld_state", Name: "State", Type: FieldTypeStatus, Options: []string{"Open", "Closed"}},
	}, Transitions: []Transition{{ID: "trn_close", Field: "fld_state", From: "Open", To: "Closed"}}}
	asked := map[string]FormInput{}
	for _, in := range m.EditFormInputs(map[string]any{"fld_kind": "Invoice"}, Actor{WorkspaceRole: WorkspaceRoleAdmin}) {
		asked[in.FieldID] = in
	}
	if in, ok := asked["fld_kind"]; !ok || in.Kind != InputSelect || in.Default != "Invoice" || len(in.Options) != 2 {
		t.Errorf("a status no Transition names is not asked as a select of its options: %+v", in)
	}
	if _, ok := asked["fld_state"]; ok {
		t.Error("a status a Transition governs is asked as a free select")
	}
	if got := AddableFieldTypeLabel(FieldTypeStatus); got != "Choice list (or status)" {
		t.Errorf("the picker does not say a status can be a choice list: %q", got)
	}
}
