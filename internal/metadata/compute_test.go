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
		"not a number":     {"    type: number\n    compute:", "    type: text\n    compute:", "only valid on a number or money field"},
		"required":         {"    type: number\n    compute:", "    type: number\n    required: true\n    compute:", "cannot be required"},
		"repeated operand": {"fields: [fld_pdam, fld_well]", "fields: [fld_pdam, fld_pdam]", "more than once"},
		"text operand":     {"  - id: fld_well\n    name: Well\n    type: number", "  - id: fld_well\n    name: Well\n    type: text", "must be a number, money or date field"},
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

// TestValidate_computedValuesChainButNeverInARing (plan A1, 2026-10-18): a computed value may read another (an outstanding
// subtracts the paid), computed after it; two that read each other have no order and are refused, naming the ring.
func TestValidate_computedValuesChainButNeverInARing(t *testing.T) {
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
	if err := Validate(m); err != nil {
		t.Fatalf("a chain was refused: %v", err)
	}
	values := map[string]any{"fld_pdam": 2.0, "fld_well": 3.0}
	data := m.ComputeOrder()
	if len(data) != 2 || data[0].ID != "fld_total" {
		t.Fatalf("compute order = %v, want fld_total before fld_double", data)
	}
	ring := strings.Replace(src, "fields: [fld_pdam, fld_well]", "fields: [fld_pdam, fld_double]", 1)
	m, err = Parse([]byte(ring))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(m); err == nil || !strings.Contains(err.Error(), "ring") {
		t.Errorf("Validate() = %v, want the ring refused", err)
	}
	_ = values
}

// TestComputeTreeParsesAndEvaluates: the tree shape -- an operation named as the key, operands that are Fields,
// literals or other operations -- parses into the expression its YAML says and evaluates as arithmetic does.
func TestComputeTreeParsesAndEvaluates(t *testing.T) {
	src := `id: mch_term
name: Term
fields:
  - { id: fld_gross, name: Gross, type: money }
  - { id: fld_vat, name: VAT, type: money }
  - { id: fld_wht, name: WHT, type: money, compute: { round: [{ multiply: [fld_gross, 0.02] }, 0] } }
  - { id: fld_net, name: Net, type: money, compute: { subtract: [fld_gross, fld_vat, fld_wht] } }
  - { id: fld_half, name: Half, type: number, compute: { divide: [fld_gross, 0] } }
`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	values := map[string]any{"fld_gross": 54000.0}
	for _, f := range m.ComputeOrder() {
		if v, ok := f.Compute.Evaluate(values); ok {
			values[f.ID] = v
		}
	}
	if values["fld_wht"] != 1080.0 || values["fld_net"] != 52920.0 {
		t.Errorf("wht %v net %v, want 1080 and 52920 (an empty VAT counts as nothing)", values["fld_wht"], values["fld_net"])
	}
	if _, divided := values["fld_half"]; divided {
		t.Error("a division by zero wrote a value")
	}
	for name, bad := range map[string]string{
		"an unknown operation": `{ average: [fld_gross, fld_vat] }`,
		"a divide of three":    `{ divide: [fld_gross, fld_vat, fld_wht] }`,
		"a date in a sum":      `{ sum: [fld_gross, fld_when] }`,
		"round by a field":     `{ round: [fld_gross, fld_vat] }`,
		"a parent off a money": `{ sum: [{ parent: fld_gross.fld_x }] }`,
	} {
		src := strings.Replace(`id: mch_term
name: Term
fields:
  - { id: fld_gross, name: Gross, type: money }
  - { id: fld_vat, name: VAT, type: money }
  - { id: fld_when, name: When, type: date }
  - { id: fld_x, name: X, type: money, compute: COMPUTE }
`, "COMPUTE", bad, 1)
		m, err := Parse([]byte(src))
		if err == nil {
			err = Validate(m)
		}
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
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

// TestValidate_moneyIsAFigureWhereverAFigureIsRead: money is stored as a number, so every place that reads a figure
// takes it -- a computed sum (and a money result), a Measure's sum and an ordered where -- rather than loading a
// money Field the runtime then sums as 0 (K-14). A count of days for add_days is still a plain number.
func TestValidate_moneyIsAFigureWhereverAFigureIsRead(t *testing.T) {
	m, err := Parse([]byte(`id: mch_term
name: Instalment
fields:
  - { id: fld_gross, name: Gross, type: money }
  - { id: fld_wht, name: WHT, type: money }
  - { id: fld_net, name: Net, type: money, compute: { op: sum, fields: [fld_gross, fld_wht] } }
  - { id: fld_due, name: Due, type: date }
  - { id: fld_days, name: Days, type: money }
datasets:
  - id: ds_big_terms
    select: records
    where: { field: fld_gross, op: gte, value: "1000000" }
    limit: 50
  - id: ds_term_totals
    measures:
      - { id: msr_gross, aggregate: sum, field: fld_gross }
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(m); err != nil {
		t.Fatalf("money was refused where a figure is read: %v", err)
	}
	m.Fields = append(m.Fields, domain.Field{ID: "fld_next", Name: "Next", Type: domain.FieldTypeDate,
		Compute: &domain.FieldCompute{Op: domain.ComputeAddDays, Fields: []string{"fld_due", "fld_days"}}})
	if err := Validate(m); err == nil || !strings.Contains(err.Error(), "must be a number field") {
		t.Errorf("add_days took money as its count of days: %v", err)
	}
}
