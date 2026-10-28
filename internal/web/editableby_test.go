package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// The generic update routes keep a Field its Machine says only some roles may change: a change by anyone else is refused (422) and the record is
// untouched, a Field left out keeps its stored value, a value sent back unchanged is not a change, and a role that may change it, or an admin, does.
func TestKeepsLockedFieldsHoldsAFieldToWhoMayChangeIt(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Editable By", "editable-by-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, "editable_by@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	m := &domain.Machine{ID: "mch_expense_editable_by", ApplicationID: "app_expense", Fields: []domain.Field{
		{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
		{ID: "fld_amount", Name: "Amount", Type: domain.FieldTypeNumber, EditableBy: []string{"reviewer"}},
	}}
	rec, err := store.CreateRecord(wsCtx, m.ID, map[string]any{"fld_title": "Taxi", "fld_amount": 100.0})
	if err != nil {
		t.Fatal(err)
	}
	try := func(actor domain.Actor, values map[string]any) (bool, int, map[string]any) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/", nil).WithContext(wsCtx)
		ok := keepsLockedFields(w, req, store, m, rec.ID, values, actor)
		return ok, w.Code, values
	}
	author := domain.Actor{WorkspaceRole: domain.WorkspaceRoleMember, Roles: map[string][]string{"app_expense": {"author"}}}
	reviewer := domain.Actor{WorkspaceRole: domain.WorkspaceRoleMember, Roles: map[string][]string{"app_expense": {"reviewer"}}}
	admin := domain.Actor{WorkspaceRole: domain.WorkspaceRoleAdmin}

	if ok, code, _ := try(author, map[string]any{"fld_title": "Taxi", "fld_amount": 999.0}); ok || code != http.StatusUnprocessableEntity {
		t.Errorf("an author changed the amount: ok=%v code=%d", ok, code)
	}
	if ok, _, v := try(author, map[string]any{"fld_title": "Cab"}); !ok || v["fld_amount"] != 100.0 {
		t.Errorf("a Field left out must keep its stored value: ok=%v values=%v", ok, v)
	}
	if ok, _, _ := try(author, map[string]any{"fld_title": "Cab", "fld_amount": 100.0}); !ok {
		t.Error("the amount sent back unchanged was refused")
	}
	for name, actor := range map[string]domain.Actor{"a reviewer": reviewer, "an admin": admin} {
		if ok, code, _ := try(actor, map[string]any{"fld_amount": 999.0}); !ok {
			t.Errorf("%s was refused: %d", name, code)
		}
	}
	if ok, _, _ := try(domain.Actor{}, map[string]any{"fld_amount": 1.0}); ok {
		t.Error("an unidentified caller changed the amount")
	}
}

// Equal is not identical: a locked text Field sent back as "7" for a stored "007" (or "1000" for "1e3") is no change, but what is written is the stored
// spelling, never the one the request sent.
func TestKeepsLockedFieldsWritesTheStoredSpellingOfAnUnchangedValue(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Locked Spelling", "locked-spelling-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, "locked_spelling@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	m := &domain.Machine{ID: "mch_spelling", ApplicationID: "app_x", Fields: []domain.Field{
		{ID: "fld_code", Name: "Code", Type: domain.FieldTypeText, EditableBy: []string{"reviewer"}},
	}}
	rec, err := store.CreateRecord(wsCtx, m.ID, map[string]any{"fld_code": "007"})
	if err != nil {
		t.Fatal(err)
	}
	author := domain.Actor{WorkspaceRole: domain.WorkspaceRoleMember, Roles: map[string][]string{"app_x": {"author"}}}
	for _, sent := range []string{"7", "+007", "7.0", "0007"} {
		values := map[string]any{"fld_code": sent}
		w := httptest.NewRecorder()
		if ok := keepsLockedFields(w, httptest.NewRequest(http.MethodPatch, "/", nil).WithContext(wsCtx), store, m, rec.ID, values, author); ok && values["fld_code"] != "007" {
			t.Errorf("sent %q: the value to write is %v, want the stored 007", sent, values["fld_code"])
		}
	}
}

// Every write of a person's submitted values to a record judges the Machine's locks: the generic routes do through commitRecordUpdate, and the document wizard's
// continue step, which updates a draft itself, must call keepsLockedFields too (the Fase 4 review found it did not).
func TestTheDocumentWizardContinueStepJudgesLockedFields(t *testing.T) {
	src, err := os.ReadFile("document.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func continueDocumentWizard(")
	if start < 0 {
		t.Fatal("continueDocumentWizard moved")
	}
	end := strings.Index(text[start+1:], "\nfunc ")
	if !strings.Contains(text[start:start+1+end], "keepsLockedFields(") {
		t.Error("continueDocumentWizard writes a person's values without keepsLockedFields")
	}
}
