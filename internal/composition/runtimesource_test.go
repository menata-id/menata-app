package composition

import (
	"context"
	"slices"
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/registry"
)

func TestRuntimeSourcesAndProvidersAgree(t *testing.T) {
	var declared []string
	for id := range registry.RuntimeSources {
		declared = append(declared, id)
	}
	slices.Sort(declared)
	if got := RuntimeProviderIDs(); !slices.Equal(got, declared) {
		t.Errorf("providers %v, registry sources %v: a source with no provider answers an error, a provider with no source can never be named", got, declared)
	}
}

func fakeRuntimeLoader(t *testing.T, rows []*data.Record, ds domain.Dataset, withRuntime bool) *Loader {
	t.Helper()
	runtimeProviders["fake"] = func(context.Context, *Loader, domain.Actor) ([]*data.Record, error) { return rows, nil }
	t.Cleanup(func() { delete(runtimeProviders, "fake") })
	m := &domain.Machine{ID: "mch_fake", Name: "Fake", Runtime: "fake", Datasets: []domain.Dataset{ds},
		Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}, {ID: "fld_n", Type: domain.FieldTypeNumber}}}
	l := NewLoader(nil, map[string]*domain.Machine{})
	if withRuntime {
		l = l.WithRuntime([]*domain.Machine{m}, domain.Actor{ID: "u1"})
	} else {
		l.machines[m.ID] = m
	}
	return l
}

func fakeRows(names ...string) []*data.Record {
	var out []*data.Record
	for i, n := range names {
		out = append(out, &data.Record{ID: n, MachineID: "mch_fake", Values: map[string]any{"fld_name": n, "fld_n": float64(10 - i*3)}, CreatedAt: time.Unix(int64(i), 0)})
	}
	return out
}

func TestSelectRuntime_filtersSortsAndCutsLikeTheDatabasePath(t *testing.T) {
	ds := domain.Dataset{ID: "ds_fake", Source: "mch_fake", Select: domain.SelectRecords, Limit: 2,
		Where: &expression.Predicate{All: []expression.Comparison{{Field: "fld_name", Op: expression.OpNotEquals, Value: "carol"}}},
		Sort:  []domain.SortKey{{Field: "fld_n", Direction: "asc"}}}
	l := fakeRuntimeLoader(t, fakeRows("alice", "bob", "carol", "dave"), ds, true)
	got, truncated, err := l.selectRecords(context.Background(), "ds_fake", expression.Context{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	// alice 10, bob 7, carol 4 (excluded), dave 1: ascending by number is dave, bob, alice; the limit keeps two.
	if !slices.Equal(ids, []string{"dave", "bob"}) || !truncated {
		t.Errorf("got %v truncated=%v, want [dave bob] truncated", ids, truncated)
	}
}

func TestSelectRuntime_isRefusedToALoaderNotGivenTheRuntime(t *testing.T) {
	ds := domain.Dataset{ID: "ds_fake", Source: "mch_fake", Select: domain.SelectRecords}
	l := fakeRuntimeLoader(t, fakeRows("alice"), ds, false)
	if _, _, err := l.selectRecords(context.Background(), "ds_fake", expression.Context{}); err == nil {
		t.Error("an ordinary loader read a runtime machine")
	}
}

func TestMatchesPredicates(t *testing.T) {
	v := map[string]any{"fld_a": "x", "fld_n": "5"}
	for name, tc := range map[string]struct {
		p    []data.FieldPredicate
		want bool
	}{
		"no predicate":                  {nil, true},
		"equal":                         {[]data.FieldPredicate{{Field: "fld_a", Value: "x"}}, true},
		"not equal fails":               {[]data.FieldPredicate{{Field: "fld_a", Value: "x", Negate: true}}, false},
		"empty on a present value":      {[]data.FieldPredicate{{Field: "fld_a", Empty: true}}, false},
		"empty on an absent value":      {[]data.FieldPredicate{{Field: "fld_z", Empty: true}}, true},
		"ordered":                       {[]data.FieldPredicate{{Field: "fld_n", Order: ">", Value: "3"}}, true},
		"ordered fails":                 {[]data.FieldPredicate{{Field: "fld_n", Order: "<", Value: "3"}}, false},
		"one alternative holds":         {[]data.FieldPredicate{{Field: "fld_a", Value: "q", Alternative: true}, {Field: "fld_a", Value: "x", Alternative: true}}, true},
		"no alternative holds":          {[]data.FieldPredicate{{Field: "fld_a", Value: "q", Alternative: true}, {Field: "fld_a", Value: "r", Alternative: true}}, false},
		"alternatives AND a plain fail": {[]data.FieldPredicate{{Field: "fld_a", Value: "x", Alternative: true}, {Field: "fld_n", Value: "9"}}, false},
	} {
		if got := matchesPredicates(v, tc.p); got != tc.want {
			t.Errorf("%s = %v, want %v", name, got, tc.want)
		}
	}
}

func TestSelectRuntime_sortsDescendingAndByText(t *testing.T) {
	ds := domain.Dataset{ID: "ds_fake", Source: "mch_fake", Select: domain.SelectRecords, Sort: []domain.SortKey{{Field: "fld_name", Direction: "desc"}}}
	l := fakeRuntimeLoader(t, fakeRows("bob", "Alice", "carol"), ds, true)
	got, _, err := l.selectRecords(context.Background(), "ds_fake", expression.Context{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if !slices.Equal(ids, []string{"carol", "bob", "Alice"}) {
		t.Errorf("descending by name, case-insensitively = %v, want [carol bob Alice]", ids)
	}
}
