package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// An Application the assistant just published can be taken back: its files and roles go and the proposal is open again. Not after
// the ten minutes, not once anyone has added a record, and not an Application the assistant did not publish.
func TestUndoPublishTakesBackOnlyWhatIsRecentUnusedAndTheAssistants(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "undo_publish_test@example.com"
	ws, err := store.CreateWorkspace(ctx, "Undo Test", "undo-publish-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	user, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_email": email})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "workspaces")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(root, "user.yaml"), []byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644)
	manifest := filepath.Join(dir, ws.Slug+".yaml")
	os.WriteFile(manifest, []byte("workspace: "+ws.Slug+"\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644)
	cfg := config.Config{SessionSecret: "undo-publish-secret", MetadataPath: dir}

	newApp := func(id string) aiassist.GeneratedChange {
		return aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
			ID: id, Name: "Laporan " + id, Roles: []string{"staf"}, PublisherRole: "staf",
			Navigation: []aiassist.GeneratedMenuItem{{Label: "Laporan", MachineID: "mch_" + id}},
			Machines: []aiassist.GeneratedMachine{{ID: "mch_" + id, Name: "Laporan",
				Fields: []aiassist.GeneratedField{{ID: "fld_judul", Name: "Judul", Type: "text", Required: true}}}},
		}}
	}
	workspaceNow := func() context.Context {
		loaded, err := metadata.LoadApplication(manifest)
		if err != nil {
			t.Fatal(err)
		}
		return rendering.WithCurrentWorkspace(wsCtx, loaded.Workspace, "Undo Test", false)
	}
	// publishViaAssistant runs the real publish route, so the session, its publication time and the publisher's role are the real ones.
	publishViaAssistant := func(id string) string {
		change := newApp(id)
		content, _ := json.Marshal(aiassist.Reply{Message: "ok", Change: &change})
		session, err := store.CreateAISession(wsCtx, ws.ID, user.ID, aiassist.KindNewApplication)
		if err != nil {
			t.Fatal(err)
		}
		store.AppendAISessionTurn(wsCtx, session.ID, "model", string(content))
		store.UpdateAISessionStatus(wsCtx, ws.ID, session.ID, data.AISessionStatusGenerated)
		r := chi.NewRouter()
		r.Post("/new-application/{session}/publish", publishNewApplication(store, nil, cfg, func(string) error { return nil }))
		req := httptest.NewRequest(http.MethodPost, "/new-application/"+session.ID+"/publish", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, user.ID, 0)})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req.WithContext(workspaceNow()))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("publish %s = %d: %s", id, rec.Code, rec.Body.String())
		}
		return session.ID
	}
	var reloads int
	at := time.Now()
	env := actionEnv{store: store, cfg: cfg, reload: func(string) error { reloads++; return nil }, now: func() time.Time { return at }}
	undo := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		runUndoPublish(rec, httptest.NewRequest(http.MethodPost, "/runtime-actions/undo_publish?record="+id, nil).WithContext(workspaceNow()), env, id)
		return rec
	}
	installed := func(id string) bool {
		loaded, err := metadata.LoadApplication(manifest)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := loaded.Workspace.ApplicationByID(id)
		return ok
	}

	// Within the window: taken back, its roles gone, the proposal open for review again.
	session := publishViaAssistant("app_undo_a")
	at = time.Now().Add(5 * time.Minute)
	if rec := undo("app_undo_a"); rec.Code != http.StatusOK || reloads != 1 || installed("app_undo_a") {
		t.Fatalf("undo within the window = %d (%s), reloads %d, still installed: %v", rec.Code, rec.Body.String(), reloads, installed("app_undo_a"))
	}
	if s, err := store.GetAISession(wsCtx, ws.ID, session); err != nil || s.Status != data.AISessionStatusGenerated {
		t.Errorf("the proposal after an undo: %v, %v -- want it open for review again", s, err)
	}
	if members, _ := store.ListMembers(ctx, ws.ID); len(members) != 1 || members[0].AppRoles["app_undo_a"] != "" {
		t.Errorf("the publisher still holds a role in the undone Application: %v", members)
	}
	if _, err := os.Stat(filepath.Join(dir, ws.Slug, "undo_a.yaml")); !os.IsNotExist(err) {
		t.Error("the Machine file is still there")
	}

	// Past the window.
	publishViaAssistant("app_undo_b")
	at = time.Now().Add(11 * time.Minute)
	if rec := undo("app_undo_b"); rec.Code != http.StatusUnprocessableEntity || !installed("app_undo_b") {
		t.Errorf("undo past the window = %d, installed %v, want 422 and still installed", rec.Code, installed("app_undo_b"))
	}

	// Anyone added a record.
	publishViaAssistant("app_undo_c")
	if _, err := store.CreateRecord(wsCtx, "mch_app_undo_c", map[string]any{"fld_judul": "x"}); err != nil {
		t.Fatal(err)
	}
	at = time.Now().Add(2 * time.Minute)
	if rec := undo("app_undo_c"); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "records") || !installed("app_undo_c") {
		t.Errorf("undo with a record = %d %q, installed %v", rec.Code, rec.Body.String(), installed("app_undo_c"))
	}

	// An Application the assistant did not publish.
	change := newApp("app_undo_d")
	if _, err := aiassist.Write(manifest, change, nil); err != nil {
		t.Fatal(err)
	}
	if rec := undo("app_undo_d"); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "only an application the assistant published") || !installed("app_undo_d") {
		t.Errorf("undo of a hand-installed Application = %d, installed %v", rec.Code, installed("app_undo_d"))
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM records WHERE workspace_id = $1`, ws.ID)
	})
}
