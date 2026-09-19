package metadata

import (
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

func TestParse_view(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
view:
  layout: board
  group_by: fld_status
`)

	m, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if m.View.Layout != domain.LayoutBoard || m.View.GroupBy != "fld_status" {
		t.Errorf("View = %+v, want {board fld_status}", m.View)
	}
}

func TestParse_viewCardFields(t *testing.T) {
	yaml := []byte(`
id: mch_task
name: Task
view:
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
	want := []domain.CardField{
		{Field: "fld_title", Role: domain.CardFieldRoleTitle},
		{Field: "fld_status", Role: domain.CardFieldRoleStatus},
	}
	if len(m.View.CardFields) != len(want) {
		t.Fatalf("View.CardFields = %+v, want %+v", m.View.CardFields, want)
	}
	for i, cf := range m.View.CardFields {
		if cf != want[i] {
			t.Errorf("View.CardFields[%d] = %+v, want %+v", i, cf, want[i])
		}
	}
}

func TestParse_noView(t *testing.T) {
	m, err := Parse([]byte("id: mch_task\nname: Task\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if m.View.Layout != "" {
		t.Errorf("View.Layout = %q, want empty when no view: block is present", m.View.Layout)
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
