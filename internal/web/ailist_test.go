package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/storage"
)

// TestGeneratedListsShowEachViewerTheirOwnRecords is roadmap stage 1, W5-a's end-to-end proof: an Application
// the assistant generated with a "mine" list and an "all" list is published, and the real router answers
// /pages/<id> -- the "mine" page shows each of two people only the records that name them, the "all" page
// shows both. Passing the loader is not the same as scoping by who is looking.
func TestGeneratedListsShowEachViewerTheirOwnRecords(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const slug, emailA, emailB = "ai-list-test", "ai_list_a@example.com", "ai_list_b@example.com"
	ws, err := store.CreateWorkspace(ctx, "AI List Test", slug)
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, emailA)
	cleanupAuthTest(t, pool, ws.ID, emailB)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	root := t.TempDir()
	for _, f := range []string{"user.yaml", "activity.yaml", "notification.yaml"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "metadata", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "workspaces", slug+".yaml")
	if err := os.WriteFile(manifest, []byte("workspace: "+slug+"\nmachines:\n  - ../activity.yaml\n  - ../notification.yaml\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: "app_ai_list", Name: "Permintaan", Roles: []string{"staf"}, PublisherRole: "staf",
		Navigation: []aiassist.GeneratedMenuItem{
			{Label: "Semua", MachineID: "mch_ai_list_minta", List: &aiassist.GeneratedList{Scope: aiassist.ListScopeAll, Limit: 50}},
			{Label: "Milik saya", MachineID: "mch_ai_list_minta", List: &aiassist.GeneratedList{Scope: aiassist.ListScopeMine, PersonField: "fld_pemohon", Limit: 50}},
		},
		Machines: []aiassist.GeneratedMachine{{
			ID: "mch_ai_list_minta", Name: "Permintaan",
			Fields: []aiassist.GeneratedField{
				{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
				{ID: "fld_pemohon", Name: "Pemohon", Type: "person"},
			},
			CardFields: []aiassist.GeneratedCardField{{Field: "fld_judul", Role: "title"}, {Field: "fld_pemohon", Role: "person"}},
			Permissions: []aiassist.GeneratedPermission{
				{ID: "prm_minta_create", Action: "create", Roles: []string{"staf"}},
			},
		}},
	}}
	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := aiassist.Validate(change, aiassist.ExistingStateFrom(loaded.Workspace)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := aiassist.Write(manifest, change, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err = metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatalf("the generated Workspace does not load: %v", err)
	}
	installed := loaded.Workspace
	installed.Slug = ws.Slug

	member := func(email string) *data.Record {
		u, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": email})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddMember(ctx, ws.ID, u.ID, email, "admin", ""); err != nil {
			t.Fatal(err)
		}
		if err := store.SetMemberAppRole(ctx, ws.ID, u.ID, "app_ai_list", "staf"); err != nil {
			t.Fatal(err)
		}
		return u
	}
	a, b := member(emailA), member(emailB)
	for title, owner := range map[string]string{"Laptop baru": a.ID, "Kursi kantor": b.ID} {
		if _, err := store.CreateRecord(wsCtx, "mch_ai_list_minta", map[string]any{"fld_judul": title, "fld_pemohon": owner}); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.Config{SessionSecret: slug + "-secret", TemplatePath: realLibrary(t)}
	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := Routes(Deps{UserMachine: userMachineOf(t, loaded), Store: store, Files: files, Cfg: cfg,
		Workspaces: map[string]domain.Workspace{slug: installed}, DefaultWorkspaceID: ws.ID})
	get := func(userID, route string) string {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, userID, 0)})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", route, rec.Code, firstChars(rec.Body.String()))
		}
		return rec.Body.String()
	}
	mineRoute, allRoute := "/pages/nav_ai_list_ai_list_minta_mine", "/pages/nav_ai_list"
	for _, route := range []string{mineRoute, allRoute} {
		found := false
		for _, n := range installed.Applications[0].AllNavigation {
			found = found || n.Route == route
		}
		if !found {
			t.Fatalf("the generated menu has no entry for %s: %+v", route, installed.Applications[0].AllNavigation)
		}
	}
	for _, c := range []struct {
		viewer       *data.Record
		sees, hidden string
	}{{a, "Laptop baru", "Kursi kantor"}, {b, "Kursi kantor", "Laptop baru"}} {
		out := get(c.viewer.ID, mineRoute)
		if !strings.Contains(out, c.sees) {
			t.Errorf("the \"mine\" page does not show %s its own record %q", c.viewer.ID, c.sees)
		}
		if strings.Contains(out, c.hidden) {
			t.Errorf("the \"mine\" page shows %s a record that names someone else: %q", c.viewer.ID, c.hidden)
		}
	}
	all := get(a.ID, allRoute)
	for _, want := range []string{"Laptop baru", "Kursi kantor"} {
		if !strings.Contains(all, want) {
			t.Errorf("the \"all\" page does not show %q", want)
		}
	}
}
