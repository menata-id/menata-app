package execution

import (
	"context"
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
