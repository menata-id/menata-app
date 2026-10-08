package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// TestSubmitDeleteAccount_wrongPasswordChangesNothing_rightPasswordEndsTheLogin asserts what a person
// asking for deletion is owed: a wrong password deletes nothing, and a right one leaves the old
// address unable to sign in and the session cookie cleared.
func TestSubmitDeleteAccount_wrongPasswordChangesNothing_rightPasswordEndsTheLogin(t *testing.T) {
	const (
		email    = "delete_account_test@example.com"
		password = "the-account-password"
	)
	_, _, ctx, store, _, installed, _ := routerSetupParts(t, "deleteaccount")
	workspaceID, _ := data.WorkspaceScope(ctx)
	row, err := store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}

	record, err := store.CreateRecord(ctx, domain.UserMachineID, map[string]any{})
	if err != nil {
		t.Fatalf("CreateRecord(member): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, domain.UserMachineID, record.ID) })
	if err := store.AddMember(ctx, workspaceID, record.ID, email, "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	hash, err := authorization.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	deleteCredentialForTest(t, ctx, email)
	if err := store.CreateCredential(ctx, email, "Test Person", hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	t.Cleanup(func() { deleteCredentialForTest(t, ctx, email) })

	cfg := config.Config{SessionSecret: "deleteaccount-secret"}
	workspaces := map[string]domain.Workspace{row.Slug: installed}
	post := func(pw string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/account-delete", strings.NewReader(url.Values{"password": {pw}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(rendering.WithCurrentWorkspace(data.WithWorkspaceScope(req.Context(), workspaceID), installed, "Test Workspace", false))
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, record.ID, 0)})
		r := chi.NewRouter()
		r.Post("/account-delete", submitDeleteAccount(store, nil, workspaces, cfg))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	if rec := post("not-the-password"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong password = %d, want 422; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	if outcome := authenticateMember(ctx, store, email, password); outcome != loginOK {
		t.Fatalf("a wrong password deleted the account (%v)", outcome)
	}

	rec := post(password)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/delete-account?done=1" {
		t.Fatalf("deletion = %d to %q, want 303 to /delete-account?done=1; body=%s", rec.Code, rec.Header().Get("Location"), firstChars(rec.Body.String()))
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == authorization.SessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the session cookie was not cleared")
	}
	if outcome := authenticateMember(ctx, store, email, password); outcome == loginOK {
		t.Error("the old address still signs in after deletion")
	}

	pool := tracedTestPool(t)
	var anon string
	if err := pool.QueryRow(ctx, `SELECT email FROM workspace_members WHERE workspace_id = $1 AND user_record_id = $2`, workspaceID, record.ID).Scan(&anon); err != nil {
		t.Fatalf("read anonymized membership: %v", err)
	}
	t.Cleanup(func() { deleteCredentialForTest(t, ctx, anon) })
	if strings.Contains(anon, "delete_account_test") {
		t.Errorf("membership still carries the real address: %q", anon)
	}
}
