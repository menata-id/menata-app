package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

func validMachine() *domain.Machine {
	return &domain.Machine{
		ID:   "mch_task",
		Name: "Task",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText, Required: true},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}},
		},
	}
}

func TestValidate_valid(t *testing.T) {
	if err := Validate(validMachine()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_badMachineID(t *testing.T) {
	m := validMachine()
	m.ID = "task"
	assertIssue(t, m, "machine id")
}

func TestValidate_missingName(t *testing.T) {
	m := validMachine()
	m.Name = ""
	assertIssue(t, m, "name is required")
}

func TestValidate_badFieldID(t *testing.T) {
	m := validMachine()
	m.Fields[0].ID = "title"
	assertIssue(t, m, "field id")
}

func TestValidate_duplicateFieldID(t *testing.T) {
	m := validMachine()
	m.Fields[1].ID = m.Fields[0].ID
	assertIssue(t, m, "declared more than once")
}

func TestValidate_unknownType(t *testing.T) {
	m := validMachine()
	m.Fields[0].Type = "widget"
	assertIssue(t, m, "unknown type")
}

func TestValidate_statusWithoutOptions(t *testing.T) {
	m := validMachine()
	m.Fields[1].Options = nil
	assertIssue(t, m, "requires at least one option")
}

func TestValidate_statusDefaultNotAnOption(t *testing.T) {
	m := validMachine()
	m.Fields[1].Default = "archived"
	assertIssue(t, m, "is not one of its own options")
}

func TestValidate_statusDefaultValid(t *testing.T) {
	m := validMachine()
	m.Fields[1].Default = "done"
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_relationWithoutTarget(t *testing.T) {
	m := validMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_project", Name: "Project", Type: domain.FieldTypeRelation})
	assertIssue(t, m, "requires a valid target machine id")
}

func TestValidate_relationWithValidTarget(t *testing.T) {
	m := validMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_project", Name: "Project", Type: domain.FieldTypeRelation, RelatedMachine: "mch_project"})
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func validConstraint() domain.Constraint {
	return domain.Constraint{
		ID:         "cst_status_done",
		On:         "fld_status",
		WhenEquals: "done",
		BlockIf: domain.RelationBlock{
			RelatedMachine: "mch_project",
			RelatedField:   "fld_task",
			Condition:      expression.Comparison{Field: "fld_status", Op: expression.OpNotEquals, Value: "done"},
		},
	}
}

func TestValidate_constraintValid(t *testing.T) {
	m := validMachine()
	m.Constraints = []domain.Constraint{validConstraint()}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_constraintBadID(t *testing.T) {
	m := validMachine()
	c := validConstraint()
	c.ID = "status_done"
	m.Constraints = []domain.Constraint{c}
	assertIssue(t, m, "constraint id")
}

func TestValidate_constraintOnUnknownField(t *testing.T) {
	m := validMachine()
	c := validConstraint()
	c.On = "fld_ghost"
	m.Constraints = []domain.Constraint{c}
	assertIssue(t, m, "is not a field of machine")
}

func TestValidate_constraintWhenEqualsNotAnOption(t *testing.T) {
	m := validMachine()
	c := validConstraint()
	c.WhenEquals = "archived"
	m.Constraints = []domain.Constraint{c}
	assertIssue(t, m, "is not one of field")
}

func TestValidate_constraintUnknownOp(t *testing.T) {
	m := validMachine()
	c := validConstraint()
	c.BlockIf.Condition.Op = "greater_than"
	m.Constraints = []domain.Constraint{c}
	assertIssue(t, m, "is not a known operator")
}

func validEvent() domain.Event {
	return domain.Event{
		ID: "evt_task_status_changed",
		On: "fld_status",
		Then: domain.Service{
			Name:    domain.ServiceLogActivity,
			Summary: "moved from {old} to {new}",
		},
	}
}

func TestValidate_eventValid(t *testing.T) {
	m := validMachine()
	m.Events = []domain.Event{validEvent()}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_eventBadID(t *testing.T) {
	m := validMachine()
	e := validEvent()
	e.ID = "status_changed"
	m.Events = []domain.Event{e}
	assertIssue(t, m, "event id")
}

func TestValidate_eventDuplicateID(t *testing.T) {
	m := validMachine()
	m.Events = []domain.Event{validEvent(), validEvent()}
	assertIssue(t, m, "declared more than once")
}

func TestValidate_eventOnUnknownField(t *testing.T) {
	m := validMachine()
	e := validEvent()
	e.On = "fld_ghost"
	m.Events = []domain.Event{e}
	assertIssue(t, m, "is not a field of machine")
}

func TestValidate_eventWhenEqualsOptional(t *testing.T) {
	m := validMachine()
	e := validEvent() // WhenEquals left empty -- "any change fires it"
	m.Events = []domain.Event{e}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil: when_equals is optional for an Event", err)
	}
}

func TestValidate_eventWhenEqualsNotAnOption(t *testing.T) {
	m := validMachine()
	e := validEvent()
	e.WhenEquals = "archived"
	m.Events = []domain.Event{e}
	assertIssue(t, m, "is not one of field")
}

func TestValidate_eventUnknownService(t *testing.T) {
	m := validMachine()
	e := validEvent()
	e.Then.Name = "send_carrier_pigeon"
	m.Events = []domain.Event{e}
	assertIssue(t, m, "is not a service this runtime realizes")
}

func TestValidate_eventMissingSummary(t *testing.T) {
	m := validMachine()
	e := validEvent()
	e.Then.Summary = ""
	m.Events = []domain.Event{e}
	assertIssue(t, m, "then.summary is required")
}

func TestValidate_eventSummaryOverrideWithoutWhen(t *testing.T) {
	m := validMachine()
	e := validEvent()
	e.Then.SummaryOverride = "completed"
	m.Events = []domain.Event{e}
	assertIssue(t, m, "must be set together")
}

func TestValidate_eventSummaryOverrideWhenWithoutOverride(t *testing.T) {
	m := validMachine()
	e := validEvent()
	e.Then.SummaryOverrideWhen = "done"
	m.Events = []domain.Event{e}
	assertIssue(t, m, "must be set together")
}

func TestValidate_eventOnCreateValid(t *testing.T) {
	m := validMachine()
	m.Events = []domain.Event{{
		ID:       "evt_task_created",
		OnCreate: true,
		Then:     domain.Service{Name: domain.ServiceLogActivity, Summary: "created"},
	}}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_eventOnCreateWithOnFails(t *testing.T) {
	m := validMachine()
	e := validEvent() // On: "fld_status"
	e.OnCreate = true
	m.Events = []domain.Event{e}
	assertIssue(t, m, "on, on_create and schedule are mutually exclusive")
}

func TestValidate_eventOnCreateWithWhenEqualsFails(t *testing.T) {
	m := validMachine()
	m.Events = []domain.Event{{
		ID:         "evt_task_created",
		OnCreate:   true,
		WhenEquals: "done",
		Then:       domain.Service{Name: domain.ServiceLogActivity, Summary: "created"},
	}}
	assertIssue(t, m, "when_equals is not meaningful with on_create")
}

func TestValidate_eventNeitherOnNorOnCreateFails(t *testing.T) {
	m := validMachine()
	m.Events = []domain.Event{{
		ID:   "evt_task_created",
		Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "created"},
	}}
	assertIssue(t, m, "exactly one of on, on_create or schedule is required")
}

func machineWithDueDate() *domain.Machine {
	m := validMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_due_date", Name: "Due Date", Type: domain.FieldTypeDate})
	return m
}

func TestValidate_scheduleValid(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID: "evt_task_overdue",
		Schedule: &domain.Schedule{
			DateField: "fld_due_date", When: domain.ScheduleWhenOverdue,
			GuardField: "fld_status", GuardEquals: "done",
		},
		Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_scheduleValidWithNoGuard(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID:       "evt_task_overdue",
		Schedule: &domain.Schedule{DateField: "fld_due_date", When: domain.ScheduleWhenOverdue},
		Then:     domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil: guard_field/guard_equals are optional", err)
	}
}

func TestValidate_scheduleAndOnFails(t *testing.T) {
	m := machineWithDueDate()
	e := validEvent() // On: "fld_status"
	e.Schedule = &domain.Schedule{DateField: "fld_due_date", When: domain.ScheduleWhenOverdue}
	m.Events = []domain.Event{e}
	assertIssue(t, m, "on, on_create and schedule are mutually exclusive")
}

func TestValidate_scheduleWithWhenEqualsFails(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID:         "evt_task_overdue",
		Schedule:   &domain.Schedule{DateField: "fld_due_date", When: domain.ScheduleWhenOverdue},
		WhenEquals: "done",
		Then:       domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	assertIssue(t, m, "when_equals is not meaningful with schedule")
}

func TestValidate_scheduleDateFieldNotAField(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID:       "evt_task_overdue",
		Schedule: &domain.Schedule{DateField: "fld_nope", When: domain.ScheduleWhenOverdue},
		Then:     domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	assertIssue(t, m, "schedule.date_field \"fld_nope\" is not a field")
}

func TestValidate_scheduleDateFieldWrongType(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID:       "evt_task_overdue",
		Schedule: &domain.Schedule{DateField: "fld_status", When: domain.ScheduleWhenOverdue},
		Then:     domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	assertIssue(t, m, "schedule.date_field \"fld_status\" must be type date")
}

func TestValidate_scheduleUnknownWhen(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID:       "evt_task_overdue",
		Schedule: &domain.Schedule{DateField: "fld_due_date", When: "due_yesterday"},
		Then:     domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	assertIssue(t, m, "schedule.when \"due_yesterday\" is not a comparison this runtime knows")
}

func TestValidate_scheduleGuardFieldWithoutGuardEqualsFails(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID:       "evt_task_overdue",
		Schedule: &domain.Schedule{DateField: "fld_due_date", When: domain.ScheduleWhenOverdue, GuardField: "fld_status"},
		Then:     domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	assertIssue(t, m, "schedule.guard_field and schedule.guard_equals must be set together")
}

func TestValidate_scheduleGuardEqualsNotAnOptionFails(t *testing.T) {
	m := machineWithDueDate()
	m.Events = []domain.Event{{
		ID: "evt_task_overdue",
		Schedule: &domain.Schedule{
			DateField: "fld_due_date", When: domain.ScheduleWhenOverdue,
			GuardField: "fld_status", GuardEquals: "archived",
		},
		Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "overdue"},
	}}
	assertIssue(t, m, "schedule.guard_equals \"archived\" is not one of field \"fld_status\"'s options")
}

func machineWithAssignee() *domain.Machine {
	m := validMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID})
	return m
}

func validMemberRemovalBlock() domain.MemberRemovalBlock {
	return domain.MemberRemovalBlock{
		ID:         "blk_step_pending",
		ActorField: "fld_assignee",
		Condition:  expression.Comparison{Field: "fld_status", Op: expression.OpEquals, Value: "todo"},
		Reason:     "has a pending approval step",
	}
}

func TestValidate_memberRemovalBlockValid(t *testing.T) {
	m := machineWithAssignee()
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{validMemberRemovalBlock()}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_memberRemovalBlockBadID(t *testing.T) {
	m := machineWithAssignee()
	b := validMemberRemovalBlock()
	b.ID = "step_pending"
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{b}
	assertIssue(t, m, "member removal block id")
}

func TestValidate_memberRemovalBlockDuplicateID(t *testing.T) {
	m := machineWithAssignee()
	b := validMemberRemovalBlock()
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{b, b}
	assertIssue(t, m, "member removal block id \"blk_step_pending\" is declared more than once")
}

func TestValidate_memberRemovalBlockActorFieldNotAField(t *testing.T) {
	m := machineWithAssignee()
	b := validMemberRemovalBlock()
	b.ActorField = "fld_nope"
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{b}
	assertIssue(t, m, "actor_field \"fld_nope\" is not a field of machine")
}

func TestValidate_memberRemovalBlockActorFieldNotPerson(t *testing.T) {
	m := machineWithAssignee()
	b := validMemberRemovalBlock()
	b.ActorField = "fld_status"
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{b}
	assertIssue(t, m, "actor_field \"fld_status\" must be type person, got \"status\"")
}

func TestValidate_memberRemovalBlockConditionFieldNotAField(t *testing.T) {
	m := machineWithAssignee()
	b := validMemberRemovalBlock()
	b.Condition.Field = "fld_nope"
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{b}
	assertIssue(t, m, "condition.field \"fld_nope\" is not a field of machine")
}

func TestValidate_memberRemovalBlockUnknownOp(t *testing.T) {
	m := machineWithAssignee()
	b := validMemberRemovalBlock()
	b.Condition.Op = "greater_than"
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{b}
	assertIssue(t, m, "condition.op \"greater_than\" is not a known operator")
}

func TestValidate_memberRemovalBlockMissingReason(t *testing.T) {
	m := machineWithAssignee()
	b := validMemberRemovalBlock()
	b.Reason = ""
	m.MemberRemovalBlocks = []domain.MemberRemovalBlock{b}
	assertIssue(t, m, "reason is required")
}

func TestValidate_noViewsIsValid(t *testing.T) {
	m := validMachine() // no views: at all -- one implicit table, as before views existed
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_viewRequiresAName(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "vw_task_table", Type: domain.ViewTable}}
	assertIssue(t, m, "must declare a name")
}

func TestValidate_viewIDPattern(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "board", Type: domain.ViewTable}}
	assertIssue(t, m, "must match ^vw_")
}

