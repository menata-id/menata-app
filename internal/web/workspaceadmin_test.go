package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// requireWorkspaceAdmin had no test of its own until 2026-09-22, which is uncomfortable for a gate
// whose own doc comment records that it **used to fail open** -- an absent membership row was let
// through, so the gate passed whenever its own lookup found nothing (2026-09-21 authorization
// review). It was corrected then, and covered by nothing.
//
// It got one when the gate was edited for an unrelated reason: it stopped calling
// store.GetMembership (four queries) and started reading the membership resolveIdentity had
// already resolved. A query-count change is not a reason to re-verify an authorization gate by
// reading it, so these four cases are the four branches, stated as behaviour rather than as
// coverage:
//
//	member + admin role      -> through
//	member + non-admin role  -> forbidden
//	no row + configured admin identity -> through (the one intended exception)
//	no row + anyone else     -> forbidden (the arm that once did the opposite)
func TestRequireWorkspaceAdmin(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Admin Gate", "admin-gate-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	const email = "admin_gate@example.com"
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	newMember := func(t *testing.T, name, addr, workspaceRole string) string {
		t.Helper()
		rec, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_email": addr})
		if err != nil {
			t.Fatalf("CreateRecord(%s): %v", name, err)
		}
		if err := store.AddMember(ctx, ws.ID, rec.ID, addr, workspaceRole, ""); err != nil {
			t.Fatalf("AddMember(%s): %v", name, err)
		}
		return rec.ID
	}

	adminID := newMember(t, "admin", email, "admin")
	memberID := newMember(t, "member", "admin_gate_member@example.com", "member")

	cfg := config.Config{SessionSecret: "admin-gate-secret", AdminUserID: "admin", SecureCookies: false}

	// serve runs one request through the gate as subject, with the Workspace scope requireAuth
	// would have established, and reports the status the gate produced.
	serve := func(t *testing.T, subject string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/workspace-members", nil)
		req = req.WithContext(data.WithWorkspaceScope(req.Context(), ws.ID))
		if subject != "" {
			req.AddCookie(&http.Cookie{
				Name:  authorization.SessionCookieName,
				Value: sessionCookieValueForTest(t, cfg, subject, 0),
			})
		}
		rec := httptest.NewRecorder()
		requireWorkspaceAdmin(store, cfg)(passOrFail(t)).ServeHTTP(rec, req)
		return rec.Code
	}

	for _, tc := range []struct {
		name    string
		subject string
		want    int
	}{
		{"a Workspace admin is let through", adminID, http.StatusOK},
		{"a plain member is refused", memberID, http.StatusForbidden},
		{"the configured admin identity has no membership row and is let through", cfg.AdminUserID, http.StatusOK},
		{"an identity with no membership row is refused", "rec_no_such_member", http.StatusForbidden},
		{"an unauthenticated request is refused", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := serve(t, tc.subject); got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}
