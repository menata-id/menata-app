package composition

import (
	"testing"

	"menata.app/internal/action"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

func machineMap(ids ...string) map[string]*domain.Machine {
	m := make(map[string]*domain.Machine, len(ids))
	for _, id := range ids {
		m[id] = &domain.Machine{ID: id}
	}
	return m
}

// TestMachineSliceIsStable locks the defect Phase 18 Step 2's before/after HTML comparison found:
// Go randomizes map iteration, so an unsorted slice made a record's child sections render in a
// different order on every reload. Ten runs is enough -- with 5 entries, a random order repeats
// the same sequence by chance only rarely, and a regression here shows up immediately.
func TestMachineSliceIsStable(t *testing.T) {
	l := NewLoader(nil, machineMap("mch_task", "mch_project", "mch_activity", "mch_user", "mch_list"))

	want := l.machineSlice()
	if len(want) != 5 {
		t.Fatalf("got %d machines, want 5", len(want))
	}
	for run := 0; run < 10; run++ {
		got := l.machineSlice()
		for i := range got {
			if got[i].ID != want[i].ID {
				t.Fatalf("run %d: order changed at %d: got %s, want %s", run, i, got[i].ID, want[i].ID)
			}
		}
	}
}

func TestMachineSliceIsSortedByID(t *testing.T) {
	l := NewLoader(nil, machineMap("mch_user", "mch_activity", "mch_task"))

	got := l.machineSlice()
	want := []string{"mch_activity", "mch_task", "mch_user"}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("position %d: got %s, want %s", i, got[i].ID, id)
		}
	}
}

func TestDisplayString(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"nil is empty", nil, ""},
		{"string passes through", "in_review", "in_review"},
		{"number is rendered", float64(3), "3"},
		{"bool is rendered", true, "true"},
		{"slice is rendered as JSON", []any{"a", "b"}, `["a","b"]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DisplayString(tc.value); got != tc.want {
				t.Errorf("DisplayString(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// ApproverOptions is the fix for a hole the roles decision of 2026-09-21 opened: an unfiltered
// picker lets a submitter name someone who can never decide, producing a step that looks normal
// and is actionable by nobody.
func TestApproverOptions(t *testing.T) {
	stepMachine := &domain.Machine{
		ID:            action.StepMachineID,
		ApplicationID: "app_document_approval",
		Permissions: []domain.Permission{{
			ID: "prm_decide_own_step", Action: domain.ActionDecide, Roles: []string{"approver"},
		}},
	}
	all := rendering.RelationOptions{
		domain.UserMachineID: []rendering.RelationOption{
			{ID: "usr_ana", Label: "Ana"},   // approver, directly
			{ID: "usr_budi", Label: "Budi"}, // approver, through a Group
			{ID: "usr_rina", Label: "Rina"}, // submitter only
			{ID: "usr_maya", Label: "Maya"}, // no role in this Application at all
		},
		"mch_document": []rendering.RelationOption{{ID: "doc_1", Label: "Contract"}},
	}
	members := []data.Membership{
		{UserRecordID: "usr_ana", AppRoles: map[string]string{"app_document_approval": "approver"}},
		{UserRecordID: "usr_budi", Groups: []data.Group{{Grants: map[string]string{"app_document_approval": "approver"}}}},
		{UserRecordID: "usr_rina", AppRoles: map[string]string{"app_document_approval": "submitter"}},
		{UserRecordID: "usr_maya"},
	}

	got := ApproverOptions(all, stepMachine, members)
	var kept []string
	for _, o := range got[domain.UserMachineID] {
		kept = append(kept, o.ID)
	}
	if want := []string{"usr_ana", "usr_budi"}; len(kept) != 2 || kept[0] != want[0] || kept[1] != want[1] {
		t.Errorf("kept = %v, want %v -- a role held through a Group qualifies exactly as a direct one does", kept, want)
	}
	// Only the person list is narrowed. Every other reference field is untouched, because this is
	// one case rather than a general rule about pickers.
	if len(got["mch_document"]) != 1 {
		t.Error("only the mch_user list is narrowed")
	}
}

// A Machine whose decide Permission names no role narrows nothing -- "no rule declared" means
// unrestricted here as it does everywhere else (001 Principle #6), not "nobody qualifies".
func TestApproverOptions_noRoleRuleKeepsEveryone(t *testing.T) {
	stepMachine := &domain.Machine{
		ID:          action.StepMachineID,
		Permissions: []domain.Permission{{ID: "prm_decide_own_step", Action: domain.ActionDecide, ActorField: "fld_assignee"}},
	}
	all := rendering.RelationOptions{domain.UserMachineID: []rendering.RelationOption{{ID: "usr_ana"}, {ID: "usr_rina"}}}

	if got := ApproverOptions(all, stepMachine, nil); len(got[domain.UserMachineID]) != 2 {
		t.Errorf("kept %d, want both -- an unrestricted action must not empty the picker", len(got[domain.UserMachineID]))
	}
}
