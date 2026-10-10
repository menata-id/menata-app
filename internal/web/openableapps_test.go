package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// TestCanOpenApplication pins the predicate requireApplicationAccess gates with, which the
// Application lists now ask too (S0.1): a role-less Application is open to everyone, a role-bearing
// one only to a holder of some role there, and the shared admin credential to everything.
func TestCanOpenApplication(t *testing.T) {
	withRoles := domain.Application{ID: "app_a", Roles: []string{"approver"}}
	roleless := domain.Application{ID: "app_b"}
	cfg := config.Config{AdminUserID: "usr_admin"}

	cases := []struct {
		name  string
		app   domain.Application
		actor domain.Actor
		want  bool
	}{
		{"role-less, anyone", roleless, domain.Actor{ID: "usr_x"}, true},
		{"role-less, unidentified", roleless, domain.Actor{}, true},
		{"roles declared, none held", withRoles, domain.Actor{ID: "usr_x"}, false},
		{"roles declared, held elsewhere only", withRoles, domain.Actor{ID: "usr_x", Roles: map[string][]string{"app_b": {"approver"}}}, false},
		{"roles declared, one held", withRoles, domain.Actor{ID: "usr_x", Roles: map[string][]string{"app_a": {"approver"}}}, true},
		{"shared admin credential", withRoles, domain.Actor{ID: "usr_admin"}, true},
		{"empty id never matches an empty AdminUserID", withRoles, domain.Actor{}, false},
	}
	for _, tc := range cases {
		c := cfg
		if tc.name == "empty id never matches an empty AdminUserID" {
			c = config.Config{}
		}
		if got := canOpenApplication(tc.app, tc.actor, c); got != tc.want {
			t.Errorf("%s: canOpenApplication = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestWorkspaceHomeAndLauncherListOnlyOpenableApplications is S0.1's "done means" through the real
// middleware chain: a member with no role in a role-bearing Application sees it neither as a Home
// card nor in the launcher, still sees a role-less one, and sees both once given a role; the shared
// admin credential sees everything. Before S0.1 the first case rendered a card ("Your role: —") whose
// link answered 403.
func TestWorkspaceHomeAndLauncherListOnlyOpenableApplications(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()

	ws, err := store.CreateWorkspace(ctx, "Openable Apps", "openable-apps-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, "openable_apps@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	member, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_name": "Rina", "fld_email": "openable_apps@example.com"})
	if err != nil {
		t.Fatalf("CreateRecord(user): %v", err)
	}
	if err := store.AddMember(wsCtx, ws.ID, member.ID, "openable_apps@example.com", "member", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	const gated, open = "Gated Approvals", "Open Board"
	workspace := domain.Workspace{
		Slug: ws.Slug,
		Applications: []domain.Application{
			{ID: "app_gated", Name: gated, Roles: []string{"approver"}},
			{ID: "app_open", Name: open},
		},
	}

	home := func(cfg config.Config) string {
		r := chi.NewRouter()
		r.Use(resolveIdentity(store, cfg))
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(rendering.WithCurrentWorkspace(req.Context(), workspace, "Openable Apps", false)))
			})
		})
		r.Get("/home", showWorkspaceHome(store, cfg))
		req := httptest.NewRequest(http.MethodGet, "/home", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, member.ID, 0)})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req.WithContext(wsCtx))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /home: status %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	// launcher is the part of the page from the launcher's popover onwards, so a name found there is
	// the launcher's and not Home's card list.
	launcher := func(body string) string {
		_, after, ok := strings.Cut(body, `id="menata-app-launcher"`)
		if !ok {
			t.Fatal("launcher popover not rendered")
		}
		return after
	}

	cfg := config.Config{SessionSecret: "test-secret-for-openable-apps"}
	body := home(cfg)
	if n := strings.Count(body, gated); n != 0 {
		t.Errorf("no role: %q appears %d times on /home, want 0 -- its card and launcher row would only answer 403", gated, n)
	}
	if !strings.Contains(launcher(body), open) || strings.Count(body, open) < 2 {
		t.Errorf("no role: %q should still be a Home card and a launcher row -- it declares no roles", open)
	}

	if err := store.SetMemberAppRole(wsCtx, ws.ID, member.ID, "app_gated", "approver"); err != nil {
		t.Fatalf("SetMemberAppRole: %v", err)
	}
	body = home(cfg)
	if !strings.Contains(launcher(body), gated) || strings.Count(body, gated) < 2 {
		t.Errorf("approver: %q should be a Home card and a launcher row once a role is held", gated)
	}
	if err := store.SetMemberAppRole(wsCtx, ws.ID, member.ID, "app_gated", ""); err != nil {
		t.Fatalf("SetMemberAppRole(clear): %v", err)
	}
	if n := strings.Count(home(cfg), gated); n != 0 {
		t.Fatalf("role cleared: %q appears %d times, want 0 -- the fixture did not reset", gated, n)
	}

	admin := cfg
	admin.AdminUserID = member.ID
	body = home(admin)
	if !strings.Contains(launcher(body), gated) || !strings.Contains(launcher(body), open) {
		t.Errorf("shared admin credential: the launcher should list both Applications")
	}
}
