package composition

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

// relationWhereFixture seeds one parent with three children (two todo, one done) and one parent with none, over
// a Dataset declaring an unfiltered Relation and an "open" one (`status not_equals $done`) on the same child.
func relationWhereFixture(t *testing.T, suffix string, openWhere *expression.Predicate, completion *domain.Completion) (*Loader, context.Context, string, string) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run the relation-where integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	store := data.NewStore(pool)
	ctx := data.WithWorkspaceScope(context.Background(), "ws_relwhere_test")

	parentID, childID := "mch_relwhere_parent_"+suffix, "mch_relwhere_child_"+suffix
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id = ANY($1)`, []string{parentID, childID}); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	full, err := store.CreateRecord(ctx, parentID, map[string]any{"fld_name": "Full"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.CreateRecord(ctx, parentID, map[string]any{"fld_name": "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"todo", "todo", "done"} {
		if _, err := store.CreateRecord(ctx, childID, map[string]any{"fld_parent": full.ID, "fld_status": status}); err != nil {
			t.Fatal(err)
		}
	}

	parent := &domain.Machine{
		ID:     parentID,
		Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}},
		Datasets: []domain.Dataset{{
			ID: "ds_parents", Source: parentID, Select: domain.SelectRecords, Limit: 10,
			Relations: []domain.Relation{
				{ID: "rel_all", Machine: childID, Via: "fld_parent"},
				{ID: "rel_open", Machine: childID, Via: "fld_parent", Where: openWhere},
			},
		}},
	}
	child := &domain.Machine{
		ID: childID,
		Fields: []domain.Field{
			{ID: "fld_parent", Type: domain.FieldTypeRelation, RelatedMachine: parentID},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"todo", "done"}},
		},
		Completion: completion,
	}
	l := NewLoader(store, map[string]*domain.Machine{parentID: parent, childID: child})
	return l, ctx, full.ID, empty.ID
}

func openWhere() *expression.Predicate {
	return &expression.Predicate{All: []expression.Comparison{{Field: "fld_status", Op: expression.OpNotEquals, Value: expression.SentinelDone}}}
}

// The filter is the whole feature: "2 open" beside "3 cards" for one parent, and 0 beside 0 for a parent with no
// children -- a parent with none must still appear, which is what a Dimension over the children cannot do.
func TestRelationWhereCountsOnlyTheChildrenItKeeps(t *testing.T) {
	l, ctx, fullID, emptyID := relationWhereFixture(t, "count", openWhere(), &domain.Completion{Field: "fld_status", Done: "done"})
	sel, err := l.SelectRelated(ctx, "ds_parents", expression.Context{})
	if err != nil {
		t.Fatalf("SelectRelated: %v", err)
	}
	for _, c := range []struct {
		rel, parent string
		want        int
	}{
		{"rel_all", fullID, 3}, {"rel_open", fullID, 2}, {"rel_all", emptyID, 0}, {"rel_open", emptyID, 0},
	} {
		if got := len(sel.Related(c.rel, c.parent)); got != c.want {
			t.Errorf("%s of %s = %d, want %d", c.rel, c.parent, got, c.want)
		}
	}
}

// Two Relations over one (child Machine, Field) are one query: "all cards" beside "open cards" must not be two
// reads of the same rows, or the GET sweep's repeated==0 invariant would fail for the Dashboard.
func TestRelationsOverOneChildShareOneRead(t *testing.T) {
	l, ctx, _, _ := relationWhereFixture(t, "share", openWhere(), &domain.Completion{Field: "fld_status", Done: "done"})
	if _, err := l.SelectRelated(ctx, "ds_parents", expression.Context{}); err != nil {
		t.Fatalf("SelectRelated: %v", err)
	}
	if got := l.Reads(); got != 2 { // the parents, then the children once
		t.Errorf("reads = %d, want 2 (parents + one shared child read)", got)
	}
}

// `$done` is the child's own completion value; a child declaring none cannot answer it, and a filter that
// quietly keeps nothing (or everything) is the wrong answer to that -- it is an error.
func TestDoneFilterOverAChildWithNoCompletionIsRefused(t *testing.T) {
	l, ctx, _, _ := relationWhereFixture(t, "nocompletion", openWhere(), nil)
	if _, err := l.SelectRelated(ctx, "ds_parents", expression.Context{}); err == nil {
		t.Error("SelectRelated resolved $done against a Machine with no completion: want an error")
	}
}

func TestRelationFilter_resolvesContextValuesAndNeverMutatesTheDeclaration(t *testing.T) {
	child := &domain.Machine{ID: "mch_c", Completion: &domain.Completion{Field: "fld_status", Done: "finished"}}
	rel := domain.Relation{ID: "rel_x", Machine: "mch_c", Via: "fld_p", Where: &expression.Predicate{All: []expression.Comparison{
		{Field: "fld_status", Op: expression.OpNotEquals, Value: expression.SentinelDone},
		{Field: "fld_due", Op: expression.OpEquals, Value: expression.SentinelToday},
	}}}
	got, err := relationFilter(rel, child, expression.Context{Today: "2026-10-11"})
	if err != nil {
		t.Fatalf("relationFilter: %v", err)
	}
	if got.All[0].Value != "finished" || got.All[1].Value != "2026-10-11" {
		t.Errorf("resolved = %+v, want $done -> finished and $today -> the request date", got.All)
	}
	if rel.Where.All[0].Value != expression.SentinelDone {
		t.Error("relationFilter rewrote the declaration it was given -- the next request would read a resolved value")
	}
	rel.Where.All = append(rel.Where.All, expression.Comparison{Field: "fld_a", Op: expression.OpEquals, Value: expression.SentinelCurrentUser})
	if _, err := relationFilter(rel, child, expression.Context{Today: "2026-10-11"}); err == nil {
		t.Error("an unresolvable context value must be an error, not a filter")
	}
	if f, err := relationFilter(domain.Relation{}, child, expression.Context{}); f != nil || err != nil {
		t.Errorf("no where: = (%v, %v), want (nil, nil)", f, err)
	}
}
