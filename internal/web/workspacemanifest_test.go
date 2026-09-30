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

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// These reproduce 2026-09-30 exactly: a Workspace created through the UI had a `workspaces` row and
// no manifest, its requests carried the zero Workspace, and the first publish into it aimed at
// metadata/workspaces/.yaml. menata-app-document's audits/2026-09-30-kajian-workspace-baru-tanpa-manifest.md.

func realLibrary(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "metadata"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

type manifestlessSetup struct {
	store   *data.Store
	ws      *data.Workspace
	userID  string
	session *data.AISession
	wsCtx   context.Context
}

// newManifestlessWorkspace is a Workspace as the UI used to leave it: a row, an admin, a generated
// proposal waiting to publish, and nothing on disk.
func newManifestlessWorkspace(t *testing.T, name, email string) manifestlessSetup {
	t.Helper()
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, name, strings.ToLower(strings.ReplaceAll(name, " ", "-")))
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	user, err := store.CreateRecord(wsCtx, domain.UserMachineID, map[string]any{"fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(user): %v", err)
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	change := aiassist.GeneratedChange{
		Kind: aiassist.KindNewApplication,
		Application: &aiassist.GeneratedApplication{
			ID: "app_branch_waste", Name: "Branch Waste",
			Roles: []string{"approver", "reporter"}, PublisherRole: "approver",
			Navigation: []aiassist.GeneratedMenuItem{{Label: "Cabang", MachineID: "mch_branch"}},
			Machines: []aiassist.GeneratedMachine{{
				ID: "mch_branch", Name: "Branch",
				Fields: []aiassist.GeneratedField{{ID: "fld_branch_name", Name: "Branch Name", Type: "text", Required: true}},
			}},
		},
	}
	content, err := json.Marshal(aiassist.Reply{Message: "Here it is.", Change: &change})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateAISession(wsCtx, ws.ID, user.ID, aiassist.KindNewApplication)
	if err != nil {
		t.Fatalf("CreateAISession: %v", err)
	}
	if err := store.AppendAISessionTurn(wsCtx, session.ID, "model", string(content)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAISessionStatus(wsCtx, ws.ID, session.ID, data.AISessionStatusGenerated); err != nil {
		t.Fatal(err)
	}
	return manifestlessSetup{store: store, ws: ws, userID: user.ID, session: session, wsCtx: wsCtx}
}

func (s manifestlessSetup) publish(t *testing.T, cfg config.Config, ai aiassist.Client) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/new-application/{session}/publish", publishNewApplication(s.store, ai, cfg, func() error { return nil }))
	req := httptest.NewRequest(http.MethodPost, "/new-application/"+s.session.ID+"/publish", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, s.userID, 0)})
	// The zero Workspace: what currentWorkspace resolves for a Workspace with no manifest.
	req = req.WithContext(rendering.WithCurrentWorkspace(s.wsCtx, domain.Workspace{}, s.ws.Name, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestPublishNewApplication_healsAWorkspaceCreatedWithoutAManifest: the publish writes the missing
// installation, then publishes into it.
func TestPublishNewApplication_healsAWorkspaceCreatedWithoutAManifest(t *testing.T) {
	s := newManifestlessWorkspace(t, "Manifestless Heal Test", "manifestless_heal@example.com")
	metadataDir := t.TempDir()
	cfg := config.Config{SessionSecret: "manifestless-heal-secret", MetadataPath: metadataDir, TemplatePath: realLibrary(t)}

	rec := s.publish(t, cfg, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publish = %d, want 303; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/machines/mch_branch" {
		t.Errorf("Location = %q, want the new Application's first Machine", got)
	}
	loaded, err := metadata.LoadWorkspaces(metadataDir)
	if err != nil {
		t.Fatalf("the healed directory does not load: %v", err)
	}
	ws, ok := loaded[s.ws.Slug]
	if !ok || len(ws.Workspace.Applications) != 1 || ws.Workspace.Applications[0].ID != "app_branch_waste" {
		t.Fatalf("the Workspace's manifest does not install the published Application: %+v", ws.Workspace.Applications)
	}
	if _, err := os.Stat(filepath.Join(metadataDir, ".yaml")); err == nil {
		t.Error("metadata/workspaces/.yaml was written")
	}
}

// TestPublishNewApplication_anEnvironmentFailureStaysOutOfTheConversation: a failure no proposal can
// fix is shown to the person, adds no turn, and leaves the draft publishable. Here the manifest is
// missing and cannot be written, because no template library is configured.
func TestPublishNewApplication_anEnvironmentFailureStaysOutOfTheConversation(t *testing.T) {
	s := newManifestlessWorkspace(t, "Manifestless Env Test", "manifestless_env@example.com")
	cfg := config.Config{SessionSecret: "manifestless-env-secret", MetadataPath: t.TempDir()}
	ai := &recordingAIClient{reply: aiassist.Reply{Message: "Please try again."}}

	rec := s.publish(t, cfg, ai)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("publish = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not because of the proposed application") {
		t.Errorf("the page does not say the proposal was not at fault: %s", rec.Body.String())
	}
	if ai.sawTurns != nil {
		t.Error("the assistant was asked to fix a server-side failure")
	}
	after, err := s.store.GetAISession(s.wsCtx, s.ws.ID, s.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Turns) != 1 {
		t.Errorf("session has %d turns, want the 1 it had -- nothing may be added for an environment failure", len(after.Turns))
	}
	if after.Status != data.AISessionStatusGenerated {
		t.Errorf("session status = %q, want it still %q so Publish can be pressed again", after.Status, data.AISessionStatusGenerated)
	}
}

// TestRegisterWorkspace_writesTheWorkspacesManifest: a Workspace created by sign-up has its
// installation from the first request, rather than on its first write.
func TestRegisterWorkspace_writesTheWorkspacesManifest(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "register_manifest@example.com"
	metadataDir := t.TempDir()
	cfg := config.Config{MetadataPath: metadataDir, TemplatePath: realLibrary(t)}

	if err := registerWorkspace(ctx, store, cfg, "Register Manifest Test", "Reg Tester", email, "password123", map[string]any{}); err != nil {
		t.Fatalf("registerWorkspace: %v", err)
	}
	ws, err := store.WorkspaceBySlug(ctx, "register-manifest-test")
	if err != nil {
		t.Fatalf("WorkspaceBySlug: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)

	loaded, err := metadata.LoadWorkspaces(metadataDir)
	if err != nil {
		t.Fatalf("does not load: %v", err)
	}
	if _, ok := loaded["register-manifest-test"]; !ok {
		t.Error("sign-up created a Workspace with no manifest")
	}
}
