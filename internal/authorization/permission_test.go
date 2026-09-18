package authorization

import (
	"testing"

	"menata.app/internal/domain"
)

func stepMachine(perms ...domain.Permission) *domain.Machine {
	return &domain.Machine{
		ID:   "mch_approval_step",
		Name: "Approval Step",
		Fields: []domain.Field{
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		},
		Permissions: perms,
	}
}

var decideOwnStep = domain.Permission{ID: "prm_decide_own_step", Action: domain.ActionDecide, ActorField: "fld_assignee"}

func TestAllowsAction_actorIsAssignee(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if !AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, "rec_rina") {
		t.Fatal("assignee must be allowed to decide their own step")
	}
}

func TestAllowsAction_actorIsSomeoneElse(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, "rec_maya") {
		t.Fatal("a non-assignee must not be allowed to decide someone else's step")
	}
}

func TestAllowsAction_unidentifiedActor(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, "") {
		t.Fatal("an unidentified caller is not an actor and must be denied")
	}
}

func TestAllowsAction_unassignedRecord(t *testing.T) {
	for name, values := range map[string]map[string]any{
		"empty value":  {"fld_assignee": ""},
		"absent field": {},
		"wrong shape":  {"fld_assignee": 42},
	} {
		if AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, "rec_rina") {
			t.Errorf("%s: an unassigned record must be actionable by no one", name)
		}
	}
}

func TestAllowsAction_noPermissionDeclared(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if !AllowsAction(stepMachine(), domain.ActionDecide, values, "rec_maya") {
		t.Fatal("a Machine declaring no Permission leaves the action unrestricted")
	}
}

func TestAllowsAction_otherActionUnaffected(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if !AllowsAction(stepMachine(decideOwnStep), "submit", values, "rec_maya") {
		t.Fatal("a Permission must only govern the action it names")
	}
}
