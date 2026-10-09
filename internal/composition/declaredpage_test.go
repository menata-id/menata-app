package composition

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
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
	tree, err := DeclaredPage(ctx, l, "", nil, boundMetricGrid(1))
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
	first, err := DeclaredPage(ctx, l, "", nil, boundMetricGrid(1))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 40 {
		l2 := NewLoader(l.store, l.machines)
		again, err := DeclaredPage(ctx, l2, "", nil, boundMetricGrid(1))
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
	if _, err := DeclaredPage(ctx1, l1, "", nil, boundMetricGrid(1)); err != nil {
		t.Fatal(err)
	}
	l10, ctx10 := declaredPageLoader(t, "cost10", map[string]int{"draft": 1})
	if _, err := DeclaredPage(ctx10, l10, "", nil, boundMetricGrid(10)); err != nil {
		t.Fatal(err)
	}
	if l1.Reads() != 1 || l10.Reads() != 1 {
		t.Errorf("reads = %d for one bound node and %d for ten; want 1 and 1", l1.Reads(), l10.Reads())
	}
	lNone, ctxNone := declaredPageLoader(t, "cost0", nil)
	static := domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}}
	if _, err := DeclaredPage(ctxNone, lNone, "", nil, static); err != nil {
		t.Fatal(err)
	}
	if lNone.Reads() != 0 {
		t.Errorf("an unbound page read %d times; want 0", lNone.Reads())
	}
}

// recordListLoader seeds one record per title into a throwaway Machine that projects `title` and `status`, and
// declares a `select: records` Dataset over it ordered by title with the given limit.
func recordListLoader(t *testing.T, suffix string, titles []string, limit int) (*Loader, context.Context) {
	t.Helper()
	l, ctx := declaredPageLoader(t, suffix, nil)
	machineID := "mch_declared_page_" + suffix
	for _, title := range titles {
		if _, err := l.store.CreateRecord(ctx, machineID, map[string]any{"fld_title": title, "fld_status": "draft"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &domain.Machine{
		ID: machineID,
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"draft", "approved"}},
		},
		CardFields: []domain.CardField{
			{Field: "fld_title", Role: domain.CardFieldRoleTitle},
			{Field: "fld_status", Role: domain.CardFieldRoleStatus},
		},
		Datasets: []domain.Dataset{{
			ID: "ds_recent", Source: machineID, Select: domain.SelectRecords, Limit: limit,
			Sort: []domain.SortKey{{Field: "fld_title"}},
		}},
	}
	return NewLoader(l.store, map[string]*domain.Machine{machineID: m}), ctx
}

func recordList() domain.PageNode {
	return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{
		Kind: "component", Type: "Collection", Props: map[string]string{"gap": "tight"},
		Binding: &domain.PageBinding{Dataset: "ds_recent", Rows: domain.PageRowsRecords},
		Children: []domain.PageNode{{Kind: "layout", Type: "row", Children: []domain.PageNode{
			{Kind: "static", Type: "paragraph", From: map[string]string{"text": "title"}},
			{Kind: "component", Type: "StatusBadge", Props: map[string]string{"tone": "info"}, From: map[string]string{"label": "status"}},
		}}},
	}}}
}

func itemTexts(tree ir.UINode) (out []string) {
	for _, item := range tree.Children[0].Children {
		out = append(out, item.Children[0].Props["text"]+"/"+item.Children[1].Props["label"])
	}
	return out
}

// Items follow the Dataset's own `sort:` (applied by the database), carry the Machine's Projection and not a
// Field id, and the limit bounds the list.
func TestDeclaredPage_recordsFollowTheDatasetSortAndLimit(t *testing.T) {
	l, ctx := recordListLoader(t, "reclist", []string{"charlie", "alpha", "bravo", "delta"}, 3)
	tree, err := DeclaredPage(ctx, l, "", nil, recordList())
	if err != nil {
		t.Fatalf("DeclaredPage: %v", err)
	}
	want := []string{"alpha/draft", "bravo/draft", "charlie/draft"}
	if got := itemTexts(tree); !reflect.DeepEqual(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

// 007 §20: the page reads the Dataset once however many records it holds, and once however many nodes name it.
func TestDeclaredPage_recordsQueryCostIsFlatInRecordCount(t *testing.T) {
	few, ctxFew := recordListLoader(t, "reccost1", []string{"a"}, 50)
	if _, err := DeclaredPage(ctxFew, few, "", nil, recordList()); err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 30)
	for i := range titles {
		titles[i] = string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	many, ctxMany := recordListLoader(t, "reccost30", titles, 50)
	if _, err := DeclaredPage(ctxMany, many, "", nil, recordList()); err != nil {
		t.Fatal(err)
	}
	if few.Reads() != many.Reads() || many.Reads() != 1 {
		t.Errorf("reads = %d for one record and %d for thirty; want 1 and 1", few.Reads(), many.Reads())
	}
}

// A role the Machine does not project is a fault of the declaration surfaced by Lower, not a blank cell.
func TestDeclaredPage_recordsRefuseARoleTheMachineDoesNotProject(t *testing.T) {
	l, ctx := recordListLoader(t, "recrole", []string{"a"}, 5)
	page := recordList()
	page.Children[0].Children[0].Children[0].From = map[string]string{"text": "money"}
	if _, err := DeclaredPage(ctx, l, "", nil, page); err == nil {
		t.Fatal("DeclaredPage accepted a from: role the Machine does not project")
	}
}

// A per-viewer Dataset fails closed for a page that cannot say who is viewing, rather than listing everyone's.
func TestDeclaredPage_recordsOnAPerViewerDatasetNeedTheViewer(t *testing.T) {
	l, ctx := recordListLoader(t, "recviewer", []string{"a", "b"}, 5)
	ds := &l.machines["mch_declared_page_recviewer"].Datasets[0]
	ds.Where = &expression.Predicate{All: []expression.Comparison{{Field: "fld_title", Op: expression.OpEquals, Value: expression.SentinelCurrentUser}}}
	if _, err := DeclaredPage(ctx, l, "", nil, recordList()); err == nil {
		t.Fatal("a $current_user Dataset answered for an unknown viewer")
	}
	tree, err := DeclaredPage(ctx, NewLoader(l.store, l.machines), "a", nil, recordList())
	if err != nil {
		t.Fatal(err)
	}
	if got := itemTexts(tree); !reflect.DeepEqual(got, []string{"a/draft"}) {
		t.Errorf("items = %v, want only the viewer's", got)
	}
}

// A page that claims its list is complete is told by the Data Plane whether the Dataset's own `limit:` bit,
// through the real loader and a real database: four records under a limit of three is cut, under five is not.
func TestDeclaredPage_completeNamesTheDatasetsBoundOnlyWhenItBit(t *testing.T) {
	for name, c := range map[string]struct {
		limit    int
		complete string
		want     string
	}{
		"cut and claimed":     {3, "true", "3"},
		"not cut and claimed": {5, "true", ""},
		"cut, no claim":       {3, "", ""},
	} {
		l, ctx := recordListLoader(t, "complete"+strings.ReplaceAll(name, " ", ""), []string{"charlie", "alpha", "bravo", "delta"}, c.limit)
		page := recordList()
		if c.complete != "" {
			page.Children[0].Props["complete"] = c.complete
		}
		tree, err := DeclaredPage(ctx, l, "", nil, page)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := tree.Children[0].Props["truncated"]; got != c.want {
			t.Errorf("%s: truncated = %q, want %q", name, got, c.want)
		}
	}
}
