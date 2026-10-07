package composition

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

// declaredPageLoader seeds `counts` records per status into a throwaway Machine, so a Binding has something
// to count. Statuses absent from `counts` get no record at all, which is the case row ordering has to handle.
func declaredPageLoader(t *testing.T, suffix string, counts map[string]int) (*Loader, context.Context) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run the declared-page integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	machineID := "mch_declared_page_" + suffix
	store := data.NewStore(pool)
	ctx := data.WithWorkspaceScope(context.Background(), "ws_declared_page_test")
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id = $1`, machineID); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	for status, n := range counts {
		for range n {
			if _, err := store.CreateRecord(ctx, machineID, map[string]any{"fld_status": status}); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
	}
	m := &domain.Machine{
		ID:     machineID,
		Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"draft", "in_review", "approved", "rejected"}}},
		Datasets: []domain.Dataset{{
			ID: "ds_by_status", Source: machineID, Dimension: "fld_status",
			Measures: []domain.Measure{{ID: "msr_total", Aggregate: domain.AggregateCount}},
		}},
	}
	return NewLoader(store, map[string]*domain.Machine{machineID: m}), ctx
}

func boundMetricGrid(n int) domain.PageNode {
	root := domain.PageNode{Kind: "layout", Type: "grid", Props: map[string]string{"gap": "default"}}
	for range n {
		root.Children = append(root.Children, domain.PageNode{
			Kind: "component", Type: "Metric",
			Binding: &domain.PageBinding{Dataset: "ds_by_status", Measure: "msr_total", Rows: domain.PageRowsDimension},
		})
	}
	return root
}

func declaredRowsOf(tree ir.UINode) (out [][2]string) {
	for _, c := range tree.Children {
		out = append(out, [2]string{c.Props["label"], c.Props["value"]})
	}
	return out
}

// Declared option order, a declared option with no record as an explicit 0, and a value no option names last.
func TestDeclaredPage_rowsFollowOptionOrderAndKeepEmptyOptions(t *testing.T) {
	l, ctx := declaredPageLoader(t, "order", map[string]int{"approved": 2, "draft": 1, "legacy": 3})
	tree, err := DeclaredPage(ctx, l, boundMetricGrid(1))
	if err != nil {
		t.Fatalf("DeclaredPage: %v", err)
	}
	want := [][2]string{{"draft", "1"}, {"in_review", "0"}, {"approved", "2"}, {"rejected", "0"}, {"legacy", "3"}}
	if got := declaredRowsOf(tree); !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

// A map ranges differently every run (007 §4.6), so one call passing proves little: 40 fresh loaders.
func TestDeclaredPage_rowOrderIsDeterministic(t *testing.T) {
	l, ctx := declaredPageLoader(t, "determinism", map[string]int{"zeta": 1, "alpha": 1, "mid": 1, "approved": 1})
	first, err := DeclaredPage(ctx, l, boundMetricGrid(1))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 40 {
		l2 := NewLoader(l.store, l.machines)
		again, err := DeclaredPage(ctx, l2, boundMetricGrid(1))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(declaredRowsOf(first), declaredRowsOf(again)) {
			t.Fatalf("render %d differs: %v vs %v", i, declaredRowsOf(first), declaredRowsOf(again))
		}
	}
}

// Cost is the number of distinct Datasets, never the number of nodes: ten bound nodes over one Dataset are one
// read, and an unbound tree is none.
func TestDeclaredPage_queryCostIsFlatInNodeCount(t *testing.T) {
	l1, ctx1 := declaredPageLoader(t, "cost1", map[string]int{"draft": 1})
	if _, err := DeclaredPage(ctx1, l1, boundMetricGrid(1)); err != nil {
		t.Fatal(err)
	}
	l10, ctx10 := declaredPageLoader(t, "cost10", map[string]int{"draft": 1})
	if _, err := DeclaredPage(ctx10, l10, boundMetricGrid(10)); err != nil {
		t.Fatal(err)
	}
	if l1.Reads() != 1 || l10.Reads() != 1 {
		t.Errorf("reads = %d for one bound node and %d for ten; want 1 and 1", l1.Reads(), l10.Reads())
	}
	lNone, ctxNone := declaredPageLoader(t, "cost0", nil)
	static := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}}
	if _, err := DeclaredPage(ctxNone, lNone, static); err != nil {
		t.Fatal(err)
	}
	if lNone.Reads() != 0 {
		t.Errorf("an unbound page read %d times; want 0", lNone.Reads())
	}
}
