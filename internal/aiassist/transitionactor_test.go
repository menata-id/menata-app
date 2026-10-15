package aiassist

import (
	"slices"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// withGatedMove adds a person Field and a status move limited by role and by that person to the fixture's second
// Machine, the way the assistant is now allowed to.
func withGatedMove(a *GeneratedApplication, roles []string, actorField string) {
	m := &a.Machines[1]
	m.Fields = append(m.Fields, GeneratedField{ID: "fld_pelapor", Name: "Pelapor", Type: "person"}, GeneratedField{ID: "fld_catatan", Name: "Catatan", Type: "text"})
	m.Transitions = []GeneratedTransition{{ID: "trn_send", Name: "Kirim", Field: "fld_status", From: "draft", To: "sent", Roles: roles, ActorField: actorField}}
}

func TestTransitionActorIsWrittenLoadedAndDescribedBack(t *testing.T) {
	manifestPath := installedWasteApp(t)
	loaded := publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		withGatedMove(a, []string{"submitter"}, "fld_pelapor")
	}))
	var got domain.Transition
	for _, m := range loaded.Machines {
		if m.ID == "mch_pencatatan_sampah" && len(m.Transitions) == 1 {
			got = m.Transitions[0]
		}
	}
	if !slices.Equal(got.Roles, []string{"submitter"}) || got.ActorField != "fld_pelapor" || got.Action != domain.ActionEdit {
		t.Fatalf("the loaded edge = %+v, want roles [submitter], actor_field fld_pelapor, action edit", got)
	}
	state, _ := stateOf(t, manifestPath)
	back := DescribeApplication("app_pengelolaan_sampah", state.Applications["app_pengelolaan_sampah"])
	tr := back.Machines[1].Transitions
	if len(tr) != 1 || !slices.Equal(tr[0].Roles, []string{"submitter"}) || tr[0].ActorField != "fld_pelapor" {
		t.Fatalf("the described edge = %+v, want the roles and actor_field it was written with", tr)
	}
}

func TestTransitionActorIsHeldToTheLoadersRules(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles []string
		actor string
		want  string
	}{
		{"a role the application does not declare", []string{"bos"}, "", "bos"},
		{"an actor_field that is not a person", nil, "fld_catatan", "fld_catatan"},
		{"an actor_field the machine lacks", nil, "fld_hilang", "fld_hilang"},
	} {
		manifestPath := installedWasteApp(t)
		change := update(t, manifestPath, func(a *GeneratedApplication) { withGatedMove(a, tc.roles, tc.actor) })
		state, _ := stateOf(t, manifestPath)
		err := Validate(change, state)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate = %v, want an error naming %q", tc.name, err, tc.want)
		}
	}
}

// Who may take an existing step is not changed by an update: the file edit only appends, so a change would be
// reported in the plan and never written.
func TestAnUpdateCannotChangeWhoMayTakeAStep(t *testing.T) {
	manifestPath := installedWasteApp(t)
	publish(t, manifestPath, update(t, manifestPath, func(a *GeneratedApplication) {
		withGatedMove(a, []string{"submitter"}, "fld_pelapor")
	}))
	for name, edit := range map[string]func(*GeneratedTransition){
		"roles":       func(x *GeneratedTransition) { x.Roles = []string{"approver"} },
		"roles gone":  func(x *GeneratedTransition) { x.Roles = nil },
		"actor_field": func(x *GeneratedTransition) { x.ActorField = "" },
	} {
		change := update(t, manifestPath, func(a *GeneratedApplication) { edit(&a.Machines[1].Transitions[0]) })
		state, _ := stateOf(t, manifestPath)
		plan := PlanUpdate(DescribeApplication(change.TargetAppID, state.Applications[change.TargetAppID]), *change.Application)
		if len(plan.Refusals) != 1 || !strings.Contains(plan.Refusals[0], "trn_send") {
			t.Errorf("%s: refusals = %v, want one naming trn_send", name, plan.Refusals)
		}
	}
	same := update(t, manifestPath, func(a *GeneratedApplication) {})
	state, _ := stateOf(t, manifestPath)
	if plan := PlanUpdate(DescribeApplication(same.TargetAppID, state.Applications[same.TargetAppID]), *same.Application); len(plan.Refusals) != 0 {
		t.Errorf("an unchanged step was refused: %v", plan.Refusals)
	}
}

func TestNewApplicationStepRolesMustBeDeclared(t *testing.T) {
	good := validLeaveRequestChange()
	good.Application.Machines[0].Transitions[0].Roles = []string{"Supervisor"}
	if err := Validate(good, ExistingState{}); err != nil {
		t.Fatalf("a declared role on a step was refused: %v", err)
	}
	bad := validLeaveRequestChange()
	bad.Application.Machines[0].Transitions[0].Roles = []string{"Ghost"}
	if err := Validate(bad, ExistingState{}); err == nil || !strings.Contains(err.Error(), "Ghost") {
		t.Fatalf("an undeclared role on a step = %v, want an error naming it", err)
	}
}

// A NEW application takes the other write path (a whole file, not an appended line), so it is held separately.
func TestNewApplicationWritesWhoMayTakeAStep(t *testing.T) {
	manifestPath := installedWasteApp(t)
	change := GeneratedChange{Kind: KindNewApplication, Application: &GeneratedApplication{
		ID: "app_cuti", Name: "Cuti", Roles: []string{"manajer", "karyawan"}, PublisherRole: "manajer",
		Navigation: []GeneratedMenuItem{{Label: "Cuti", MachineID: "mch_cuti"}},
		Machines: []GeneratedMachine{{ID: "mch_cuti", Name: "Cuti",
			Fields: []GeneratedField{
				{ID: "fld_manajer", Name: "Manajer", Type: "person"},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"diajukan", "disetujui"}},
			},
			Permissions: []GeneratedPermission{{ID: "prm_cuti_create", Action: "create", Roles: []string{"karyawan"}}},
			Transitions: []GeneratedTransition{{ID: "trn_setujui", Name: "Setujui", Field: "fld_status", From: "diajukan", To: "disetujui", Roles: []string{"manajer"}, ActorField: "fld_manajer"}},
		}},
	}}
	loaded := publish(t, manifestPath, change)
	for _, m := range loaded.Machines {
		if m.ID != "mch_cuti" {
			continue
		}
		got := m.Transitions[0]
		if !slices.Equal(got.Roles, []string{"manajer"}) || got.ActorField != "fld_manajer" {
			t.Fatalf("the written edge = %+v, want roles [manajer] and actor_field fld_manajer", got)
		}
		return
	}
	t.Fatal("the new Machine was not loaded")
}
