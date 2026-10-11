package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// count_distinct names a Field of the Machine; a percentage names two Measures of its own Dataset, neither a
// percentage, and nothing of its own to count or filter; numerator/denominator on anything else is a fault.
func TestDataset_derivedMeasuresAreCheckedAtLoad(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{"fld_project": {ID: "fld_project", Type: domain.FieldTypeText}}
	all := domain.Measure{ID: "msr_all", Aggregate: domain.AggregateCount}
	run := func(ms ...domain.Measure) string {
		ds := domain.Dataset{ID: "ds_t", Source: "mch_x", Measures: append([]domain.Measure{all}, ms...)}
		return strings.Join(validateDataset(m, ds, fields, map[string]bool{}), "; ")
	}
	if got := run(
		domain.Measure{ID: "msr_p", Aggregate: domain.AggregateCountDistinct, Field: "fld_project"},
		domain.Measure{ID: "msr_s", Aggregate: domain.AggregatePercentage, Numerator: "msr_p", Denominator: "msr_all"},
	); got != "" {
		t.Fatalf("valid derived measures were refused: %s", got)
	}
	where := &expression.Predicate{All: []expression.Comparison{{Field: "fld_project", Op: expression.OpEquals, Value: "x"}}}
	for name, tc := range map[string]struct {
		ms   []domain.Measure
		want string
	}{
		"distinct with no field":     {[]domain.Measure{{ID: "msr_p", Aggregate: domain.AggregateCountDistinct}}, "distinct values of a field"},
		"percentage with no parts":   {[]domain.Measure{{ID: "msr_s", Aggregate: domain.AggregatePercentage}}, "names no denominator"},
		"percentage of an unknown":   {[]domain.Measure{{ID: "msr_s", Aggregate: domain.AggregatePercentage, Numerator: "msr_nope", Denominator: "msr_all"}}, `"msr_nope" is not a measure`},
		"percentage of a percentage": {[]domain.Measure{{ID: "msr_s", Aggregate: domain.AggregatePercentage, Numerator: "msr_all", Denominator: "msr_all"}, {ID: "msr_t", Aggregate: domain.AggregatePercentage, Numerator: "msr_s", Denominator: "msr_all"}}, "itself a percentage"},
		"percentage with a where":    {[]domain.Measure{{ID: "msr_s", Aggregate: domain.AggregatePercentage, Numerator: "msr_all", Denominator: "msr_all", Where: where}}, "no where: of its own"},
		"numerator on a count":       {[]domain.Measure{{ID: "msr_c", Aggregate: domain.AggregateCount, Numerator: "msr_all"}}, "belong to a percentage"},
	} {
		if got := run(tc.ms...); !strings.Contains(got, tc.want) {
			t.Errorf("%s: issues = %q, want one mentioning %q", name, got, tc.want)
		}
	}
}
