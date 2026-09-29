package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/data"
)

// TestShowInference_rendersResolvedDerivationsAndTheirSources is the screen's baseline: a real
// authenticated request against the real router, asserting the page states both halves of what 001 #6
// asks for -- the resolved value *and* the declaration it was read from.
//
// Asserting the source matters as much as the value: a page that printed only "fld_decision" would
// tell a reader what the runtime decided without letting them check it, which is the difference
// between a diagnostic and a claim.
func TestShowInference_rendersResolvedDerivationsAndTheirSources(t *testing.T) {
	h, cookie := newRouterTestSetup(t, "inference")

	req := httptest.NewRequest(http.MethodGet, "/inference", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /inference = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// A resolved value, the declaration it came from, and the role that owes it.
	for _, want := range []string{"fld_decision", "transitions[action=decide].field", "decision", "step"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not mention %q -- a derivation the runtime makes is not explained", want)
		}
	}
	// The vocabulary itself has to be on the page: a status a reader cannot name is not an explanation.
	for _, want := range []string{"resolved", "not applicable"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not render the status %q", want)
		}
	}
}

// TestShowInference_isWorkspaceAdminOnly holds the access decision (owner, 2026-09-29: "admin only").
// It is asserted rather than assumed because the gate is one line in router.go's ar group, and a route
// moved out of that group would keep working -- for everyone.
func TestShowInference_isWorkspaceAdminOnly(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupParts(t, "inference_member")
	workspaceID, ok := data.WorkspaceScope(ctx)
	if !ok {
		t.Fatal("the fixture ctx carries no Workspace scope")
	}

	// Demote the viewer to a plain member; the fixture makes them an admin.
	if err := store.UpdateMemberRole(ctx, workspaceID, actorID, "member", ""); err != nil {
		t.Fatalf("demote the fixture viewer to member: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/inference", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("GET /inference returned 200 for a plain member -- this screen describes how the " +
			"Workspace is assembled and is admin-only by the same reasoning /install-application is")
	}
}
