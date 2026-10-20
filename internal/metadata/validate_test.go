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
	assertIssue(t, m, "schedule.guard_field and schedule.guard_equals (or guard_not_equals) must be set together")
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

// The view switch is a SegmentedControl whose options are one per line, so a name with a line break would draw a
// second, empty choice (independent review of SegmentedControl, 2026-10-14).
func TestValidate_viewNameIsOneLine(t *testing.T) {
	m := validMachine()
	m.Views = []domain.View{{ID: "vw_task_table", Name: "Line1\nLine2", Type: domain.ViewTable}}
	assertIssue(t, m, "line break")
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

// A Machine's plural is a word or two on one line: it is said in a board's summary, and a paragraph there is a Machine written wrong.
func TestValidate_plural(t *testing.T) {
	ok := validMachine()
	ok.Plural = "tasks"
	if err := Validate(ok); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	for name, plural := range map[string]string{"two lines": "tasks\nand more", "too long": strings.Repeat("x", maxPluralRunes+1)} {
		m := validMachine()
		m.Plural = plural
		t.Run(name, func(t *testing.T) { assertIssue(t, m, "plural") })
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

func TestValidate_transitionActor(t *testing.T) {
	machine := func(tr domain.Transition) *domain.Machine {
		m := transitionMachine(tr)
		m.Fields = append(m.Fields, domain.Field{ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID})
		return m
	}
	edge := domain.Transition{ID: "trn_x", Name: "X", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionEdit}
	withRoles := edge
	withRoles.Roles = []string{"manager"}
	withActor := edge
	withActor.ActorField = "fld_owner"
	decide := withRoles
	decide.Action = domain.ActionDecide
	blank := edge
	blank.Roles = []string{" "}
	missing := edge
	missing.ActorField = "fld_nope"
	notPerson := edge
	notPerson.ActorField = "fld_note"
	for _, tc := range []struct {
		name string
		tr   domain.Transition
		want string
	}{
		{"roles on an edit edge", withRoles, ""},
		{"a person field on an edit edge", withActor, ""},
		{"roles on a decide edge", decide, "only an edge with action"},
		{"a blank role", blank, "roles entry 0 is empty"},
		{"an actor_field the machine lacks", missing, "not a field of this machine"},
		{"an actor_field that is not a reference", notPerson, "fld_note"},
	} {
		err := Validate(machine(tc.tr))
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: Validate = %v, want nil", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: Validate = %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
}

func TestValidatePermissionRoles_coversTransitions(t *testing.T) {
	apps := []domain.Application{{ID: "app_x", Roles: []string{"manager"}}}
	mk := func(appID string, roles ...string) []*domain.Machine {
		return []*domain.Machine{{ID: "mch_x", ApplicationID: appID, Transitions: []domain.Transition{{ID: "trn_x", Roles: roles}}}}
	}
	if err := validatePermissionRoles(apps, mk("app_x", "manager")); err != nil {
		t.Errorf("a declared role was refused: %v", err)
	}
	if err := validatePermissionRoles(apps, mk("app_x", "boss")); err == nil || !strings.Contains(err.Error(), "boss") {
		t.Errorf("an undeclared role = %v, want an error naming it", err)
	}
	if err := validatePermissionRoles(apps, mk("", "manager")); err == nil || !strings.Contains(err.Error(), "no application claims") {
		t.Errorf("roles on an unclaimed Machine = %v, want a refusal", err)
	}
}

func TestValidate_conditionalAndDerivedFields(t *testing.T) {
	base := func(extra ...domain.Field) *domain.Machine {
		m := validMachine()
		m.Fields = append(m.Fields,
			domain.Field{ID: "fld_kind", Name: "Kind", Type: domain.FieldTypeStatus, Options: []string{"a", "b"}},
			domain.Field{ID: "fld_note2", Name: "Note", Type: domain.FieldTypeText})
		m.Fields = append(m.Fields, extra...)
		return m
	}
	rw := func(f domain.Field, field, equals string) domain.Field {
		f.RequiredWhen = &domain.FieldCondition{Field: field, Equals: equals}
		return f
	}
	df := func(f domain.Field, field string, days map[string]int) domain.Field {
		f.DefaultFrom = &domain.FieldDefaultFrom{Field: field, Days: days}
		return f
	}
	txt := domain.Field{ID: "fld_x", Name: "X", Type: domain.FieldTypeText}
	date := domain.Field{ID: "fld_d", Name: "D", Type: domain.FieldTypeDate}
	for _, tc := range []struct {
		name string
		f    domain.Field
		want string
	}{
		{"a sound required_when", rw(txt, "fld_kind", "a"), ""},
		{"a sound default_from", df(date, "fld_kind", map[string]int{"a": 1, "b": 3}), ""},
		{"required beside required_when", rw(domain.Field{ID: "fld_x", Name: "X", Type: domain.FieldTypeText, Required: true}, "fld_kind", "a"), "required and required_when together"},
		{"required_when on itself", rw(txt, "fld_x", "a"), "names itself"},
		{"required_when on a missing field", rw(txt, "fld_nope", "a"), "not a field of this machine"},
		{"required_when on a text field", rw(txt, "fld_note2", "a"), "must be a status field"},
		{"required_when on a value that is not an option", rw(txt, "fld_kind", "zzz"), "never be required"},
		{"default_from on a text field", df(txt, "fld_kind", map[string]int{"a": 1}), "only valid on a date field"},
		{"default_from beside a default", df(domain.Field{ID: "fld_d", Name: "D", Type: domain.FieldTypeDate, Default: "2026-01-01"}, "fld_kind", map[string]int{"a": 1}), "one source"},
		{"default_from reading a missing field", df(date, "fld_nope", map[string]int{"a": 1}), "not a field of this machine"},
		{"default_from reading a text field", df(date, "fld_note2", map[string]int{"a": 1}), "must be a status field"},
		{"default_from naming a value that is not an option", df(date, "fld_kind", map[string]int{"zzz": 1}), "not one of"},
		{"default_from with a negative count", df(date, "fld_kind", map[string]int{"a": -1}), "not in the past"},
		{"default_from with no days", df(date, "fld_kind", nil), "derive nothing"},
	} {
		err := Validate(base(tc.f))
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: Validate = %v, want nil", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: Validate = %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
}

func TestValidate_scheduleGuardPolarities(t *testing.T) {
	mk := func(s domain.Schedule) *domain.Machine {
		m := validMachine()
		m.Fields = append(m.Fields, domain.Field{ID: "fld_due", Name: "Due", Type: domain.FieldTypeDate})
		s.DateField, s.When = "fld_due", domain.ScheduleWhenOverdue
		m.Events = []domain.Event{{ID: "evt_x", Schedule: &s, Then: domain.Service{Name: domain.ServiceLogActivity, Summary: "x"}}}
		return m
	}
	if err := Validate(mk(domain.Schedule{GuardField: "fld_status", GuardNotEquals: "done"})); err != nil {
		t.Errorf("guard_not_equals on an option was refused: %v", err)
	}
	for _, tc := range []struct {
		s    domain.Schedule
		want string
	}{
		{domain.Schedule{GuardField: "fld_status", GuardEquals: "todo", GuardNotEquals: "done"}, "two polarities"},
		{domain.Schedule{GuardField: "fld_status", GuardNotEquals: "zzz"}, "schedule.guard_not_equals"},
		{domain.Schedule{GuardNotEquals: "done"}, "must be set together"},
	} {
		if err := Validate(mk(tc.s)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: Validate = %v, want an error containing %q", tc.s, err, tc.want)
		}
	}
}

func TestValidatePermissionRoles_coversNotifiedRoles(t *testing.T) {
	apps := []domain.Application{{ID: "app_x", Roles: []string{"koordinator"}}}
	mk := func(appID, role string) []*domain.Machine {
		return []*domain.Machine{{ID: "mch_x", ApplicationID: appID, Events: []domain.Event{{ID: "evt_x", Then: domain.Service{Name: domain.ServiceSendNotification, Notify: &domain.Notify{RecipientRole: role}}}}}}
	}
	if err := validatePermissionRoles(apps, mk("app_x", "koordinator")); err != nil {
		t.Errorf("a declared role was refused: %v", err)
	}
	if err := validatePermissionRoles(apps, mk("app_x", "bos")); err == nil || !strings.Contains(err.Error(), "bos") {
		t.Errorf("an undeclared notified role = %v, want an error naming it", err)
	}
	if err := validatePermissionRoles(apps, mk("", "koordinator")); err == nil || !strings.Contains(err.Error(), "no application claims") {
		t.Errorf("a notified role on an unclaimed Machine = %v, want a refusal", err)
	}
	if err := validatePermissionRoles(apps, []*domain.Machine{{ID: "mch_y", ApplicationID: "app_x", Events: []domain.Event{{ID: "evt_y", Then: domain.Service{Name: domain.ServiceSendNotification, Notify: &domain.Notify{RecipientField: "fld_p"}}}}}}); err != nil {
		t.Errorf("a person recipient was judged by the role vocabulary: %v", err)
	}
}

func TestValidate_aggregateBounds(t *testing.T) {
	zero := 0.0
	hi, lo := 10.0, 20.0
	mk := func(b domain.AggregateBound) *domain.Machine {
		m := validMachine()
		m.Fields = append(m.Fields,
			domain.Field{ID: "fld_qty", Name: "Qty", Type: domain.FieldTypeNumber},
			domain.Field{ID: "fld_item", Name: "Item", Type: domain.FieldTypeText},
			domain.Field{ID: "fld_when", Name: "When", Type: domain.FieldTypeDate})
		m.Bounds = []domain.AggregateBound{b}
		return m
	}
	good := domain.AggregateBound{ID: "ens_x", SumOf: "fld_qty", Per: "fld_item", AtLeast: &zero}
	if err := Validate(mk(good)); err != nil {
		t.Fatalf("a sound bound was refused: %v", err)
	}
	with := func(f func(*domain.AggregateBound)) domain.AggregateBound { b := good; f(&b); return b }
	for _, tc := range []struct {
		name string
		b    domain.AggregateBound
		want string
	}{
		{"a bad id", with(func(b *domain.AggregateBound) { b.ID = "limit" }), "must match"},
		{"a missing sum field", with(func(b *domain.AggregateBound) { b.SumOf = "fld_nope" }), "sum_of"},
		{"a text sum field", with(func(b *domain.AggregateBound) { b.SumOf = "fld_item" }), "must be a number or money"},
		{"a missing group field", with(func(b *domain.AggregateBound) { b.Per = "fld_nope" }), "per \"fld_nope\" is not a field"},
		{"the same field twice", with(func(b *domain.AggregateBound) { b.Per = "fld_qty" }), "name the same field"},
		{"a date group", with(func(b *domain.AggregateBound) { b.Per = "fld_when" }), "must group records by"},
		{"no bound", with(func(b *domain.AggregateBound) { b.AtLeast = nil }), "bounds nothing"},
		{"an empty range", with(func(b *domain.AggregateBound) { b.AtLeast, b.AtMost = &lo, &hi }), "no total could satisfy"},
	} {
		if err := Validate(mk(tc.b)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate = %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
	dup := mk(good)
	dup.Bounds = append(dup.Bounds, good)
	if err := Validate(dup); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("a repeated id = %v", err)
	}
}

func TestValidateBoundsAreGenericallyWritten(t *testing.T) {
	zero := 0.0
	b := domain.AggregateBound{ID: "ens_x", SumOf: "fld_n", Per: "fld_p", AtLeast: &zero}
	if err := validateBoundsAreGenericallyWritten([]*domain.Machine{{ID: "mch_stock", Bounds: []domain.AggregateBound{b}}}); err != nil {
		t.Errorf("an ordinary Machine was refused: %v", err)
	}
	for name, m := range map[string]*domain.Machine{
		"the user Machine":    {ID: domain.UserMachineID, Bounds: []domain.AggregateBound{b}},
		"an engine's Machine": {ID: "mch_doc", WorkflowRole: "document", Bounds: []domain.AggregateBound{b}},
		"the activity log":    {ID: "mch_activity", Bounds: []domain.AggregateBound{b}},
	} {
		if err := validateBoundsAreGenericallyWritten([]*domain.Machine{m}); err == nil {
			t.Errorf("%s: ensure: was accepted on a Machine the generic routes do not write", name)
		}
	}
}

func TestValidateFrozenWhen(t *testing.T) {
	period := &domain.Machine{ID: "mch_period", Fields: []domain.Field{
		{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"open", "closed"}},
		{ID: "fld_name", Type: domain.FieldTypeText}}}
	mk := func(f domain.FrozenWhen) []*domain.Machine {
		return []*domain.Machine{period, {ID: "mch_voucher", Fields: []domain.Field{{ID: "fld_period", Type: domain.FieldTypeRelation, RelatedMachine: "mch_period"}}, Frozen: &f}}
	}
	if err := validateFrozenWhen(mk(domain.FrozenWhen{Via: "fld_period", Field: "fld_status", Equals: "closed"})); err != nil {
		t.Fatalf("a sound freeze was refused: %v", err)
	}
	for name, tc := range map[string]struct {
		f    domain.FrozenWhen
		want string
	}{
		"a missing field": {domain.FrozenWhen{Via: "fld_period", Field: "fld_nope", Equals: "closed"}, "is not a field of"},
		"a text field":    {domain.FrozenWhen{Via: "fld_period", Field: "fld_name", Equals: "closed"}, "must be a status field"},
		"a non-option":    {domain.FrozenWhen{Via: "fld_period", Field: "fld_status", Equals: "sealed"}, "freeze nothing"},
	} {
		if err := validateFrozenWhen(mk(tc.f)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	if err := validateFrozenWhen([]*domain.Machine{{ID: domain.UserMachineID, Frozen: &domain.FrozenWhen{Via: "x", Field: "y", Equals: "z"}}}); err == nil {
		t.Error("frozen_when was accepted on a Machine the runtime writes itself")
	}
	// The per-Machine half, through Validate.
	m := validMachine()
	m.Frozen = &domain.FrozenWhen{Via: "fld_nope", Field: "f", Equals: "e"}
	if err := Validate(m); err == nil || !strings.Contains(err.Error(), "frozen_when.via") {
		t.Errorf("a missing via = %v", err)
	}
	m = validMachine()
	m.Frozen = &domain.FrozenWhen{Via: "fld_title", Field: "f", Equals: "e"}
	if err := Validate(m); err == nil || !strings.Contains(err.Error(), "must be a relation field") {
		t.Errorf("a text via = %v", err)
	}
	m = validMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_rel", Name: "R", Type: domain.FieldTypeRelation, RelatedMachine: "mch_other"})
	m.Frozen = &domain.FrozenWhen{Via: "fld_rel"}
	if err := Validate(m); err == nil || !strings.Contains(err.Error(), "needs field and equals") {
		t.Errorf("a freeze naming nothing = %v", err)
	}
}

func TestValidate_computeAddDaysAndTransitionSets(t *testing.T) {
	mk := func(edit func(*domain.Machine)) *domain.Machine {
		m := transitionMachine(domain.Transition{ID: "trn_x", Name: "X", Field: "fld_decision", From: "pending", To: "approved", Action: domain.ActionEdit})
		m.Fields = append(m.Fields,
			domain.Field{ID: "fld_when", Name: "When", Type: domain.FieldTypeDate},
			domain.Field{ID: "fld_every", Name: "Every", Type: domain.FieldTypeNumber},
			domain.Field{ID: "fld_next", Name: "Next", Type: domain.FieldTypeDate, Compute: &domain.FieldCompute{Op: domain.ComputeAddDays, Fields: []string{"fld_when", "fld_every"}}})
		if edit != nil {
			edit(m)
		}
		return m
	}
	if err := Validate(mk(nil)); err != nil {
		t.Fatalf("a sound add_days was refused: %v", err)
	}
	next := func(m *domain.Machine) *domain.Field { return &m.Fields[len(m.Fields)-1] }
	for name, tc := range map[string]struct {
		edit func(*domain.Machine)
		want string
	}{
		"a number result":       {func(m *domain.Machine) { next(m).Type = domain.FieldTypeNumber }, "only valid on a date field"},
		"one operand":           {func(m *domain.Machine) { next(m).Compute.Fields = []string{"fld_when"} }, "takes exactly 2 operands"},
		"operands swapped":      {func(m *domain.Machine) { next(m).Compute.Fields = []string{"fld_every", "fld_when"} }, "operand 1 must be a date"},
		"a text second operand": {func(m *domain.Machine) { next(m).Compute.Fields = []string{"fld_when", "fld_note"} }, "must be a number field"},
		"sum still wants numbers": {func(m *domain.Machine) {
			next(m).Type, next(m).Compute = domain.FieldTypeNumber, &domain.FieldCompute{Op: domain.ComputeSum, Fields: []string{"fld_when"}}
		}, "operand 1 must be a figure, got a date"},
		"sets on a decide edge": {func(m *domain.Machine) {
			m.Transitions[0].Action, m.Transitions[0].Sets = domain.ActionDecide, map[string]string{"fld_when": "today"}
		}, "only an edge with action"},
		"sets a missing field":  {func(m *domain.Machine) { m.Transitions[0].Sets = map[string]string{"fld_nope": "today"} }, "not a field of this machine"},
		"sets from nowhere":     {func(m *domain.Machine) { m.Transitions[0].Sets = map[string]string{"fld_when": "tomorrow"} }, "not a source"},
		"sets a text field":     {func(m *domain.Machine) { m.Transitions[0].Sets = map[string]string{"fld_note": "today"} }, "not a date"},
		"sets a computed field": {func(m *domain.Machine) { m.Transitions[0].Sets = map[string]string{"fld_next": "today"} }, "already writes"},
	} {
		if err := Validate(mk(tc.edit)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate = %v, want an error containing %q", name, err, tc.want)
		}
	}
	if err := Validate(mk(func(m *domain.Machine) { m.Transitions[0].Sets = map[string]string{"fld_when": "today"} })); err != nil {
		t.Errorf("a sound sets was refused: %v", err)
	}
}

func TestValidateGenerates(t *testing.T) {
	mk := func(edit func(loan, inst *domain.Machine)) []*domain.Machine {
		loan := &domain.Machine{ID: "mch_loan", Fields: []domain.Field{
			{ID: "fld_months", Type: domain.FieldTypeNumber}, {ID: "fld_total", Type: domain.FieldTypeNumber},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"approved", "paid_out"}}},
			Transitions: []domain.Transition{{ID: "trn_pay", Name: "Pay out", Field: "fld_status", From: "approved", To: "paid_out", Action: domain.ActionEdit,
				Generates: &domain.GenerateSchedule{Machine: "mch_inst", CountField: "fld_months", LinkField: "fld_loan", DueField: "fld_due", DueEveryDays: 30, AmountField: "fld_amount", AmountFrom: "fld_total"}}}}
		inst := &domain.Machine{ID: "mch_inst", Fields: []domain.Field{
			{ID: "fld_loan", Type: domain.FieldTypeRelation, RelatedMachine: "mch_loan"},
			{ID: "fld_due", Type: domain.FieldTypeDate}, {ID: "fld_amount", Type: domain.FieldTypeNumber},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"open", "paid"}, Required: true, Default: "open"}}}
		if edit != nil {
			edit(loan, inst)
		}
		return []*domain.Machine{loan, inst}
	}
	if err := validateGenerates(mk(nil)); err != nil {
		t.Fatalf("a sound schedule was refused: %v", err)
	}
	for name, tc := range map[string]struct {
		edit func(loan, inst *domain.Machine)
		want string
	}{
		"a decide edge":         {func(l, i *domain.Machine) { l.Transitions[0].Action = domain.ActionDecide }, "only an edge with action"},
		"no spacing":            {func(l, i *domain.Machine) { l.Transitions[0].Generates.DueEveryDays = 0 }, "at least 1"},
		"a text count":          {func(l, i *domain.Machine) { l.Fields[0].Type = domain.FieldTypeText }, "count_field"},
		"a missing amount_from": {func(l, i *domain.Machine) { l.Transitions[0].Generates.AmountFrom = "fld_nope" }, "amount_from"},
		"an unknown target":     {func(l, i *domain.Machine) { l.Transitions[0].Generates.Machine = "mch_nope" }, "not a machine of this workspace"},
		"itself":                {func(l, i *domain.Machine) { l.Transitions[0].Generates.Machine = "mch_loan" }, "of itself"},
		"a runtime machine": {func(l, i *domain.Machine) {
			l.Transitions[0].Generates.Machine = domain.UserMachineID
			i.ID = domain.UserMachineID
		}, "written by the runtime"},
		"a link to elsewhere":  {func(l, i *domain.Machine) { i.Fields[0].RelatedMachine = "mch_other" }, "pointing back"},
		"a text due date":      {func(l, i *domain.Machine) { i.Fields[1].Type = domain.FieldTypeText }, "due_field"},
		"a text amount":        {func(l, i *domain.Machine) { i.Fields[2].Type = domain.FieldTypeText }, "amount_field"},
		"the same field twice": {func(l, i *domain.Machine) { l.Transitions[0].Generates.DueField = "fld_amount" }, "three different fields"},
		"an unfillable field": {func(l, i *domain.Machine) {
			i.Fields = append(i.Fields, domain.Field{ID: "fld_must", Type: domain.FieldTypeText, Required: true})
		}, "cannot fill"},
	} {
		if err := validateGenerates(mk(tc.edit)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

func TestValidateBalanced(t *testing.T) {
	mk := func(edit func(entry, line *domain.Machine)) []*domain.Machine {
		entry := &domain.Machine{ID: "mch_entry", Fields: []domain.Field{
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"draft", "posted"}}},
			Transitions: []domain.Transition{{ID: "trn_post", Name: "Post", Field: "fld_status", From: "draft", To: "posted", Action: domain.ActionEdit,
				Balanced: &domain.BalanceRule{Machine: "mch_line", Via: "fld_entry", Debit: "fld_debit", Credit: "fld_credit"}}}}
		line := &domain.Machine{ID: "mch_line", Fields: []domain.Field{
			{ID: "fld_entry", Type: domain.FieldTypeRelation, RelatedMachine: "mch_entry"},
			{ID: "fld_debit", Type: domain.FieldTypeNumber}, {ID: "fld_credit", Type: domain.FieldTypeNumber}}}
		if edit != nil {
			edit(entry, line)
		}
		return []*domain.Machine{entry, line}
	}
	if err := validateBalanced(mk(nil)); err != nil {
		t.Fatalf("a sound balance rule was refused: %v", err)
	}
	for name, tc := range map[string]struct {
		edit func(entry, line *domain.Machine)
		want string
	}{
		"a decide edge":       {func(e, l *domain.Machine) { e.Transitions[0].Action = domain.ActionDecide }, "only an edge with action"},
		"an unknown target":   {func(e, l *domain.Machine) { e.Transitions[0].Balanced.Machine = "mch_nope" }, "not a machine of this workspace"},
		"itself":              {func(e, l *domain.Machine) { e.Transitions[0].Balanced.Machine = "mch_entry" }, "against itself"},
		"a link to elsewhere": {func(e, l *domain.Machine) { l.Fields[0].RelatedMachine = "mch_other" }, "pointing back"},
		"a text debit":        {func(e, l *domain.Machine) { l.Fields[1].Type = domain.FieldTypeText }, "debit"},
		"a missing credit":    {func(e, l *domain.Machine) { e.Transitions[0].Balanced.Credit = "fld_nope" }, "credit"},
		"one field twice":     {func(e, l *domain.Machine) { e.Transitions[0].Balanced.Credit = "fld_debit" }, "two different fields"},
	} {
		if err := validateBalanced(mk(tc.edit)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// `sort: {field: fld_list, by: sort_order}` orders by the *related record's* column -- the lists of a board in the board's order -- so the Field is a
// reference and `by` one of the record columns a plain sort may name; anything else is a load error, never an ordering that quietly does nothing.
func TestValidate_sortByTheRelatedRecordsColumn(t *testing.T) {
	machine := func(by, field string) *domain.Machine {
		m := validMachine()
		m.Fields = append(m.Fields, domain.Field{ID: "fld_list", Name: "List", Type: domain.FieldTypeRelation, RelatedMachine: "mch_list"})
		m.Datasets = []domain.Dataset{{ID: "ds_all", Source: m.ID, Select: domain.SelectRecords, Limit: 10, Sort: []domain.SortKey{{Field: field, By: by}}}}
		return m
	}
	if err := Validate(machine("sort_order", "fld_list")); err != nil {
		t.Fatalf("a reference Field sorted by its record's sort_order: %v", err)
	}
	assertIssue(t, machine("sort_order", "fld_title"), "by:")
	assertIssue(t, machine("sort_order", "sort_order"), "by:")
	assertIssue(t, machine("nope", "fld_list"), "by:")
}
