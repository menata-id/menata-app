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
	if !AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, domain.Actor{ID: "rec_rina"}) {
		t.Fatal("assignee must be allowed to decide their own step")
	}
}

func TestAllowsAction_actorIsSomeoneElse(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, domain.Actor{ID: "rec_maya"}) {
		t.Fatal("a non-assignee must not be allowed to decide someone else's step")
	}
}

func TestAllowsAction_unidentifiedActor(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, domain.Actor{ID: ""}) {
		t.Fatal("an unidentified caller is not an actor and must be denied")
	}
}

func TestAllowsAction_unassignedRecord(t *testing.T) {
	for name, values := range map[string]map[string]any{
		"empty value":  {"fld_assignee": ""},
		"absent field": {},
		"wrong shape":  {"fld_assignee": 42},
	} {
		if AllowsAction(stepMachine(decideOwnStep), domain.ActionDecide, values, domain.Actor{ID: "rec_rina"}) {
			t.Errorf("%s: an unassigned record must be actionable by no one", name)
		}
	}
}

func TestAllowsAction_noPermissionDeclared(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if !AllowsAction(stepMachine(), domain.ActionDecide, values, domain.Actor{ID: "rec_maya"}) {
		t.Fatal("a Machine declaring no Permission leaves the action unrestricted")
	}
}

func TestAllowsAction_otherActionUnaffected(t *testing.T) {
	values := map[string]any{"fld_assignee": "rec_rina"}
	if !AllowsAction(stepMachine(decideOwnStep), "submit", values, domain.Actor{ID: "rec_maya"}) {
		t.Fatal("a Permission must only govern the action it names")
	}
}

// --- CAP-F24: a record picks its own actor kind (Fase 6c-1) ---

// dynamicStepMachine is mch_approval_step as metadata/approval_step.yaml now declares it: the
// legacy person Field, the status Field that selects an arm, and the group Field.
//
// Note fld_assignee is BOTH ActorField and ActorUserField, exactly as the real metadata declares
// it. That is not a shortcut in the fixture -- it is the shape, and it is why the User arm and the
// legacy fallback can never disagree about who a step belongs to.
func dynamicStepMachine() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_approval_step",
		Name: "Approval Step",
		Fields: []domain.Field{
			{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
			{ID: "fld_approver_type", Name: "Approver Type", Type: domain.FieldTypeStatus, Options: []string{domain.ActorKindUser, domain.ActorKindGroup}},
			{ID: "fld_approver_group", Name: "Approver Group", Type: domain.FieldTypeGroup},
		},
		Permissions: []domain.Permission{{
			ID:         "prm_decide_own_step",
			Action:     domain.ActionDecide,
			ActorField: "fld_assignee",
			DynamicActor: &domain.DynamicActorGate{
				ActorTypeField:  "fld_approver_type",
				ActorUserField:  "fld_assignee",
				ActorGroupField: "fld_approver_group",
			},
		}},
	}
}

// The three paths, each proven in both directions. A gate that only ever says yes proves nothing,
// so every case below has its negative twin -- the same shape upstream's own conformance run used
// (T240-T242, "all six positive+negative pairs").
func TestAllowsAction_dynamicActorGate(t *testing.T) {
	rina := domain.Actor{ID: "rec_rina"}
	maya := domain.Actor{ID: "rec_maya"}
	legalMember := domain.Actor{ID: "rec_budi", Groups: map[string]bool{"grp_legal": true}}
	outsider := domain.Actor{ID: "rec_ana", Groups: map[string]bool{"grp_finance": true}}

	userStep := map[string]any{
		"fld_approver_type": domain.ActorKindUser,
		"fld_assignee":      "rec_rina",
	}
	groupStep := map[string]any{
		"fld_approver_type":  domain.ActorKindGroup,
		"fld_assignee":       "rec_rina", // set, and deliberately ignored by the Group arm
		"fld_approver_group": "grp_legal",
	}
	// No fld_approver_type at all: every Approval Step written before this capability existed.
	legacyStep := map[string]any{"fld_assignee": "rec_rina"}

	for _, tc := range []struct {
		name   string
		values map[string]any
		actor  domain.Actor
		want   bool
	}{
		{"User arm: the named person decides", userStep, rina, true},
		{"User arm: anyone else is refused", userStep, maya, false},
		{"Group arm: a member of the named Group decides", groupStep, legalMember, true},
		{"Group arm: a non-member is refused, even in another Group", groupStep, outsider, false},
		{"legacy record: the assignee still decides", legacyStep, rina, true},
		{"legacy record: anyone else is still refused", legacyStep, maya, false},

		// The Group arm does not fall back to fld_assignee. If it did, a step handed to the Legal
		// Group would still be decidable by whoever happened to be its original assignee -- an
		// access path nobody declared, and invisible because the happy path would look fine.
		{"Group arm: the assignee is NOT an implicit approver", groupStep, rina, false},
		// A member of the right Group has no business deciding a step the record says belongs to
		// one person.
		{"User arm: group membership grants nothing", userStep, legalMember, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AllowsAction(dynamicStepMachine(), domain.ActionDecide, tc.values, tc.actor); got != tc.want {
				t.Errorf("AllowsAction = %v, want %v", got, tc.want)
			}
		})
	}
}

// A gate that resolves to Group but names no Group is actionable by nobody -- the same rule an
// unassigned record already follows. The alternative, falling through to fld_assignee, would turn
// a half-filled form into a silent grant.
func TestAllowsAction_groupArmWithNoGroupNamed(t *testing.T) {
	anyone := domain.Actor{ID: "rec_rina", Groups: map[string]bool{"grp_legal": true}}
	for name, values := range map[string]map[string]any{
		"empty group":  {"fld_approver_type": domain.ActorKindGroup, "fld_assignee": "rec_rina", "fld_approver_group": ""},
		"absent group": {"fld_approver_type": domain.ActorKindGroup, "fld_assignee": "rec_rina"},
		"wrong shape":  {"fld_approver_type": domain.ActorKindGroup, "fld_assignee": "rec_rina", "fld_approver_group": 42},
	} {
		if AllowsAction(dynamicStepMachine(), domain.ActionDecide, values, anyone) {
			t.Errorf("%s: a Group-gated step naming no Group must be actionable by no one", name)
		}
	}
}

// An unrecognized actor-type value falls back rather than failing open OR closed arbitrarily.
// This is the case a typo in metadata or a half-migrated record produces, and it must behave like
// a record that never opted in -- not like one that granted everyone access.
func TestAllowsAction_unrecognizedActorTypeFallsBack(t *testing.T) {
	values := map[string]any{"fld_approver_type": "Team", "fld_assignee": "rec_rina"}
	if !AllowsAction(dynamicStepMachine(), domain.ActionDecide, values, domain.Actor{ID: "rec_rina"}) {
		t.Error("an unrecognized actor type must fall back to actor_field, so the assignee still decides")
	}
	if AllowsAction(dynamicStepMachine(), domain.ActionDecide, values, domain.Actor{ID: "rec_maya"}) {
		t.Error("...and the fallback must still refuse everyone else")
	}
}

// An Actor with no Groups map must not panic or match, which is what makes the zero Actor safe to
// pass from any call site that has not been taught to resolve groups yet.
func TestActor_zeroValueIsInNoGroup(t *testing.T) {
	if (domain.Actor{}).InGroup("grp_legal") {
		t.Error("the zero Actor belongs to nothing")
	}
	if (domain.Actor{ID: "rec_rina"}).InGroup("grp_legal") {
		t.Error("an Actor with a nil Groups map belongs to nothing")
	}
}
