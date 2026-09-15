package metadata

import "testing"

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

func TestParse_invalidYAML(t *testing.T) {
	_, err := Parse([]byte("id: [this is not a machine"))
	if err == nil {
		t.Fatal("Parse() error = nil, want error for malformed YAML")
	}
}