func TestValidate_viewIDsAreUniqueWithinAMachine(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{
		{ID: "vw_task_table", Type: domain.ViewTable},
		{ID: "vw_task_table", Type: domain.ViewBoard, GroupBy: "fld_status"},
	}
	assertIssue(t, m, "is declared more than once")
}

func TestValidate_viewUnknownType(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "vw_task_carousel", Type: "carousel"}}
	assertIssue(t, m, "has unknown type")
}

func TestValidate_viewBoardValidGroupBy(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "vw_task_board", Name: "Board", Type: domain.ViewBoard, GroupBy: "fld_status"}}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_viewBoardUnknownGroupBy(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "vw_task_board", Type: domain.ViewBoard, GroupBy: "fld_ghost"}}
	assertIssue(t, m, "must be a field of this machine")
}

// group_by on a type that does not group is rejected rather than ignored: a declaration the
// runtime silently drops reads as if it were working, which is the failure mode this whole step
// exists to stop.
func TestValidate_viewGroupByOnANonBoard(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "vw_task_table", Type: domain.ViewTable, GroupBy: "fld_status"}}
	assertIssue(t, m, "means nothing here")
}

// The cross-check that makes the two halves connect: a cards View needs something to project.
func TestValidate_viewCardsRequiresCardFields(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "vw_task_cards", Type: domain.ViewCards}}
	assertIssue(t, m, "must declare card_fields")
}

