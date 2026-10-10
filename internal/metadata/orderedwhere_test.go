package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// An ordered operator orders a number or a date, and the value must be one the Field could hold. $today is a
// date: against anything else it is refused whatever the operator, and it is refused outside a records
// Dataset's `where:`, where nothing would resolve it and it would be compared as a literal.
func TestDatasetWhere_orderedOperatorsAreCheckedAgainstTheField(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{
		"fld_due":    {ID: "fld_due", Type: domain.FieldTypeDate},
		"fld_points": {ID: "fld_points", Type: domain.FieldTypeNumber},
		"fld_name":   {ID: "fld_name", Type: domain.FieldTypeText},
	}
	run := func(c expression.Comparison) string {
		ds := domain.Dataset{ID: "ds_t", Source: "mch_x", Select: domain.SelectRecords, Limit: 5,
			Where: &expression.Predicate{All: []expression.Comparison{c}}}
		return strings.Join(validateDataset(m, ds, fields, map[string]bool{}), "; ")
	}
	cmp := func(field string, op expression.Op, value string) expression.Comparison {
		return expression.Comparison{Field: field, Op: op, Value: value}
	}

	for _, ok := range []expression.Comparison{
		cmp("fld_due", expression.OpLessThan, expression.SentinelToday),
		cmp("fld_due", expression.OpGreaterOrEqual, "2026-10-10"),
		cmp("fld_due", expression.OpLessOrEqual, "$parameters.until"),
		cmp("fld_points", expression.OpGreaterThan, "9"),
		cmp("fld_due", expression.OpEquals, expression.SentinelToday),
	} {
		if got := run(ok); got != "" {
			t.Errorf("%+v was refused: %s", ok, got)
		}
	}
	for name, c := range map[string]struct {
		cmp  expression.Comparison
		want string
	}{
		"ordering a text":            {cmp("fld_name", expression.OpLessThan, "m"), "orders a number or a date"},
		"a date that is no date":     {cmp("fld_due", expression.OpLessThan, "tomorrow"), "not a YYYY-MM-DD date"},
		"a number that is no number": {cmp("fld_points", expression.OpLessThan, "many"), "is not a number"},
		"$today on a number":         {cmp("fld_points", expression.OpLessThan, expression.SentinelToday), "is a date"},
		"$today on a text, equals":   {cmp("fld_name", expression.OpEquals, expression.SentinelToday), "is a date"},
	} {
		if got := run(c.cmp); !strings.Contains(got, c.want) {
			t.Errorf("%s: issues = %q, want one containing %q", name, got, c.want)
		}
	}
}

func TestMeasureWhere_refusesAContextValueNothingResolves(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{"fld_due": {ID: "fld_due", Type: domain.FieldTypeDate}}
	ds := domain.Dataset{ID: "ds_t", Source: "mch_x", Dimension: "fld_due",
		Measures: []domain.Measure{{ID: "msr_n", Aggregate: domain.AggregateCount,
			Where: &expression.Comparison{Field: "fld_due", Op: expression.OpLessThan, Value: expression.SentinelToday}}}}
	got := strings.Join(validateDataset(m, ds, fields, map[string]bool{}), "; ")
	if !strings.Contains(got, "only a `select: records` Dataset's `where:` resolves") {
		t.Errorf("a measure comparing against $today was accepted: %q", got)
	}
}
