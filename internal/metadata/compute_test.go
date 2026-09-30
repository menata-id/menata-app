package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

const waterMachine = `id: mch_water
name: Water
fields:
  - id: fld_pdam
    name: PDAM
    type: number
  - id: fld_well
    name: Well
    type: number
  - id: fld_total
    name: Total
    type: number
    compute:
      op: sum
      fields: [fld_pdam, fld_well]
`

// TestParse_computedField: the declaration reaches domain.Field intact and passes validation.
func TestParse_computedField(t *testing.T) {
	m, err := Parse([]byte(waterMachine))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	c := m.Fields[2].Compute
	if c == nil || c.Op != domain.ComputeSum || len(c.Fields) != 2 {
		t.Fatalf("compute = %+v, want sum over two fields", c)
	}
}

// TestValidate_computedFieldRefusesWhatItCannotEvaluate: every shape the evaluation does not handle
// is a load error, not a blank cell at runtime.
func TestValidate_computedFieldRefusesWhatItCannotEvaluate(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"unknown op":       {"op: sum", "op: average", "is not one of"},
		"missing operand":  {"fields: [fld_pdam, fld_well]", "fields: [fld_pdam, fld_nope]", "does not declare"},
		"self reference":   {"fields: [fld_pdam, fld_well]", "fields: [fld_pdam, fld_total]", "names itself"},
		"no operands":      {"fields: [fld_pdam, fld_well]", "fields: []", "at least one field"},
		"not a number":     {"    type: number\n    compute:", "    type: text\n    compute:", "only valid on a number field"},
		"required":         {"    type: number\n    compute:", "    type: number\n    required: true\n    compute:", "cannot be required"},
		"repeated operand": {"fields: [fld_pdam, fld_well]", "fields: [fld_pdam, fld_pdam]", "more than once"},
		"text operand":     {"  - id: fld_well\n    name: Well\n    type: number", "  - id: fld_well\n    name: Well\n    type: text", "must be a number field"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := strings.Replace(waterMachine, tc.from, tc.to, 1)
			if src == waterMachine {
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

// TestValidate_computedOperandMayNotBeComputed: chains are refused, which is what makes evaluation
// order irrelevant and cycles impossible.
func TestValidate_computedOperandMayNotBeComputed(t *testing.T) {
	src := waterMachine + `  - id: fld_double
    name: Double
    type: number
    compute:
      op: sum
      fields: [fld_total, fld_pdam]
`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(m); err == nil || !strings.Contains(err.Error(), "is itself computed") {
		t.Errorf("Validate() = %v, want a refusal of a computed operand", err)
	}
}

// TestComputedFieldsAreGenericallyWritten: a computed Field is only allowed where the generic routes
// are the only writers.
func TestComputedFieldsAreGenericallyWritten(t *testing.T) {
	m, err := Parse([]byte(waterMachine))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateComputedFieldsAreGenericallyWritten([]*domain.Machine{m}); err != nil {
		t.Errorf("an unbound machine was refused: %v", err)
	}
	m.WorkflowRole = "document"
	if err := validateComputedFieldsAreGenericallyWritten([]*domain.Machine{m}); err == nil {
		t.Error("a machine cast in a workflow role was allowed a computed field")
	}
	m.WorkflowRole, m.ID = "", domain.UserMachineID
	if err := validateComputedFieldsAreGenericallyWritten([]*domain.Machine{m}); err == nil {
		t.Error("mch_user was allowed a computed field")
	}
}