func TestValidate_viewCardsWithCardFields(t *testing.T) {
	m := validMachine()
	m.CardFields = []domain.CardField{{Field: "fld_status", Role: domain.CardFieldRoleStatus}}
	m.Views = []domain.View{{ID: "vw_task_cards", Name: "Cards", Type: domain.ViewCards}}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_slaFieldValid(t *testing.T) {
	m := validMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_due_date", Name: "Due Date", Type: domain.FieldTypeDate})
	m.SLAField = "fld_due_date"
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_slaFieldUnknown(t *testing.T) {
	m := validMachine()
	m.SLAField = "fld_ghost"
	assertIssue(t, m, "is not a field of this machine")
}

func TestValidate_slaFieldNotADate(t *testing.T) {
	m := validMachine()
	m.SLAField = "fld_title"
	assertIssue(t, m, "must be a date field")
}

func TestValidate_cardFieldsValid(t *testing.T) {
	m := validMachine()
	m.CardFields = []domain.CardField{
		{Field: "fld_title", Role: domain.CardFieldRoleTitle},
		{Field: "fld_status", Role: domain.CardFieldRoleStatus},
	}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_cardFieldsUnknownField(t *testing.T) {
	m := validMachine()
	m.CardFields = []domain.CardField{{Field: "fld_ghost", Role: domain.CardFieldRoleTitle}}
	assertIssue(t, m, "is not a field of this machine")
}

func TestValidate_cardFieldsUnknownRole(t *testing.T) {
	m := validMachine()
	m.CardFields = []domain.CardField{{Field: "fld_title", Role: "carousel"}}
	assertIssue(t, m, "has unknown role")
}

func TestValidate_containerRoleNeedsARelationField(t *testing.T) {
	m := validMachine()
	m.CardFields = []domain.CardField{{Field: "fld_title", Role: domain.CardFieldRoleContainer}}
	assertIssue(t, m, "has the container role, so it must be a relation field")
}

// permissionMachine is a Machine shaped like mch_approval_step: a person Field an Action can be
// scoped to (development-history.md Phase 16).
func permissionMachine() *domain.Machine {
	m := validMachine()
	m.Fields = append(m.Fields, domain.Field{
		ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID,
	})
	m.Permissions = []domain.Permission{
		{ID: "prm_decide_own_step", Action: domain.ActionDecide, ActorField: "fld_assignee"},
	}
	return m
}

func TestValidate_permissionValid(t *testing.T) {
	if err := Validate(permissionMachine()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_permissionBadID(t *testing.T) {
	m := permissionMachine()
	m.Permissions[0].ID = "decide_own_step"
	assertIssue(t, m, "permission id")
}

func TestValidate_permissionDuplicateID(t *testing.T) {
	m := permissionMachine()
	m.Permissions = append(m.Permissions, m.Permissions[0])
	assertIssue(t, m, "declared more than once")
}

func TestValidate_permissionUnknownAction(t *testing.T) {
	m := permissionMachine()
	m.Permissions[0].Action = "publish"
	assertIssue(t, m, "is not an action this runtime realizes")
}

func TestValidate_permissionUnknownActorField(t *testing.T) {
	m := permissionMachine()
	m.Permissions[0].ActorField = "fld_nobody"
	assertIssue(t, m, "is not a field of machine")
}

// The three branches of unboundActorField (validate.go), one test each, because the message used to be
// one sentence for all three and was wrong in two of them: it read "must reference an identity (a person
// or relation field), got %q" and answered `got "person"` for a person field -- contradicting itself on
// one line. What is actually missing differs by type, and so does what an author should do about it.
func TestValidate_permissionActorFieldIsNotAReference(t *testing.T) {
	m := permissionMachine()
	m.Permissions[0].ActorField = "fld_title"
	// A text field can never carry a target, so naming the type *is* the answer -- the one case the old
	// wording got right.
	assertIssue(t, m, "cannot name an identity")
}

func TestValidate_permissionActorFieldIsARelationWithNoTarget(t *testing.T) {
	m := permissionMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_owner", Name: "Owner", Type: domain.FieldTypeRelation})
	m.Permissions[0].ActorField = "fld_owner"
	// The one case a hand-written manifest reaches: the type is right and `machine:` is missing, so the
	// message must point at the target rather than the type.
	assertIssue(t, m, "no `machine:` target")
}

// TestValidate_permissionActorFieldIsAnUnnormalisedPerson is the branch that only a Machine built in Go
// can reach: Normalize binds every person Field to mch_user, so through the loader this is impossible.
// Two test fixtures were in exactly this state on 2026-09-29, and the old message blamed the type.
func TestValidate_permissionActorFieldIsAnUnnormalisedPerson(t *testing.T) {
	m := permissionMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson})
	m.Permissions[0].ActorField = "fld_owner"
	assertIssue(t, m, "was not normalised")

	// And Normalize is what makes it valid -- the inference, not a hand-written RelatedMachine.
	if err := Validate(Normalize(m)); err != nil {
		t.Errorf("Validate(Normalize(m)) = %v, want nil -- normalising binds person to mch_user", err)
	}
}

func assertIssue(t *testing.T, m *domain.Machine, substr string) {
	t.Helper()
	err := Validate(m)
	if err == nil {
		t.Fatalf("Validate() error = nil, want error containing %q", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), substr)
	}
}

// dynamicActorMachine is permissionMachine plus a declared actor gate (CAP-F24, Fase 6c-1),
// shaped exactly like metadata/approval_step.yaml: fld_assignee serves as both actor_field and
// actor_user_field.
func dynamicActorMachine() *domain.Machine {
	m := permissionMachine()
	m.Fields = append(m.Fields,
		domain.Field{ID: "fld_approver_type", Name: "Approver Type", Type: domain.FieldTypeStatus,
			Options: []string{domain.ActorKindUser, domain.ActorKindGroup}},
		domain.Field{ID: "fld_approver_group", Name: "Approver Group", Type: domain.FieldTypeGroup},
	)
	m.Permissions[0].DynamicActor = &domain.DynamicActorGate{
		ActorTypeField:  "fld_approver_type",
		ActorUserField:  "fld_assignee",
		ActorGroupField: "fld_approver_group",
	}
	return m
}

func TestValidate_dynamicActorValid(t *testing.T) {
	if err := Validate(dynamicActorMachine()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

// Each of the three Fields is checked for type, not just existence, because a mis-typed one fails
// where nobody looks: authorization would compare an identity against whatever that Field holds,
// return false, and present as "this person may never approve anything" rather than as a metadata
// error. These cases are the load-time refusal that turns a silent denial into a startup failure.
func TestValidate_dynamicActorRejectsWrongShapes(t *testing.T) {
	for name, mutate := range map[string]func(*domain.Machine){
		"type field is not a status": func(m *domain.Machine) {
			m.Permissions[0].DynamicActor.ActorTypeField = "fld_assignee"
		},
		"type field does not declare the values the gate resolves": func(m *domain.Machine) {
			for i, f := range m.Fields {
				if f.ID == "fld_approver_type" {
					m.Fields[i].Options = []string{"person", "team"}
				}
			}
		},
		"group field is not a group": func(m *domain.Machine) {
			m.Permissions[0].DynamicActor.ActorGroupField = "fld_assignee"
		},
		"user field is not an identity": func(m *domain.Machine) {
			m.Permissions[0].DynamicActor.ActorUserField = "fld_approver_type"
		},
		"a named field does not exist": func(m *domain.Machine) {
			m.Permissions[0].DynamicActor.ActorGroupField = "fld_nope"
		},
		"partially declared gate": func(m *domain.Machine) {
			m.Permissions[0].DynamicActor.ActorGroupField = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := dynamicActorMachine()
			mutate(m)
			if err := Validate(m); err == nil {
				t.Error("Validate() = nil, want an error -- a gate the runtime cannot resolve protects nothing while looking configured")
			}
		})
	}
}

// A group Field names no Machine, and saying otherwise is refused rather than ignored: a
// declaration the runtime silently drops reads as though it were working.
func TestValidate_groupFieldRejectsAMachineTarget(t *testing.T) {
	m := dynamicActorMachine()
	for i, f := range m.Fields {
		if f.ID == "fld_approver_group" {
			m.Fields[i].RelatedMachine = "mch_user"
		}
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() = nil, want an error -- a Group is a Workspace platform record, so machine: could never resolve")
	}
}

// transitionMachine is the shape validateTransition is checked against: one status Field with
// three options, plus a text Field, so both "not a status field" and "not one of its options" have
// something real to fail on.
func transitionMachine(transitions ...domain.Transition) *domain.Machine {
	return &domain.Machine{
		ID:   "mch_approval_step",
		Name: "Approval Step",
		Fields: []domain.Field{
			{ID: "fld_decision", Name: "Decision", Type: domain.FieldTypeStatus, Options: []string{"pending", "approved", "rejected"}},
			{ID: "fld_note", Name: "Note", Type: domain.FieldTypeText},
		},
		Transitions: transitions,
	}
}

func TestValidate_transitions(t *testing.T) {
	valid := domain.Transition{ID: "trn_step_approve", Name: "Approve", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionDecide}

	tests := []struct {
		name    string
		machine *domain.Machine
		wantErr bool
	}{
		{name: "a well-formed edge", machine: transitionMachine(valid)},
		{
			// The declared "the runtime performs this itself" case -- mch_document's whole state
			// model. Rejecting it would force a derived field to name an Action it has none of.
			name:    "an edge naming no action",
			machine: transitionMachine(domain.Transition{ID: "trn_document_approved", Name: "Approved", Field: "fld_decision", From: "pending", To: "approved"}),
		},
		{
			name:    "an id that is not a trn_ id",
			machine: transitionMachine(domain.Transition{ID: "approve", Name: "Approve", Field: "fld_decision", From: "pending", To: "approved"}),
			wantErr: true,
		},
		{
			name:    "no name -- no screen can derive one from from/to",
			machine: transitionMachine(domain.Transition{ID: "trn_step_approve", Field: "fld_decision", From: "pending", To: "approved"}),
			wantErr: true,
		},
		{
			name:    "a field this machine does not declare",
			machine: transitionMachine(domain.Transition{ID: "trn_x", Name: "X", Field: "fld_missing", From: "pending", To: "approved"}),
			wantErr: true,
		},
		{
			// Silent failure if allowed: CheckTransitions only ever inspects status Fields, so
			// this edge would never match and the move it describes would be refused forever.
			name:    "a field that is not a status field",
			machine: transitionMachine(domain.Transition{ID: "trn_x", Name: "X", Field: "fld_note", From: "a", To: "b"}),
			wantErr: true,
		},
		{
			name:    "a value the field does not declare",
			machine: transitionMachine(domain.Transition{ID: "trn_x", Name: "X", Field: "fld_decision", From: "pending", To: "cancelled"}),
			wantErr: true,
		},
		{
			// CheckTransitions skips a value that does not change, so this edge could never fire.
			name:    "from and to are the same value",
			machine: transitionMachine(domain.Transition{ID: "trn_x", Name: "X", Field: "fld_decision", From: "pending", To: "pending"}),
			wantErr: true,
		},
		{
			name:    "an action this runtime does not realize",
			machine: transitionMachine(domain.Transition{ID: "trn_x", Name: "X", Field: "fld_decision", From: "pending", To: "approved", Action: "publish"}),
			wantErr: true,
		},
		{
			name:    "the same id twice",
			machine: transitionMachine(valid, valid),
			wantErr: true,
		},
		{
			// TransitionFor returns the first match, so two declarations of one edge would make
			// which Action governs it depend on declaration order.
			name: "the same edge under two ids",
			machine: transitionMachine(valid,
				domain.Transition{ID: "trn_step_approve_again", Name: "Approve", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionEdit}),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.machine); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// A Permission may now gate on roles alone, with no actor_field at all -- but it must still gate
// on something. A Permission naming no arm whatsoever reads as a guard and checks nothing.
func TestValidate_permissionMustGateOnSomething(t *testing.T) {
	m := transitionMachine()
	m.Permissions = []domain.Permission{{ID: "prm_role_only", Action: domain.ActionEdit, Roles: []string{"approver"}}}
	if err := Validate(m); err != nil {
		t.Errorf("Validate() = %v, want nil -- a role-only permission is a valid shape as of CAP-P01", err)
	}

	m.Permissions = []domain.Permission{{ID: "prm_empty", Action: domain.ActionEdit}}
	if err := Validate(m); err == nil {
		t.Error("Validate() = nil, want an error -- a permission that gates on nothing protects nothing")
	}
}

func TestValidate_completion(t *testing.T) {
	ok := validMachine()
	ok.Completion = &domain.Completion{Field: "fld_status", Done: "done"}
	if err := Validate(ok); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	for _, tc := range []struct {
		name string
		c    domain.Completion
		want string
	}{
		{"unknown field", domain.Completion{Field: "fld_nope", Done: "done"}, "is not a field of this machine"},
		{"not a status field", domain.Completion{Field: "fld_title", Done: "done"}, "must be a status field"},
		{"value not an option", domain.Completion{Field: "fld_status", Done: "shipped"}, "is not one of"},
	} {
		m := validMachine()
		c := tc.c
		m.Completion = &c
		t.Run(tc.name, func(t *testing.T) { assertIssue(t, m, tc.want) })
	}
}

func cardTagsMachines() (card, join, tag *domain.Machine) {
	card = &domain.Machine{ID: "mch_task", Name: "Task",
		Fields:   []domain.Field{{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText}},
		CardTags: &domain.CardTags{Machine: "mch_task_label", Via: "fld_task", Tag: "fld_label"}}
	join = &domain.Machine{ID: "mch_task_label", Name: "Task label", Fields: []domain.Field{
		{ID: "fld_task", Name: "Task", Type: domain.FieldTypeRelation, RelatedMachine: "mch_task"},
		{ID: "fld_label", Name: "Label", Type: domain.FieldTypeRelation, RelatedMachine: "mch_label"},
	}}
	tag = &domain.Machine{ID: "mch_label", Name: "Label",
		Fields: []domain.Field{
			{ID: "fld_name", Name: "Name", Type: domain.FieldTypeText},
			{ID: "fld_color", Name: "Colour", Type: domain.FieldTypeStatus, Options: []string{"blue", "rose"}},
		},
		CardFields: []domain.CardField{
			{Field: "fld_name", Role: domain.CardFieldRoleTitle},
			{Field: "fld_color", Role: domain.CardFieldRoleColor},
		}}
	return
}

func TestValidateCardTags(t *testing.T) {
	card, join, tag := cardTagsMachines()
	if err := validateCardTags([]*domain.Machine{card, join, tag}); err != nil {
		t.Fatalf("valid card_tags rejected: %v", err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(card, join, tag *domain.Machine)
		want   string
	}{
		{"join not installed", func(c, j, tg *domain.Machine) { c.CardTags.Machine = "mch_nowhere" }, "is not installed"},
		{"via is not a field", func(c, j, tg *domain.Machine) { c.CardTags.Via = "fld_nope" }, "has no field"},
		{"via points elsewhere", func(c, j, tg *domain.Machine) { j.Fields[0].RelatedMachine = "mch_label" }, "pointing back"},
		{"tag is not a reference", func(c, j, tg *domain.Machine) { j.Fields[1].RelatedMachine = "" }, "reference field naming the tag"},
		{"tag machine absent", func(c, j, tg *domain.Machine) { j.Fields[1].RelatedMachine = "mch_gone" }, "tag machine"},
		{"no title role", func(c, j, tg *domain.Machine) { tg.CardFields = tg.CardFields[1:] }, "title role"},
		{"colour not status", func(c, j, tg *domain.Machine) { tg.Fields[1].Type = domain.FieldTypeText }, "must be a status field"},
		{"colour outside palette", func(c, j, tg *domain.Machine) { tg.Fields[1].Options = []string{"blue", "chartreuse"} }, "not in the palette"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, j, tg := cardTagsMachines()
			tc.mutate(c, j, tg)
			err := validateCardTags([]*domain.Machine{c, j, tg})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// A `file` card role names the Field an upload lives in, so a Field of any other type is refused at load.
func TestValidate_FileCardRoleNeedsAFileField(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Name: "X",
		Fields:     []domain.Field{{ID: "fld_t", Name: "T", Type: domain.FieldTypeText}, {ID: "fld_f", Name: "F", Type: domain.FieldTypeFile}},
		CardFields: []domain.CardField{{Field: "fld_t", Role: domain.CardFieldRoleFile}},
	}
	if err := Validate(m); err == nil || !strings.Contains(err.Error(), "must be a file field") {
		t.Fatalf("a text field in the file role validated: %v", err)
	}
	m.CardFields[0].Field = "fld_f"
	if err := Validate(m); err != nil {
		t.Fatalf("a file field in the file role was refused: %v", err)
	}
}
