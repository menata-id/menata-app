package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/execution"
)

// planGenerated refuses a move whose schedule cannot be written and plans the ones that can; writeGenerated then writes
// the rows. Each refusal is a status the caller returns before the loan's own write, so the status never moves without
// its schedule.
func TestPlanAndWriteGenerated(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Generated Test", "generated-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, "generated_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	loanM := &domain.Machine{ID: "mch_loan_fixture", Name: "Loan",
		Fields: []domain.Field{
			{ID: "fld_months", Type: domain.FieldTypeNumber}, {ID: "fld_total", Type: domain.FieldTypeNumber},
			{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"approved", "paid_out"}}},
		Transitions: []domain.Transition{{ID: "trn_pay", Name: "Pay", Field: "fld_status", From: "approved", To: "paid_out", Action: domain.ActionEdit,
			Generates: &domain.GenerateSchedule{Machine: "mch_inst_fixture", CountField: "fld_months", LinkField: "fld_loan", DueField: "fld_due", DueEveryDays: 30, AmountField: "fld_amount", AmountFrom: "fld_total"}}}}
	instM := &domain.Machine{ID: "mch_inst_fixture", Name: "Installment",
		Fields: []domain.Field{
			{ID: "fld_loan", Type: domain.FieldTypeRelation, RelatedMachine: loanM.ID, Required: true},
			{ID: "fld_due", Type: domain.FieldTypeDate}, {ID: "fld_amount", Type: domain.FieldTypeNumber},
			{ID: "fld_note", Type: domain.FieldTypeText, Required: true}}} // a required Field nothing fills: every row is refused
	machines := map[string]*domain.Machine{loanM.ID: loanM, instM.ID: instM}

	loan, err := store.CreateRecord(wsCtx, loanM.ID, map[string]any{"fld_status": "approved", "fld_months": 2.0, "fld_total": 100.0})
	if err != nil {
		t.Fatal(err)
	}
	take := map[string]any{"fld_status": "paid_out", "fld_months": 2.0, "fld_total": 100.0}
	plan := func(values map[string]any) (code int, batchRows int) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil).WithContext(wsCtx)
		b, ok := planGenerated(rec, req, store, machines, loanM, loan.ID, values)
		if ok && b != nil {
			batchRows = len(b.Rows)
		}
		if ok {
			return http.StatusOK, batchRows
		}
		return rec.Code, 0
	}

	if code, _ := plan(take); code != http.StatusUnprocessableEntity {
		t.Errorf("a row the target Machine would refuse = %d, want 422", code)
	}
	instM.Fields[3].Required = false
	if code, rows := plan(take); code != http.StatusOK || rows != 2 {
		t.Fatalf("a sound plan = %d with %d rows, want 200 with 2", code, rows)
	}
	if code, rows := plan(map[string]any{"fld_status": "approved"}); code != http.StatusOK || rows != 0 {
		t.Errorf("a write that takes no edge = %d with %d rows, want 200 with none", code, rows)
	}
	if code, _ := plan(map[string]any{"fld_status": "paid_out", "fld_total": 100.0}); code != http.StatusUnprocessableEntity {
		t.Errorf("a missing count = %d, want 422", code)
	}

	rec := httptest.NewRecorder()
	b, _ := planGenerated(rec, httptest.NewRequest(http.MethodPatch, "/", nil).WithContext(wsCtx), store, machines, loanM, loan.ID, take)
	writeGenerated(wsCtx, execution.Services{Store: store}, machines, b, "")
	rows, err := store.ListRecordsBy(wsCtx, instM.ID, "fld_loan", loan.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("writeGenerated wrote %d rows (%v), want 2", len(rows), err)
	}
	if code, _ := plan(take); code != http.StatusConflict {
		t.Errorf("a second generation for the same loan = %d, want 409", code)
	}
	writeGenerated(wsCtx, execution.Services{Store: store}, machines, nil, "") // a nil batch writes nothing and does not panic
}
