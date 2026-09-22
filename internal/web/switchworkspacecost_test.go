package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestSwitchWorkspaceCostIsFlatInWorkspaceCount is the one assertion the sweep cannot make.
//
// /switch-workspace was the only true N+1 the 2026-09-22 query audit found: loadWorkspaceChoices
// called GetWorkspace once per membership. It showed up as `repeated=1` -- a number small enough
// to look like any of the other six findings -- purely because the identity it was measured with
// belonged to one Workspace. **`repeated=0` would not have said it was fixed**, and does not say
// so now: a loop that reads one row per membership repeats nothing when there is one membership.
//
// So this measures the shape instead of the number: the same request, by identities belonging to
// one Workspace and to three, must cost the same. That statement is false for any per-row fetch
// and stays true for the join that replaced it (data.ListMemberships).
func TestSwitchWorkspaceCostIsFlatInWorkspaceCount(t *testing.T) {
	one, _ := switchWorkspaceQueries(t, "switchcost1", 1)
	three, body := switchWorkspaceQueries(t, "switchcost3", 3)

	t.Logf("/switch-workspace: 1 workspace = %d queries, 3 workspaces = %d", one, three)
	if one == 0 {
		t.Fatal("/switch-workspace issued no queries -- this test is measuring nothing")
	}
	if three != one {
		t.Errorf("/switch-workspace cost %d queries for an identity in 3 Workspaces against %d for one\n"+
			"  the cost of this screen must not track how many Workspaces the viewer belongs to --\n"+
			"  a per-membership fetch is back (loadWorkspaceChoices), where ListMemberships' join belongs",
			three, one)
	}

	// The count alone would pass just as happily if the join returned the wrong name, or none:
	// fewer queries and a blank screen is not the trade being made here. Nothing asserted this
	// before the name moved from a per-row GetWorkspace onto ListMemberships' join, which is
	// exactly when it started needing an assertion.
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("switchcost3-%d", i)
		if !strings.Contains(body, name) {
			t.Errorf("Choose Workspace does not name %q -- ListMemberships' joined WorkspaceName is not reaching the page", name)
		}
	}
}

// switchWorkspaceQueries builds an identity belonging to workspaceCount Workspaces and returns
// what one GET /switch-workspace costs it, plus the page it rendered.
func switchWorkspaceQueries(t *testing.T, name string, workspaceCount int) (int, string) {
	t.Helper()
	pool := tracedTestPool(t)
	store := data.NewStore(pool)
	cfg := config.Config{SessionSecret: name + "-secret", SecureCookies: false}
	ctx := context.Background()
	email := name + "@example.com"

	machines := realMachines(t)
	var firstWorkspaceID, subject string
	workspaces := map[string]domain.Workspace{}

	for i := 0; i < workspaceCount; i++ {
		slug := fmt.Sprintf("%s-%d", name, i)
		ws, err := store.CreateWorkspace(ctx, slug, slug)
		if err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", slug, err)
		}
		cleanupAuthTest(t, pool, ws.ID, email)

		user, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), domain.UserMachineID,
			map[string]any{"fld_email": email})
		if err != nil {
			t.Fatalf("CreateRecord(user in %s): %v", slug, err)
		}
		if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
			t.Fatalf("AddMember(%s): %v", slug, err)
		}

		installed := testWorkspaceFor(machines)
		installed.Slug = ws.Slug
		workspaces[ws.Slug] = installed
		if i == 0 {
			firstWorkspaceID, subject = ws.ID, user.ID
		}
	}

	h := Routes(Deps{
		Machines:           machines,
		Store:              store,
		Cfg:                cfg,
		Workspaces:         workspaces,
		DefaultWorkspaceID: firstWorkspaceID,
	})

	req := httptest.NewRequest(http.MethodGet, "/switch-workspace", nil)
	req.AddCookie(&http.Cookie{
		Name:  authorization.SessionCookieName,
		Value: sessionCookieValueForTest(t, cfg, subject, 0),
	})
	queries, _, _, _ := serveAndCount(t, h, req)

	// serveAndCount discards the body, so the page is rendered once more for the assertion on
	// its contents. A second request costs nothing here and keeps serveAndCount's own shape --
	// it is shared with the sweep, which wants counts and nothing else.
	rec := httptest.NewRecorder()
	body := httptest.NewRequest(http.MethodGet, "/switch-workspace", nil)
	body.AddCookie(&http.Cookie{
		Name:  authorization.SessionCookieName,
		Value: sessionCookieValueForTest(t, cfg, subject, 0),
	})
	h.ServeHTTP(rec, body)
	return queries, rec.Body.String()
}
