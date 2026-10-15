package web

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
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

// TestPublish_anUpdateWritesAnAIUpdateLineWithTheApplicationsAge is roadmap stage 1, WU. The assistant
// publishes a new application, then (a moment later) an update to it; the second publication must write
// one AI-UPDATE line naming the application, its age since the assistant first published it, and the plan's
// adds. An update to an application the assistant did not create must say the age is unknown, never zero.
func TestPublish_anUpdateWritesAnAIUpdateLineWithTheApplicationsAge(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "ai_update_test@example.com"
	ws, err := store.CreateWorkspace(ctx, "AI Update Test", "ai-update-test-workspace")
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

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "user.yaml"), []byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, ws.Slug+".yaml")
	if err := os.WriteFile(manifest, []byte("workspace: "+ws.Slug+"\nmachines:\n  - user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{SessionSecret: "ai-update-test-secret", MetadataPath: dir}

	publish := func(change aiassist.GeneratedChange, target string) *httptest.ResponseRecorder {
		t.Helper()
		loaded, err := metadata.LoadApplication(manifest)
		if err != nil {
			t.Fatal(err)
		}
		content, _ := json.Marshal(aiassist.Reply{Message: "ok", Change: &change})
		session, err := store.CreateAISession(wsCtx, ws.ID, user.ID, target)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AppendAISessionTurn(wsCtx, session.ID, "model", string(content)); err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateAISessionStatus(wsCtx, ws.ID, session.ID, data.AISessionStatusGenerated); err != nil {
			t.Fatal(err)
		}
		r := chi.NewRouter()
		r.Post("/new-application/{session}/publish", publishNewApplication(store, nil, cfg, func(string) error { return nil }))
		req := httptest.NewRequest(http.MethodPost, "/new-application/"+session.ID+"/publish", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, user.ID, 0)})
		req = req.WithContext(rendering.WithCurrentWorkspace(wsCtx, loaded.Workspace, "AI Update Test", false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	created := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{
		ID: "app_ai_update_test", Name: "Laporan", Roles: []string{"staf"}, PublisherRole: "staf",
		Navigation: []aiassist.GeneratedMenuItem{{Label: "Laporan", MachineID: "mch_ai_update_laporan"}},
		Machines: []aiassist.GeneratedMachine{{ID: "mch_ai_update_laporan", Name: "Laporan",
			Fields: []aiassist.GeneratedField{{ID: "fld_judul", Name: "Judul", Type: "text", Required: true}}}},
	}}
	if rec := publish(created, aiassist.KindNewApplication); rec.Code != http.StatusSeeOther {
		t.Fatalf("publish new = %d: %s", rec.Code, rec.Body.String())
	}

	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatal(err)
	}
	cur := aiassist.ExistingStateFrom(loaded.Workspace).Applications["app_ai_update_test"]
	desired := aiassist.DescribeApplication("app_ai_update_test", cur)
	desired.Machines[0].Fields = append(desired.Machines[0].Fields, aiassist.GeneratedField{ID: "fld_catatan", Name: "Catatan", Type: "text"})
	update := aiassist.GeneratedChange{Kind: aiassist.KindUpdateApplication, TargetAppID: "app_ai_update_test", Application: &desired}

	var logged bytes.Buffer
	log.SetOutput(&logged)
	rec := publish(update, "app_ai_update_test")
	log.SetOutput(os.Stderr)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publish update = %d: %s", rec.Code, rec.Body.String())
	}
	out := logged.String()
	for _, want := range []string{"AI-UPDATE app=app_ai_update_test", "adds=1 changes=0 removes=0", "Catatan"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "age_hours=unknown") {
		t.Errorf("an application the assistant just created reported an unknown age:\n%s", out)
	}
	if !strings.Contains(out, "age_hours=0.0") {
		t.Errorf("a just-created application should be about 0.0 hours old:\n%s", out)
	}

	// an application the assistant did not create: unknown, never zero
	logged.Reset()
	log.SetOutput(&logged)
	logAIUpdate(ws.ID, "aise_x", "app_handwritten", time.Time{}, false, aiassist.Plan{}, time.Time{})
	log.SetOutput(os.Stderr)
	if !strings.Contains(logged.String(), "age_hours=unknown") {
		t.Errorf("an application with no known origin did not say so:\n%s", logged.String())
	}
}

// The age is the time since the assistant first published the application, in hours with one decimal.
func TestLogAIUpdate_reportsTheAgeInHours(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	logAIUpdate("ws_1", "aise_1", "app_x", now.Add(-90*time.Minute), true, aiassist.Plan{Items: []aiassist.PlanItem{
		{Op: "add", What: "field A"}, {Op: "change", What: "rename B"}, {Op: "remove", What: "field C"}, {Op: "add", What: "field D"}}}, now)
	for _, want := range []string{"age_hours=1.5", "adds=2 changes=1 removes=1", "remove: field C"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, logged.String())
		}
	}
}
