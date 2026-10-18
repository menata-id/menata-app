package execution

import (
	"context"
	"maps"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestRecomputeAllBringsStoredRecordsToTheirFormulas (owner decision K3): a formula declared or changed after records
// were stored reaches them -- a child's own value and the parent's aggregate over its children -- and a record that
// already holds its values is not written again.
func TestRecomputeAllBringsStoredRecordsToTheirFormulas(t *testing.T) {
	pool := testPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Recompute Test", "recompute-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupTest(t, pool, ws.ID, "recompute_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	contract := &domain.Machine{ID: "mch_rc_contract", Fields: []domain.Field{
		{ID: "fld_value", Type: domain.FieldTypeMoney},
		{ID: "fld_paid", Type: domain.FieldTypeMoney, Compute: &domain.FieldCompute{Op: domain.ComputeSumChildren, Expr: &domain.ComputeExpr{Op: domain.ComputeSumChildren,
			Children: &domain.ChildAggregate{Machine: "mch_rc_term", Via: "fld_contract", Field: "fld_net", Where: &domain.FieldCondition{Field: "fld_status", Equals: "Paid"}}}}},
		{ID: "fld_outstanding", Type: domain.FieldTypeMoney, Compute: &domain.FieldCompute{Op: domain.ComputeSubtract, Fields: []string{"fld_value", "fld_paid"}}},
	}}
	term := &domain.Machine{ID: "mch_rc_term", Fields: []domain.Field{
		{ID: "fld_contract", Type: domain.FieldTypeRelation, RelatedMachine: "mch_rc_contract"},
		{ID: "fld_gross", Type: domain.FieldTypeMoney},
		{ID: "fld_wht", Type: domain.FieldTypeMoney},
		{ID: "fld_net", Type: domain.FieldTypeMoney, Compute: &domain.FieldCompute{Op: domain.ComputeSubtract, Fields: []string{"fld_gross", "fld_wht"}}},
		{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"Open", "Paid"}},
	}}
	c, err := store.CreateRecord(wsCtx, contract.ID, map[string]any{"fld_value": 100.0})
	if err != nil {
		t.Fatal(err)
	}
	// Stored before the formulas: no net, no paid.
	for _, v := range []map[string]any{
		{"fld_contract": c.ID, "fld_gross": 40.0, "fld_wht": 2.0, "fld_status": "Paid"},
		{"fld_contract": c.ID, "fld_gross": 60.0, "fld_wht": 3.0, "fld_status": "Open"},
	} {
		if _, err := store.CreateRecord(wsCtx, term.ID, v); err != nil {
			t.Fatal(err)
		}
	}
	n, err := RecomputeAll(wsCtx, store, []*domain.Machine{contract, term})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("wrote %d records, want 3 (two terms and their contract)", n)
	}
	got, err := store.GetRecord(wsCtx, contract.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["fld_paid"] != 38.0 || got.Values["fld_outstanding"] != 62.0 {
		t.Errorf("contract paid %v outstanding %v, want 38 (the paid net) and 62", got.Values["fld_paid"], got.Values["fld_outstanding"])
	}
	if again, err := RecomputeAll(wsCtx, store, []*domain.Machine{contract, term}); err != nil || again != 0 {
		t.Errorf("a second pass wrote %d records (%v), want none", again, err)
	}
}

// TestRecomputeChildrenFollowsTheirParent (the review of step 3): a value a child reads through `parent:` follows the
// parent when the parent is written, rather than waiting for the next load.
func TestRecomputeChildrenFollowsTheirParent(t *testing.T) {
	pool := testPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Recompute Children Test", "recompute-children-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupTest(t, pool, ws.ID, "recompute_children_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	rate := &domain.Machine{ID: "mch_rc_rate", Fields: []domain.Field{{ID: "fld_pct", Type: domain.FieldTypeNumber}}}
	line := &domain.Machine{ID: "mch_rc_line", Fields: []domain.Field{
		{ID: "fld_rate", Type: domain.FieldTypeRelation, RelatedMachine: "mch_rc_rate"},
		{ID: "fld_gross", Type: domain.FieldTypeMoney},
		{ID: "fld_tax", Type: domain.FieldTypeMoney, Compute: &domain.FieldCompute{Op: domain.ComputeMultiply, Expr: &domain.ComputeExpr{Op: domain.ComputeMultiply,
			Args: []domain.ComputeExpr{{Field: "fld_gross"}, {Parent: &domain.ParentRef{Via: "fld_rate", Field: "fld_pct"}}}}}},
	}}
	machines := map[string]*domain.Machine{rate.ID: rate, line.ID: line}
	r, err := store.CreateRecord(wsCtx, rate.ID, map[string]any{"fld_pct": 0.1})
	if err != nil {
		t.Fatal(err)
	}
	l, err := store.CreateRecord(wsCtx, line.ID, map[string]any{"fld_rate": r.ID, "fld_gross": 200.0, "fld_tax": 20.0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateRecord(wsCtx, rate.ID, r.ID, map[string]any{"fld_pct": 0.2}); err != nil {
		t.Fatal(err)
	}
	if err := RecomputeChildren(wsCtx, store, machines, rate, r.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetRecord(wsCtx, line.ID, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["fld_tax"] != 40.0 {
		t.Errorf("tax %v, want 40 after the rate became 20%%", got.Values["fld_tax"])
	}

	// The review of step 5: an edit of a Field the child does not read leaves the child alone -- nothing is listed or locked.
	if _, err := store.UpdateRecord(wsCtx, line.ID, l.ID, map[string]any{"fld_rate": r.ID, "fld_gross": 200.0, "fld_tax": 999.0}); err != nil {
		t.Fatal(err)
	}
	same := map[string]any{"fld_pct": 0.2}
	if err := RecomputeChildren(wsCtx, store, machines, rate, r.ID, same, maps.Clone(same)); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetRecord(wsCtx, line.ID, l.ID); got.Values["fld_tax"] != 999.0 {
		t.Errorf("a parent whose read Field did not change recomputed its child (tax %v, want it left at 999)", got.Values["fld_tax"])
	}
	changed := map[string]any{"fld_pct": 0.25}
	if err := RecomputeChildren(wsCtx, store, machines, rate, r.ID, same, changed); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetRecord(wsCtx, line.ID, l.ID); got.Values["fld_tax"] != 40.0 {
		t.Errorf("a parent whose read Field changed left its child at %v, want it recomputed to 40", got.Values["fld_tax"])
	}
}
