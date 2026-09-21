package authorization

import (
	"path/filepath"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
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

// realStepMachine is mch_approval_step exactly as metadata/approval_step.yaml declares it.
//
// It exists because every other fixture in this package hand-builds a Machine, and a hand-built
// Machine states the rule the test author believed rather than the rule the runtime will apply.
// That gap is not hypothetical here: the two render tests added in Fase 6c-2 built a step Machine
// with NO Permissions at all, and AllowsAction leaves an action unrestricted when a Machine
// declares none -- so both tests only ever exercised the allowed branch, and the bug below was
// invisible to the whole suite until someone read the metadata.
func realStepMachine(t *testing.T) *domain.Machine {
	t.Helper()
	app, err := metadata.LoadApplication(filepath.Join("..", "..", "metadata", "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication: %v", err)
	}
	for _, m := range app.Machines {
		if m.ID == "mch_approval_step" {
			return m
		}
	}
	t.Fatal("mch_approval_step is not declared in the real manifest")
	return nil
}

// TestAllowsAction_groupHeldStepIsActionableByItsGroup is the regression test for a bug Fase 6c-2
// created and Fase 6c-3 closed.
//
// 6c-1 gave `decide` a dynamic actor gate; `edit` and `delete` kept `actor_field: fld_assignee`
// alone. That cost nothing while nothing could write a Group-held step. 6c-2's wizard could -- and
// such a step leaves fld_assignee empty, so a gate reading only actor_field returned false for
// EVERYONE. The signature marker on the one screen built for dragging it was draggable by nobody,
// and the step could not be deleted by anybody either.
//
// It runs against the real manifest deliberately: the failure was a missing declaration, and a
// hand-built fixture would simply have declared it.
func TestAllowsAction_groupHeldStepIsActionableByItsGroup(t *testing.T) {
	step := realStepMachine(t)
	groupStep := map[string]any{
		"fld_approver_type":  domain.ActorKindGroup,
		"fld_approver_group": "grp_legal",
	}
	// Both hold a role the step's Permissions require (CAP-P01, Fase 7), so this test keeps
	// asking the question it was written for -- does the *Group* arm work -- rather than
	// accidentally becoming a test of the role arm added beside it.
	member := domain.Actor{ID: "usr_budi", Groups: map[string]bool{"grp_legal": true}, Roles: approverRoles}
	outsider := domain.Actor{ID: "usr_ana", Groups: map[string]bool{"grp_finance": true}, Roles: approverRoles}

	for _, act := range []string{domain.ActionDecide, domain.ActionEdit, domain.ActionDelete} {
		t.Run(act, func(t *testing.T) {
			if !AllowsAction(step, act, groupStep, member) {
				t.Errorf("a member of the owning Group must be allowed to %s a Group-held step", act)
			}
			if AllowsAction(step, act, groupStep, outsider) {
				t.Errorf("someone outside the owning Group must not be allowed to %s it", act)
			}
		})
	}
}

// The other direction, also against real metadata: widening edit/delete to Groups must not have
// widened anything for a step held by a person. Every Approval Step written before CAP-F24 is this
// case, and it is the one that would break silently.
func TestAllowsAction_personHeldStepUnchangedByTheGroupArm(t *testing.T) {
	step := realStepMachine(t)
	legacyStep := map[string]any{"fld_assignee": "usr_rina"}
	rina := domain.Actor{ID: "usr_rina", Roles: approverRoles}
	// In every Group, and still not this step's approver.
	joiner := domain.Actor{ID: "usr_budi", Groups: map[string]bool{"grp_legal": true, "grp_finance": true}, Roles: approverRoles}

	for _, act := range []string{domain.ActionDecide, domain.ActionEdit, domain.ActionDelete} {
		t.Run(act, func(t *testing.T) {
			if !AllowsAction(step, act, legacyStep, rina) {
				t.Errorf("the named person must still be allowed to %s their own step", act)
			}
			if AllowsAction(step, act, legacyStep, joiner) {
				t.Errorf("group membership must grant nothing on a person-held step (%s)", act)
			}
		})
	}
}

// approverRoles is the effective-role map of someone holding `approver` in Document Approval --
// the shape internal/web.currentActor builds from data.EffectiveRoles, keyed by Application id.
//
// Named rather than inlined because both CAP-F24 tests above need it now: mch_approval_step's
// Permissions gained a `roles:` arm in Fase 7, so an actor with no roles at all fails them for a
// reason those tests are not about. That they had to change is the evidence the arm is live --
// they run against the real manifest, so nothing here could have declared it into existence.
var approverRoles = map[string][]string{"app_document_approval": {"approver"}}

// TestAllowsAction_roleArmGatesTheAssigneeToo is CAP-P01's own test, and it asks the one question
// the two above deliberately do not: being the person a record names is no longer sufficient.
//
// Against the real manifest, for the same reason the others are -- the failure this guards is a
// missing `roles:` declaration, which a hand-built Machine would simply have declared.
func TestAllowsAction_roleArmGatesTheAssigneeToo(t *testing.T) {
	step := realStepMachine(t)
	ownStep := map[string]any{"fld_assignee": "usr_rina"}

	withRole := domain.Actor{ID: "usr_rina", Roles: approverRoles}
	// The same person, holding a role that exists in the vocabulary but is not one this
	// Permission names.
	wrongRole := domain.Actor{ID: "usr_rina", Roles: map[string][]string{"app_document_approval": {"submitter"}}}
	// The same person and the right word, held in the wrong Application. Role vocabularies are
	// per-Application on purpose (domain.Application.Roles); a Workspace-wide namespace would
	// make this grant succeed.
	wrongApp := domain.Actor{ID: "usr_rina", Roles: map[string][]string{"app_project_management": {"approver"}}}
	noRole := domain.Actor{ID: "usr_rina"}

	for _, act := range []string{domain.ActionDecide, domain.ActionEdit, domain.ActionDelete} {
		t.Run(act, func(t *testing.T) {
			if !AllowsAction(step, act, ownStep, withRole) {
				t.Errorf("the assignee holding a granted role must be allowed to %s their own step", act)
			}
			for name, actor := range map[string]domain.Actor{
				"a role the permission does not name":   wrongRole,
				"the right role in another application": wrongApp,
				"no role at all":                        noRole,
			} {
				if AllowsAction(step, act, ownStep, actor) {
					t.Errorf("%s must not be allowed to %s their own step (%s)", actor.ID, act, name)
				}
			}
		})
	}
}

// TestAllowsAction_reviewerMayNotDecide holds an owner decision (2026-09-21): *a reviewer can
// only look -- they cannot submit and they cannot approve.*
//
// This test is the inverse of the one it replaces. That one asserted a reviewer assigned to a
// step could decide it, which was true while mch_approval_step named `[approver, reviewer]` --
// and that declaration was itself the problem the decision settles: approver and reviewer granted
// exactly the same thing everywhere they appeared, so the two words were indistinguishable to
// anyone reading the Members screen, while submitter granted nothing at all.
//
// Against the real manifest, because what it guards is a declaration: putting `reviewer` back
// into that roles: list is a one-word edit that no other test would notice.
func TestAllowsAction_reviewerMayNotDecide(t *testing.T) {
	step := realStepMachine(t)
	// Assigned to this very step, which is what makes the assertion about the *role* and nothing
	// else: every other arm passes.
	ownStep := map[string]any{"fld_assignee": "usr_rina"}
	reviewer := domain.Actor{ID: "usr_rina", Roles: map[string][]string{"app_document_approval": {"reviewer"}}}

	for _, act := range []string{domain.ActionDecide, domain.ActionEdit, domain.ActionDelete} {
		if AllowsAction(step, act, ownStep, reviewer) {
			t.Errorf("a reviewer may only look -- %s on their own assigned step must still be refused", act)
		}
	}

	// And the role that does grant it still does, so this is a narrowing rather than a lockout.
	if !AllowsAction(step, domain.ActionDecide, ownStep, domain.Actor{ID: "usr_rina", Roles: approverRoles}) {
		t.Error("an approver assigned to the step must still be able to decide it")
	}
}

// TestDocumentCreateExcludesReviewer is the submit half of the same decision, on the Machine that
// carries it. A reviewer holding a real role in this Application still may not create a Document,
// even one naming themselves -- which is the arm that would otherwise let them through.
func TestDocumentCreateExcludesReviewer(t *testing.T) {
	app, err := metadata.LoadApplication(filepath.Join("..", "..", "metadata", "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication: %v", err)
	}
	var document *domain.Machine
	for _, m := range app.Machines {
		if m.ID == "mch_document" {
			document = m
		}
	}
	if document == nil {
		t.Fatal("mch_document is not declared in the real manifest")
	}
	own := map[string]any{"fld_submitted_by": "usr_rina"}

	reviewer := domain.Actor{ID: "usr_rina", Roles: map[string][]string{"app_document_approval": {"reviewer"}}}
	if AllowsAction(document, domain.ActionCreate, own, reviewer) {
		t.Error("a reviewer may only look -- submitting a document must be refused")
	}
	submitter := domain.Actor{ID: "usr_rina", Roles: map[string][]string{"app_document_approval": {"submitter"}}}
	if !AllowsAction(document, domain.ActionCreate, own, submitter) {
		t.Error("a submitter creating a document in their own name must be allowed")
	}
}
