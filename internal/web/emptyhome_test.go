package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// Workspace Home of a Workspace with no Application (N03): an admin is welcomed with the way to make the first one -- the box, the library and what
// happens next -- and a member is told why there is nothing; with an Application on, neither appears; with every Application switched off the
// Workspace is empty again to whoever looks.
func TestHomeOfAWorkspaceWithNoApplicationWelcomesAnAdminAndExplainsToAMember(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Empty Home Test", "empty-home-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	member := func(email, role string) string {
		cleanupAuthTest(t, pool, ws.ID, email)
		u, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddMember(ctx, ws.ID, u.ID, email, role, ""); err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	admin, plain := member("empty_home_admin@example.com", "admin"), member("empty_home_member@example.com", "member")

	root := t.TempDir()
	dir := filepath.Join(root, "workspaces")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(root, "user.yaml"), []byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644)
	manifest := filepath.Join(dir, ws.Slug+".yaml")
	os.WriteFile(manifest, []byte("workspace: "+ws.Slug+"\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644)
	cfg := config.Config{SessionSecret: "empty-home-secret", MetadataPath: dir, TemplatePath: realLibrary(t)}

	home := func(userID string) string {
		loaded, err := metadata.LoadApplication(manifest)
		if err != nil {
			t.Fatal(err)
		}
		r := chi.NewRouter()
		r.Get("/home", showRuntimePage(store, cfg, "nav_home").ServeHTTP)
		req := httptest.NewRequest(http.MethodGet, "/home", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, userID, 0)})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req.WithContext(rendering.WithCurrentWorkspace(wsCtx, loaded.Workspace, "Empty Home Test", false)))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /home = %d: %.300s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	const welcome, explain, whatNext = "This workspace has no applications yet.", "No application to open here yet.", "What happens next"

	body := home(admin)
	for _, want := range []string{welcome, "Describe the application you need", whatNext, "See the whole library"} {
		if !strings.Contains(body, want) {
			t.Errorf("an admin's empty Home lacks %q", want)
		}
	}
	if strings.Contains(body, explain) {
		t.Error("an admin is told to ask an admin")
	}
	body = home(plain)
	if !strings.Contains(body, explain) || strings.Contains(body, welcome) || strings.Contains(body, whatNext) {
		t.Errorf("a member's empty Home: explains=%v welcome=%v whatNext=%v", strings.Contains(body, explain), strings.Contains(body, welcome), strings.Contains(body, whatNext))
	}

	// One Application on: the welcome is gone and the list is back.
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: "app_empty_home", Name: "Laporan Kosong", Roles: []string{"staf"}, PublisherRole: "staf",
		Navigation: []aiassist.GeneratedMenuItem{{Label: "Laporan", MachineID: "mch_empty_home"}},
		Machines:   []aiassist.GeneratedMachine{{ID: "mch_empty_home", Name: "Laporan", Fields: []aiassist.GeneratedField{{ID: "fld_judul", Name: "Judul", Type: "text", Required: true}}}},
	}}
	if _, err := aiassist.Write(manifest, change, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMemberAppRole(ctx, ws.ID, admin, "app_empty_home", "staf"); err != nil {
		t.Fatal(err)
	}
	body = home(admin)
	if strings.Contains(body, welcome) || strings.Contains(body, whatNext) || !strings.Contains(body, "Laporan Kosong") {
		t.Errorf("with an Application on, the welcome shows (%v / %v) or the list does not (%v)", strings.Contains(body, welcome), strings.Contains(body, whatNext), strings.Contains(body, "Laporan Kosong"))
	}

	// Switched off, it is gone from the list and the Workspace is empty again.
	if _, err := installer.SetApplicationInactive(manifest, "app_empty_home", true); err != nil {
		t.Fatal(err)
	}
	if body = home(admin); !strings.Contains(body, welcome) || strings.Contains(body, "Laporan Kosong") {
		t.Errorf("with every Application off the Home is not the empty one (welcome %v, still listed %v)", strings.Contains(body, welcome), strings.Contains(body, "Laporan Kosong"))
	}
}
