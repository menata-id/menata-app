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
		cmp("fld_due", expression.OpLessOrEqual, "$today+7"),
		cmp("fld_due", expression.OpGreaterThan, "$today-30"),
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
		"$today+7 on a number":       {cmp("fld_points", expression.OpLessThan, "$today+7"), "is a date"},
		"$today+7d, a unit":          {cmp("fld_due", expression.OpLessThan, "$today+7d"), "not a runtime context value"},
		"$today+, no number":         {cmp("fld_due", expression.OpLessThan, "$today+"), "not a runtime context value"},
	} {
		if got := run(c.cmp); !strings.Contains(got, c.want) {
			t.Errorf("%s: issues = %q, want one containing %q", name, got, c.want)
		}
	}
}

func measureIssues(m *domain.Machine, fields map[string]domain.Field, cs ...expression.Comparison) string {
	ds := domain.Dataset{ID: "ds_t", Source: m.ID, Dimension: "fld_due",
		Measures: []domain.Measure{{ID: "msr_n", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: cs}}}}
	return strings.Join(validateDataset(m, ds, fields, map[string]bool{}), "; ")
}

// A Measure resolves `$today` (the request's injected date) and nothing else: an aggregate has no viewer and no
// request parameters, so `$current_user` there would be compared as the literal text and count nothing.
func TestMeasureWhere_resolvesTodayAndRefusesTheRest(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{
		"fld_due":  {ID: "fld_due", Type: domain.FieldTypeDate},
		"fld_name": {ID: "fld_name", Type: domain.FieldTypeText},
	}
	if got := measureIssues(m, fields, expression.Comparison{Field: "fld_due", Op: expression.OpLessThan, Value: expression.SentinelToday}); got != "" {
		t.Errorf("a measure comparing a date against $today was refused: %q", got)
	}
	if got := measureIssues(m, fields, expression.Comparison{Field: "fld_due", Op: expression.OpLessOrEqual, Value: "$today+7"}); got != "" {
		t.Errorf("a measure comparing a date against $today+7 was refused: %q", got)
	}
	for _, v := range []string{expression.SentinelCurrentUser, "$parameters.x", "$today+7d"} {
		got := measureIssues(m, fields, expression.Comparison{Field: "fld_name", Op: expression.OpEquals, Value: v})
		if !strings.Contains(got, "cannot resolve") {
			t.Errorf("a measure comparing against %s was accepted: %q", v, got)
		}
	}
	if got := measureIssues(m, fields, expression.Comparison{Field: "fld_name", Op: expression.OpLessThan, Value: expression.SentinelToday}); got == "" {
		t.Error("$today against a text Field was accepted")
	}
}

// Every comparison of a conjunction is checked, not only the first.
func TestMeasureWhere_checksEveryComparison(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{"fld_due": {ID: "fld_due", Type: domain.FieldTypeDate}}
	got := measureIssues(m, fields,
		expression.Comparison{Field: "fld_due", Op: expression.OpLessThan, Value: expression.SentinelToday},
		expression.Comparison{Field: "fld_gone", Op: expression.OpEquals, Value: "x"})
	if !strings.Contains(got, `where.field "fld_gone" is not a field`) {
		t.Errorf("a second comparison naming no Field was accepted: %q", got)
	}
}

// `$done` is the value the Machine's own `completion:` names. Normalize stamps it where it can answer, and
// Validate refuses the ones it cannot -- otherwise "$done" would be compared as text and match nothing.
func TestDone_isStampedFromCompletionAndRefusedWhereItCannotBeAnswered(t *testing.T) {
	status := domain.Field{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}}
	build := func(completion *domain.Completion, field string, op expression.Op) *domain.Machine {
		cmp := expression.Comparison{Field: field, Op: op, Value: expression.SentinelDone}
		return &domain.Machine{ID: "mch_x", Fields: []domain.Field{status, {ID: "fld_other", Type: domain.FieldTypeText}}, Completion: completion,
			Datasets: []domain.Dataset{
				{ID: "ds_rec", Source: "mch_x", Select: domain.SelectRecords, Limit: 5, Where: &expression.Predicate{All: []expression.Comparison{cmp}}},
				{ID: "ds_agg", Source: "mch_x", Measures: []domain.Measure{{ID: "msr_n", Aggregate: domain.AggregateCount, Where: &expression.Predicate{All: []expression.Comparison{cmp}}}}},
			}}
	}
	done := &domain.Completion{Field: "fld_status", Done: "done"}

	m := Normalize(build(done, "fld_status", expression.OpNotEquals))
	if got := m.Datasets[0].Where.All[0].Value; got != "done" {
		t.Errorf("a records Dataset's $done = %q after Normalize, want the completion value", got)
	}
	if got := m.Datasets[1].Measures[0].Where.All[0].Value; got != "done" {
		t.Errorf("a Measure's $done = %q after Normalize, want the completion value", got)
	}
	if got := Normalize(Normalize(build(done, "fld_status", expression.OpNotEquals))).Datasets[0].Where.All[0].Value; got != "done" {
		t.Errorf("Normalize is not idempotent over $done: %q", got)
	}

	fields := map[string]domain.Field{"fld_status": status, "fld_other": {ID: "fld_other", Type: domain.FieldTypeText}}
	for name, bad := range map[string]*domain.Machine{
		"no completion": Normalize(build(nil, "fld_status", expression.OpNotEquals)),
		"another Field": Normalize(build(done, "fld_other", expression.OpEquals)),
		"ordered op":    Normalize(build(done, "fld_status", expression.OpLessThan)),
	} {
		for _, ds := range bad.Datasets {
			got := strings.Join(validateDataset(bad, ds, fields, map[string]bool{}), "; ")
			if !strings.Contains(got, "$done") {
				t.Errorf("%s, dataset %s: a $done nothing could answer was accepted: %q", name, ds.ID, got)
			}
		}
	}
}
