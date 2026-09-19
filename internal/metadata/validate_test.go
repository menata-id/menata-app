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
	assertIssue(t, m, "on_create and on must not both be set")
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
	assertIssue(t, m, "exactly one of on or on_create is required")
}

func TestValidate_viewDefaultsToTable(t *testing.T) {
	m := validMachine() // no View set at all
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_viewUnknownLayout(t *testing.T) {
	m := validMachine()
	m.View = domain.View{Layout: "carousel"}
	assertIssue(t, m, "is not a known layout")
}

func TestValidate_viewBoardValidGroupBy(t *testing.T) {
	m := validMachine()
	m.View = domain.View{Layout: domain.LayoutBoard, GroupBy: "fld_status"}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_viewBoardUnknownGroupBy(t *testing.T) {
	m := validMachine()
	m.View = domain.View{Layout: domain.LayoutBoard, GroupBy: "fld_ghost"}
	assertIssue(t, m, "is not a field of this machine")
}

func TestValidate_slaFieldValid(t *testing.T) {
	m := validMachine()
	m.Fields = append(m.Fields, domain.Field{ID: "fld_due_date", Name: "Due Date", Type: domain.FieldTypeDate})
	m.View = domain.View{SLAField: "fld_due_date"}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_slaFieldUnknown(t *testing.T) {
	m := validMachine()
	m.View = domain.View{SLAField: "fld_ghost"}
	assertIssue(t, m, "is not a field of this machine")
}

func TestValidate_slaFieldNotADate(t *testing.T) {
	m := validMachine()
	m.View = domain.View{SLAField: "fld_title"}
	assertIssue(t, m, "must be a date field")
}

func TestValidate_cardFieldsValid(t *testing.T) {
	m := validMachine()
	m.View = domain.View{CardFields: []domain.CardField{
		{Field: "fld_title", Role: domain.CardFieldRoleTitle},
		{Field: "fld_status", Role: domain.CardFieldRoleStatus},
	}}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_cardFieldsUnknownField(t *testing.T) {
	m := validMachine()
	m.View = domain.View{CardFields: []domain.CardField{{Field: "fld_ghost", Role: domain.CardFieldRoleTitle}}}
	assertIssue(t, m, "is not a field of this machine")
}

func TestValidate_cardFieldsUnknownRole(t *testing.T) {
	m := validMachine()
	m.View = domain.View{CardFields: []domain.CardField{{Field: "fld_title", Role: "carousel"}}}
	assertIssue(t, m, "has unknown role")
}

// permissionMachine is a Machine shaped like mch_approval_step: a person Field an Action can be
// scoped to (ROADMAP.md Phase 16).
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

func TestValidate_permissionActorFieldIsNotAReference(t *testing.T) {
	m := permissionMachine()
	m.Permissions[0].ActorField = "fld_title"
	assertIssue(t, m, "must reference an identity")
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
