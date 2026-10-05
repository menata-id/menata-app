package metadata

import (
	"menata.app/internal/domain"
	"strings"
	"testing"
)

const commentMachineYAML = `id: mch_comment
name: Comments
fields:
  - id: fld_task
    name: Task
    type: relation
    machine: mch_task
  - id: fld_body
    name: Comment
    type: long_text
  - id: fld_author
    name: Author
    type: person
    stamp: current_user
card_fields:
  - { field: fld_body, role: comment }
`

func TestParse_stampedFieldAndCommentRole(t *testing.T) {
	m, err := Parse([]byte(commentMachineYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := m.StampedAuthorField(); got != "fld_author" {
		t.Errorf("StampedAuthorField = %q, want fld_author", got)
	}
}

func TestValidate_stampRefusesWhatItCannotDo(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"unknown source":      {"stamp: current_user", "stamp: clock", "is not one of"},
		"not a person":        {"    type: person\n    stamp:", "    type: text\n    stamp:", "person"},
		"with a default":      {"    stamp: current_user", "    stamp: current_user\n    default: usr_x", "default"},
		"comment not long":    {"    type: long_text", "    type: text", "long_text"},
		"comment with no who": {"    stamp: current_user", "", "stamp: current_user"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := strings.Replace(commentMachineYAML, tc.from, tc.to, 1)
			if src == commentMachineYAML {
				t.Fatalf("fixture edit %q did not apply", tc.from)
			}
			m, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			err = Validate(m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %v, want an issue containing %q", err, tc.want)
			}
		})
	}
}

func TestStampedFieldsAreGenericallyWritten(t *testing.T) {
	m, err := Parse([]byte(commentMachineYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStampedFieldsAreGenericallyWritten([]*domain.Machine{m}); err != nil {
		t.Errorf("an unbound machine was refused: %v", err)
	}
	m.WorkflowRole = "document"
	if err := validateStampedFieldsAreGenericallyWritten([]*domain.Machine{m}); err == nil {
		t.Error("a machine cast in a workflow role was allowed a stamped field")
	}
	m.WorkflowRole, m.ID = "", "mch_activity"
	if err := validateStampedFieldsAreGenericallyWritten([]*domain.Machine{m}); err == nil {
		t.Error("mch_activity was allowed a stamped field")
	}
}
