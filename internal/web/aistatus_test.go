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

// TestGeneratedStatusFilterIsDrawnAndFiltersByTheTile is roadmap stage 1, W6's end-to-end proof: the tiles page
// counts records per status, each tile links to the second page with ?status=, and that page lists only the
// records in the named status.
func TestGeneratedStatusFilterIsDrawnAndFiltersByTheTile(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const slug, email = "ai-status-test", "ai_status_test@example.com"
	ws, err := store.CreateWorkspace(ctx, "AI Status Test", slug)
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
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
		ID: "app_ai_status", Name: "Tugas", Roles: []string{"staf"}, PublisherRole: "staf",
		Navigation: []aiassist.GeneratedMenuItem{
			{Label: "Per status", MachineID: "mch_ai_status_tugas", ByStatus: &aiassist.GeneratedByStatus{StatusField: "fld_status", DetailLabel: "Dalam status", Limit: 50}},
		},
		Machines: []aiassist.GeneratedMachine{{
			ID: "mch_ai_status_tugas", Name: "Tugas",
			Fields: []aiassist.GeneratedField{
				{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"rencana", "selesai"}, Default: "rencana"},
			},
			CardFields:  []aiassist.GeneratedCardField{{Field: "fld_judul", Role: "title"}, {Field: "fld_status", Role: "status"}},
			Permissions: []aiassist.GeneratedPermission{{ID: "prm_tugas_create", Action: "create", Roles: []string{"staf"}}},
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
	u, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": email})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, ws.ID, u.ID, email, "admin", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMemberAppRole(ctx, ws.ID, u.ID, "app_ai_status", "staf"); err != nil {
		t.Fatal(err)
	}
	for title, status := range map[string]string{"Siapkan bahan": "rencana", "Kirim laporan": "selesai", "Pesan tiket": "rencana"} {
		if _, err := store.CreateRecord(wsCtx, "mch_ai_status_tugas", map[string]any{"fld_judul": title, "fld_status": status}); err != nil {
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
	get := func(route string) string {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, u.ID, 0)})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", route, rec.Code, firstChars(rec.Body.String()))
		}
		return rec.Body.String()
	}
	tiles := get("/pages/nav_ai_status")
	for _, want := range []string{"rencana", "selesai", "?status=rencana", "/pages/nav_ai_status_ai_status_tugas_in_status"} {
		if !strings.Contains(tiles, want) {
			t.Errorf("the tiles page does not draw %q", want)
		}
	}
	detail := get("/pages/nav_ai_status_ai_status_tugas_in_status?status=rencana")
	for _, want := range []string{"Siapkan bahan", "Pesan tiket"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the page a tile opens does not list %q", want)
		}
	}
	if strings.Contains(detail, "Kirim laporan") {
		t.Error("the page for status rencana lists a record in another status")
	}
}
