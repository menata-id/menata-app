package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// isBalanced is the guard every update path shares (the form route, the single-field route and the JSON route all end in
// passesWriteGuards), so it is held directly: the status move is refused until the lines add up, a write that takes no
// such edge reads nothing, and a refusal says how far apart the totals are.
func TestIsBalanced(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Balanced Test", "balanced-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, "balanced_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	mk := func(machine string, v map[string]any) string {
		r, err := store.CreateRecord(wsCtx, machine, v)
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	entry := &domain.Machine{ID: "mch_entry_fixture", Name: "Entry",
		Fields: []domain.Field{{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"draft", "posted"}}},
		Transitions: []domain.Transition{{ID: "trn_post", Name: "Post", Field: "fld_status", From: "draft", To: "posted", Action: domain.ActionEdit,
			Balanced: &domain.BalanceRule{Machine: "mch_line_fixture", Via: "fld_entry", Debit: "fld_debit", Credit: "fld_credit", Reason: "Not balanced."}}}}
	plain := &domain.Machine{ID: "mch_entry_fixture", Fields: entry.Fields}

	run := func(m *domain.Machine, id string, next map[string]any) (bool, int, string) {
		rec := httptest.NewRecorder()
		ok := isBalanced(rec, httptest.NewRequest(http.MethodPatch, "/x", nil).WithContext(wsCtx), store, m, id, next)
		return ok, rec.Code, rec.Body.String()
	}
	id := mk("mch_entry_fixture", map[string]any{"fld_status": "draft"})
	post := map[string]any{"fld_status": "posted"}

	if ok, code, body := run(entry, id, post); ok || code != http.StatusUnprocessableEntity || !strings.Contains(body, "nothing to post") {
		t.Errorf("an entry with no lines: ok=%v code=%d body=%q", ok, code, body)
	}
	mk("mch_line_fixture", map[string]any{"fld_entry": id, "fld_debit": 500.0, "fld_credit": 0.0})
	credit := mk("mch_line_fixture", map[string]any{"fld_entry": id, "fld_debit": 0.0, "fld_credit": 400.0})
	if ok, code, body := run(entry, id, post); ok || code != http.StatusUnprocessableEntity || !strings.Contains(body, "debit 500.00, credit 400.00") {
		t.Errorf("an unbalanced entry: ok=%v code=%d body=%q", ok, code, body)
	}
	// Lines of another entry do not count toward this one.
	other := mk("mch_entry_fixture", map[string]any{"fld_status": "draft"})
	mk("mch_line_fixture", map[string]any{"fld_entry": other, "fld_debit": 0.0, "fld_credit": 100.0})
	if ok, _, _ := run(entry, id, post); ok {
		t.Error("another entry's line balanced this one")
	}
	if _, err := store.UpdateRecord(wsCtx, "mch_line_fixture", credit, map[string]any{"fld_entry": id, "fld_debit": 0.0, "fld_credit": 500.0}); err != nil {
		t.Fatal(err)
	}
	if ok, code, body := run(entry, id, post); !ok {
		t.Errorf("a balanced entry was refused: code=%d body=%q", code, body)
	}
	// An edit that does not move the status, a create and a Machine without the rule are not judged.
	if ok, _, _ := run(entry, id, map[string]any{"fld_status": "draft"}); !ok {
		t.Error("an edit that leaves the status alone was judged")
	}
	if ok, _, _ := run(entry, "", post); !ok {
		t.Error("a create was judged")
	}
	if ok, _, _ := run(plain, other, post); !ok {
		t.Error("a Machine declaring no rule was judged")
	}
}
