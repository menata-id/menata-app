package metadata

import (
	"reflect"
	"testing"

	"menata.app/internal/domain"
)

func TestParse(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
fields:
  - id: fld_title
    name: Title
    type: text
    required: true
  - id: fld_status
    name: Status
    type: status
    options: [todo, in_progress, done]
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if m.ID != "mch_task" {
		t.Errorf("ID = %q, want mch_task", m.ID)
	}
	if len(m.Fields) != 2 {
		t.Fatalf("len(Fields) = %d, want 2", len(m.Fields))
	}
	if m.Fields[0].ID != "fld_title" || !m.Fields[0].Required {
		t.Errorf("Fields[0] = %+v, want fld_title required=true", m.Fields[0])
	}
	if len(m.Fields[1].Options) != 3 {
		t.Errorf("Fields[1].Options = %v, want 3 options", m.Fields[1].Options)
	}
}

func TestParse_views(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
views:
  - id: vw_task_board
    name: Board
    type: board
    group_by: fld_status
  - id: vw_task_table
    name: Table
    type: table
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := []domain.View{
		{ID: "vw_task_board", Name: "Board", Type: domain.ViewBoard, GroupBy: "fld_status"},
		{ID: "vw_task_table", Name: "Table", Type: domain.ViewTable},
	}
	if !reflect.DeepEqual(m.Views, want) {
		t.Errorf("Views = %+v, want %+v", m.Views, want)
	}
}

func TestParse_cardFields(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
sla_field: fld_due_date
card_fields:
  - field: fld_title
    role: title
  - field: fld_status
    role: status
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if m.SLAField != "fld_due_date" {
		t.Errorf("SLAField = %q, want fld_due_date", m.SLAField)
	}
	want := []domain.CardField{
		{Field: "fld_title", Role: domain.CardFieldRoleTitle},
		{Field: "fld_status", Role: domain.CardFieldRoleStatus},
	}
	if !reflect.DeepEqual(m.CardFields, want) {
		t.Errorf("CardFields = %+v, want %+v", m.CardFields, want)
	}
}

// A Machine declaring no views: at all keeps rendering as one implicit table, which is how every
// Machine behaved before views: existed -- the compatibility claim DefaultView's zero value makes.
func TestParse_noViews(t *testing.T) {
	m, err := Parse([]byte("id: mch_task\nname: Task\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(m.Views) != 0 {
		t.Errorf("Views = %+v, want none when no views: block is present", m.Views)
	}
	if got := m.DefaultView().EffectiveType(); got != domain.ViewTable {
		t.Errorf("DefaultView() = %q, want an implicit table", got)
	}
}

func TestParse_personFieldAutoRelatesToUser(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
fields:
  - id: fld_assignee
    name: Assignee
    type: person
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	f := m.Fields[0]
	if f.RelatedMachine != domain.UserMachineID {
		t.Errorf("Fields[0].RelatedMachine = %q, want %q (implicit, no `machine:` key needed)", f.RelatedMachine, domain.UserMachineID)
	}
	if !f.IsReference() {
		t.Error("Fields[0].IsReference() = false, want true for a person field")
	}
}

func TestParse_permissions(t *testing.T) {
	yaml := []byte(`
id: mch_approval_step
name: Approval Step
fields:
  - id: fld_assignee
    name: Assignee
    type: person
permissions:
  - id: prm_decide_own_step
    action: decide
    actor_field: fld_assignee
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(m.Permissions) != 1 {
		t.Fatalf("len(Permissions) = %d, want 1", len(m.Permissions))
	}
	p := m.Permissions[0]
	if p.ID != "prm_decide_own_step" || p.Action != domain.ActionDecide || p.ActorField != "fld_assignee" {
		t.Errorf("Permissions[0] = %+v, want prm_decide_own_step/decide/fld_assignee", p)
	}
	if got := m.PermissionsFor(domain.ActionDecide); len(got) != 1 {
		t.Errorf("PermissionsFor(decide) returned %d permissions, want 1", len(got))
	}
	if got := m.PermissionsFor("submit"); len(got) != 0 {
		t.Errorf("PermissionsFor(submit) returned %d permissions, want 0", len(got))
	}
}

func TestParse_invalidYAML(t *testing.T) {
	_, err := Parse([]byte("id: [this is not a machine"))
	if err == nil {
		t.Fatal("Parse() error = nil, want error for malformed YAML")
	}
}

func TestParse_defaultCoercion(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
fields:
  - id: fld_status
    name: Status
    type: status
    options: [todo, in_progress, done]
    default: todo
  - id: fld_priority
    name: Priority
    type: number
    default: "3"
  - id: fld_urgent
    name: Urgent
    type: boolean
    default: "true"
  - id: fld_title
    name: Title
    type: text
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := m.Fields[0].Default; got != "todo" {
		t.Errorf("status Default = %#v, want \"todo\"", got)
	}
	if got, ok := m.Fields[1].Default.(float64); !ok || got != 3 {
		t.Errorf("number Default = %#v, want float64(3)", m.Fields[1].Default)
	}
	if got, ok := m.Fields[2].Default.(bool); !ok || got != true {
		t.Errorf("boolean Default = %#v, want true", m.Fields[2].Default)
	}
	if m.Fields[3].Default != nil {
		t.Errorf("fld_title Default = %#v, want nil (none declared)", m.Fields[3].Default)
	}
}

func TestParse_invalidNumberDefault(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
fields:
  - id: fld_priority
    name: Priority
    type: number
    default: "not a number"
`)
	if _, err := Parse(yaml); err == nil {
		t.Fatal("Parse() error = nil, want error for a non-numeric default on a number field")
	}
}

func TestParse_eventOnCreate(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
fields:
  - id: fld_title
    name: Title
    type: text
events:
  - id: evt_task_created
    on_create: true
    then:
      service: log_activity
      summary: "\"{fld_title}\" created"
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(m.Events) != 1 {
		t.Fatalf("len(Events) = %d, want 1", len(m.Events))
	}
	e := m.Events[0]
	if !e.OnCreate || e.On != "" || e.WhenEquals != "" {
		t.Errorf("Events[0] = %+v, want OnCreate=true, On=\"\", WhenEquals=\"\"", e)
	}
	if e.Then.Summary != `"{fld_title}" created` {
		t.Errorf("Events[0].Then.Summary = %q, want %q", e.Then.Summary, `"{fld_title}" created`)
	}
}
