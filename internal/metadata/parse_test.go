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

func TestParse_invalidYAML(t *testing.T) {
	_, err := Parse([]byte("id: [this is not a machine"))
	if err == nil {
		t.Fatal("Parse() error = nil, want error for malformed YAML")
	}
}
