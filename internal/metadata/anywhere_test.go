package metadata

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// `any:` is an OR-group beside `all:`; a Dataset's `where:` may carry both, and a lone comparison, `all:` alone
// and `any:` alone all lower into the one Predicate.
func TestWhere_anyLowersBesideAll(t *testing.T) {
	var doc predicateDoc
	if err := yaml.Unmarshal([]byte(`
all:
  - { field: fld_status, op: not_equals, value: $done }
any:
  - { field: fld_due, op: is_empty }
  - { field: fld_due, op: gt, value: $today+7 }
`), &doc); err != nil {
		t.Fatal(err)
	}
	p := doc.predicate()
	if len(p.All) != 1 || len(p.Any) != 2 || p.Any[0].Op != expression.OpIsEmpty || p.Any[1].Value != "$today+7" {
		t.Fatalf("predicate = %+v", p)
	}

	var alone predicateDoc
	if err := yaml.Unmarshal([]byte("any:\n  - { field: fld_a, op: is_empty }\n"), &alone); err != nil {
		t.Fatal(err)
	}
	if q := alone.predicate(); q == nil || len(q.All) != 0 || len(q.Any) != 1 {
		t.Errorf("`any:` alone = %+v, want one alternative and no conjunction", q)
	}
}

// `$done` inside `any:` must be stamped exactly as inside `all:`; otherwise it is compared as the text "$done"
// and matches nothing, silently.
func TestDone_isStampedInsideAnyToo(t *testing.T) {
	status := domain.Field{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}}
	m := Normalize(&domain.Machine{ID: "mch_x", Fields: []domain.Field{status},
		Completion: &domain.Completion{Field: "fld_status", Done: "done"},
		Datasets: []domain.Dataset{{ID: "ds_rec", Source: "mch_x", Select: domain.SelectRecords, Limit: 5,
			Where: &expression.Predicate{Any: []expression.Comparison{
				{Field: "fld_status", Op: expression.OpEquals, Value: expression.SentinelDone}}}}}})
	if got := m.Datasets[0].Where.Any[0].Value; got != "done" {
		t.Errorf("$done inside any: = %q after Normalize, want the completion value", got)
	}
}

// `is_empty` asks whether a Field has a value, so a value beside it is refused, in `all:`, in `any:` and in a
// Measure's filter; and a comparison in `any:` is held to the same Field and operator checks as one in `all:`.
func TestWhere_isEmptyTakesNoValueAndAnyIsValidatedLikeAll(t *testing.T) {
	m := &domain.Machine{ID: "mch_x"}
	fields := map[string]domain.Field{"fld_due": {ID: "fld_due", Type: domain.FieldTypeDate}}
	run := func(p *expression.Predicate) string {
		ds := domain.Dataset{ID: "ds_t", Source: "mch_x", Select: domain.SelectRecords, Limit: 5, Where: p}
		return strings.Join(validateDataset(m, ds, fields, map[string]bool{}), "; ")
	}
	empty := expression.Comparison{Field: "fld_due", Op: expression.OpIsEmpty}
	if got := run(&expression.Predicate{Any: []expression.Comparison{empty}}); got != "" {
		t.Errorf("a valid is_empty was refused: %s", got)
	}
	withValue := empty
	withValue.Value = "x"
	for name, p := range map[string]*expression.Predicate{
		"all": {All: []expression.Comparison{withValue}},
		"any": {Any: []expression.Comparison{withValue}},
	} {
		if got := run(p); !strings.Contains(got, "takes none") {
			t.Errorf("%s: is_empty with a value = %q, want a refusal", name, got)
		}
	}
	for name, p := range map[string]*expression.Predicate{
		"unknown field in any": {Any: []expression.Comparison{{Field: "fld_gone", Op: expression.OpEquals, Value: "x"}}},
		"unknown op in any":    {Any: []expression.Comparison{{Field: "fld_due", Op: "sometimes", Value: "x"}}},
		"bad date in any":      {Any: []expression.Comparison{{Field: "fld_due", Op: expression.OpGreaterThan, Value: "soon"}}},
	} {
		if got := run(p); got == "" {
			t.Errorf("%s was accepted", name)
		}
	}
	ms := domain.Dataset{ID: "ds_agg", Source: "mch_x", Measures: []domain.Measure{{ID: "msr_n", Aggregate: domain.AggregateCount,
		Where: &expression.Predicate{All: []expression.Comparison{withValue}}}}}
	if got := strings.Join(validateDataset(m, ms, fields, map[string]bool{}), "; "); !strings.Contains(got, "takes none") {
		t.Errorf("a Measure's is_empty with a value = %q, want a refusal", got)
	}
}
