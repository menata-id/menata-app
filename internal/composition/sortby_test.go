package composition

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// sortByFixture seeds two Workspaces over the same two Machines (a List and a Task pointing at one) and hands back the Task Machine and a
// pool; the records it makes are removed when the test ends.
func sortByFixture(t *testing.T) (*domain.Machine, *data.Store) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run the sort-by integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id IN ('mch_sortby_task','mch_sortby_list')`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	m := &domain.Machine{ID: "mch_sortby_task", Fields: []domain.Field{
		{ID: "fld_title", Type: domain.FieldTypeText},
		{ID: "fld_list", Type: domain.FieldTypeRelation, RelatedMachine: "mch_sortby_list"},
	}}
	return m, data.NewStore(pool)
}

func titles(rs []*data.Record) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Values["fld_title"].(string))
	}
	return out
}

// A `by:` sort reads the related record's column for the SAME Workspace only: a task whose reference names a list that sits in another
// Workspace points at nothing here and sorts last, whatever the other Workspace's list says (review of slice 5: the isolation was only
// tested by the SQL text holding the word "workspace_id").
func TestSortByARelatedColumnNeverReadsAnotherWorkspace(t *testing.T) {
	m, store := sortByFixture(t)
	a := data.WithWorkspaceScope(context.Background(), "ws_sortby_a")
	b := data.WithWorkspaceScope(context.Background(), "ws_sortby_b")
	foreign, err := store.CreateRecord(a, "mch_sortby_list", map[string]any{"fld_name": "A's first list"}) // sort_order 1 there
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"b1", "b2"} {
		if _, err := store.CreateRecord(b, "mch_sortby_list", map[string]any{"fld_name": n}); err != nil {
			t.Fatal(err)
		}
	}
	own, err := store.CreateRecord(b, "mch_sortby_list", map[string]any{"fld_name": "b3"}) // sort_order 3 in B
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ title, list string }{{"points at A's list", foreign.ID}, {"points at B's own", own.ID}} {
		if _, err := store.CreateRecord(b, "mch_sortby_task", map[string]any{"fld_title": tc.title, "fld_list": tc.list}); err != nil {
			t.Fatal(err)
		}
	}
	for dir, desc := range map[string]string{"ascending": "", "descending": domain.SortDescending} {
		keys, err := sortKeysFor(m, []domain.SortKey{{Field: "fld_list", By: "sort_order", Direction: desc}})
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := store.ListRecordsSelect(b, "mch_sortby_task", "ds_sortby", nil, keys, 10)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"points at B's own", "points at A's list"}; len(got) != 2 || titles(got)[0] != want[0] || titles(got)[1] != want[1] {
			t.Errorf("%s: %v, want %v -- a reference to another Workspace's list is a dangling one and sorts last, in either direction", dir, titles(got), want)
		}
	}
}

// Equal keys have one order: the sort ends on the record's id, so the same rows come back in the same order and the limit cuts at the same
// place every time.
func TestAnExplicitSortEndsOnTheRecordsIDSoTiesAreStable(t *testing.T) {
	m, store := sortByFixture(t)
	b := data.WithWorkspaceScope(context.Background(), "ws_sortby_b")
	list, err := store.CreateRecord(b, "mch_sortby_list", map[string]any{"fld_name": "one list"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, n := range []string{"t1", "t2", "t3", "t4"} {
		r, err := store.CreateRecord(b, "mch_sortby_task", map[string]any{"fld_title": n, "fld_list": list.ID})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	keys, err := sortKeysFor(m, []domain.SortKey{{Field: "fld_list", By: "sort_order"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := store.ListRecordsSelect(b, "mch_sortby_task", "ds_sortby", nil, keys, 10)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].ID > got[i].ID {
			t.Errorf("tied rows are not in id order: %s then %s", got[i-1].ID, got[i].ID)
		}
	}
	_ = ids
}
