package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"context"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/storage"
)

// TestGeneratedBoardIsDrawnByTheRuntime is roadmap stage 1, W4's end-to-end proof: an Application the
// assistant generated with a board view and card_fields is published into a Workspace, and the real router
// answers /machines/<id> as a board -- one column per status, each record's title on its card -- rather
// than the generic table. Passing the loader is not the same as being drawn.
func TestGeneratedBoardIsDrawnByTheRuntime(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const slug, email = "ai-board-test", "ai_board_test@example.com"
	ws, err := store.CreateWorkspace(ctx, "AI Board Test", slug)
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	// A Workspace with the runtime's shared Machines, the way a UI-created one has them.
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
		ID: "app_ai_board", Name: "Tugas", Roles: []string{"staf"}, PublisherRole: "staf",
		Navigation: []aiassist.GeneratedMenuItem{{Label: "Tugas", MachineID: "mch_ai_board_tugas"}},
		Machines: []aiassist.GeneratedMachine{{
			ID: "mch_ai_board_tugas", Name: "Tugas",
			Fields: []aiassist.GeneratedField{
				{ID: "fld_judul", Name: "Judul", Type: "text", Required: true},
				{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"rencana", "jalan", "selesai"}, Default: "rencana"},
			},
			CardFields: []aiassist.GeneratedCardField{{Field: "fld_judul", Role: "title"}, {Field: "fld_status", Role: "status"}},
			Views: []aiassist.GeneratedView{
				{ID: "vw_tugas_papan", Name: "Papan", Type: "board", GroupBy: "fld_status"},
				{ID: "vw_tugas_tabel", Name: "Tabel", Type: "table"},
			},
			Permissions: []aiassist.GeneratedPermission{
				{ID: "prm_tugas_create", Action: "create", Roles: []string{"staf"}},
				{ID: "prm_tugas_edit", Action: "edit", Roles: []string{"staf"}},
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

	actor, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": email})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, ws.ID, actor.ID, email, "admin", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMemberAppRole(ctx, ws.ID, actor.ID, "app_ai_board", "staf"); err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{"Siapkan bahan", "Kirim laporan"} {
		status := []string{"rencana", "selesai"}[i]
		if _, err := store.CreateRecord(wsCtx, "mch_ai_board_tugas", map[string]any{"fld_judul": title, "fld_status": status}); err != nil {
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

	req := httptest.NewRequest(http.MethodGet, "/machines/mch_ai_board_tugas", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, actor.ID, 0)})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", rec.Code, firstChars(out))
	}
	for _, want := range []string{"grouped by Status", "Siapkan bahan", "Kirim laporan", `aria-label="rencana"`, `aria-label="selesai"`} {
		if !strings.Contains(out, want) {
			t.Errorf("the generated board does not draw %q", want)
		}
	}
	if strings.Contains(out, "<table") {
		t.Error("the page drew the generic table instead of the generated board")
	}
}

func userMachineOf(t *testing.T, app *metadata.App) *domain.Machine {
	t.Helper()
	for _, m := range app.Machines {
		if m.ID == domain.UserMachineID {
			return m
		}
	}
	t.Fatal("no user machine loaded")
	return nil
}
