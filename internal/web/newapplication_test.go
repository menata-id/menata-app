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
	"menata.app/internal/rendering"
)

// createGeneratedSession is the fixture every draft-Application test below needs: a session that
// has reached "generated" status, with a stored model turn carrying the identical shape
// latestChange (newapplication.go) decodes -- no Gemini call involved, matching production.
func createGeneratedSession(t *testing.T, ctx context.Context, store *data.Store, workspaceID, name string) *data.AISession {
	t.Helper()
	session, err := store.CreateAISession(ctx, workspaceID, "usr_test_actor", aiassist.KindNewApplication)
	if err != nil {
		t.Fatalf("CreateAISession: %v", err)
	}
	reply := aiassist.Reply{
		Message: "Here is what I'll build.",
		Change: &aiassist.GeneratedChange{
			Kind: aiassist.KindNewApplication,
			Application: &aiassist.GeneratedApplication{
				ID: "app_leave_permits", Name: name, Description: "Generated from your description.",
			},
		},
	}
	content, err := json.Marshal(reply)
	if err != nil {
		t.Fatalf("marshal reply: %v", err)
	}
	if err := store.AppendAISessionTurn(ctx, session.ID, "model", string(content)); err != nil {
		t.Fatalf("AppendAISessionTurn: %v", err)
	}
	if err := store.UpdateAISessionStatus(ctx, session.ID, data.AISessionStatusGenerated); err != nil {
		t.Fatalf("UpdateAISessionStatus: %v", err)
	}
	session.Status = data.AISessionStatusGenerated
	return session
}

// TestShowHomeDraftApplications_listsGeneratedSession is the row Workspace Home's own lazy
// hx-get renders (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27): a session that has reached
// "generated" status shows up, named and linked to its own review screen.
func TestShowHomeDraftApplications_listsGeneratedSession(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "draft_apps_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Draft Apps Test", "draft-apps-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	session := createGeneratedSession(t, wsCtx, store, ws.ID, "Leave Requests")

	req := httptest.NewRequest(http.MethodGet, "/api/home/draft-applications", nil)
	req = req.WithContext(wsCtx)
	rec := httptest.NewRecorder()
	showHomeDraftApplications(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Leave Requests") {
		t.Errorf("body = %q, want it to name the generated Application", body)
	}
	if !strings.Contains(body, "/new-application/"+session.ID+"/review") {
		t.Errorf("body = %q, want a link to the session's own review screen", body)
	}
}

// TestShowHomeDraftApplications_omitsOtherStatuses is the negative case: only "generated"
// sessions are drafts ready to show -- open (still mid-conversation), published (already a real
// Application, listed there instead) and discarded ones must not appear.
func TestShowHomeDraftApplications_omitsOtherStatuses(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "draft_apps_omit_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Draft Apps Omit Test", "draft-apps-omit-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	if _, err := store.CreateAISession(wsCtx, ws.ID, "usr_test_actor", aiassist.KindNewApplication); err != nil {
		t.Fatalf("CreateAISession(open): %v", err)
	}
	published := createGeneratedSession(t, wsCtx, store, ws.ID, "Already Published")
	if err := store.UpdateAISessionStatus(wsCtx, published.ID, data.AISessionStatusPublished); err != nil {
		t.Fatalf("UpdateAISessionStatus(published): %v", err)
	}
	discarded := createGeneratedSession(t, wsCtx, store, ws.ID, "Discarded Idea")
	if err := store.UpdateAISessionStatus(wsCtx, discarded.ID, data.AISessionStatusDiscarded); err != nil {
		t.Fatalf("UpdateAISessionStatus(discarded): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/home/draft-applications", nil)
	req = req.WithContext(wsCtx)
	rec := httptest.NewRecorder()
	showHomeDraftApplications(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Already Published") || strings.Contains(body, "Discarded Idea") {
		t.Errorf("body = %q, want neither published nor discarded sessions listed", body)
	}
}

// TestShowNewApplication_prefillsIdeaWhenNoSessionYet is Workspace Home's own "Add an
// application" chips/box (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27): a plain ?idea= query
// param pre-fills the first message, and only when no conversation has started yet.
func TestShowNewApplication_prefillsIdeaWhenNoSessionYet(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "prefill_idea_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Prefill Idea Test", "prefill-idea-test-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)

	cfg := config.Config{SessionSecret: "prefill-idea-test-secret", GeminiAPIKey: "test-key-not-called"}
	req := httptest.NewRequest(http.MethodGet, "/new-application?idea=Leave+requests", nil)
	req = req.WithContext(wsCtx)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, "usr_test_actor", 0)})
	rec := httptest.NewRecorder()
	showNewApplication(store, nil, cfg)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `value="Leave requests"`) {
		t.Errorf("body did not prefill the idea into the message input: %s", rec.Body.String())
	}
}

// TestExistingStateFor_reservesOnlyThisWorkspacesIDs is the validation half of Workspace
// isolation, and it deliberately asserts the *opposite* of what this test said earlier on
// 2026-09-27.
//
// Its first version widened the check to every Machine id in the process, which was right while
// Machines were shared: the empty "Dokter Kecil" Workspace naming its own mch_document really did
// overwrite the one "default" had installed, because both resolved to the same file. Isolation
// removed that cause -- a Workspace holds its own Machines and a generated Application is written
// into its own directory -- so the same proposal is now simply legitimate, and refusing it would
// block a Workspace from naming its own Machines after its own business.
func TestExistingStateFor_reservesOnlyThisWorkspacesIDs(t *testing.T) {
	emptyWorkspace := domain.Workspace{
		Slug:       "dokter-kecil",
		MachineIDs: []string{"mch_user", "mch_activity"},
	}

	state := existingStateFor(emptyWorkspace)

	if !state.MachineIDs["mch_user"] {
		t.Error("this Workspace's own machine ids must be reserved")
	}
	// mch_document specifically is not this test's example any more (see
	// TestValidateNewApplication_refusesReservedMachineIDs): it is one of a short list a generated
	// Machine may never take in *any* Workspace, because internal/web/internal/composition hardcode
	// equality checks against it, not because some other Workspace happens to have installed it.
	// This assertion is still exactly the property existingStateFor itself is responsible for,
	// independent of that reserved-id list.
	if state.MachineIDs["mch_pengajuan"] {
		t.Error("mch_pengajuan is installed in another Workspace, which no longer makes it taken here -- that is what isolation means")
	}

	// The shape the real incident's proposal had, minus the one reserved id it happened to pick:
	// this Workspace's own machine, written to metadata/workspaces/dokter-kecil/pengajuan.yaml,
	// touching nobody else's.
	ownDocument := aiassist.GeneratedChange{
		Kind: aiassist.KindNewApplication,
		Application: &aiassist.GeneratedApplication{
			ID: "app_doc_submission", Name: "Pengajuan Dokumen",
			Machines: []aiassist.GeneratedMachine{{
				ID: "mch_pengajuan", Name: "Pengajuan",
				Fields: []aiassist.GeneratedField{{ID: "fld_title", Name: "Judul", Type: "text", Required: true}},
			}},
		},
	}
	if err := aiassist.Validate(ownDocument, state); err != nil {
		t.Errorf("Validate() = %v, want nil -- a Workspace naming its own machine after its own business is legitimate now", err)
	}

	// Its own ids are still reserved, which is the half that does not change: two Machines under
	// one id inside one Workspace would be genuinely ambiguous.
	duplicate := ownDocument
	duplicate.Application = &aiassist.GeneratedApplication{
		ID: "app_people", Name: "People",
		Machines: []aiassist.GeneratedMachine{{
			ID: "mch_user", Name: "User",
			Fields: []aiassist.GeneratedField{{ID: "fld_title", Name: "Title", Type: "text", Required: true}},
		}},
	}
	err := aiassist.Validate(duplicate, state)
	if err == nil {
		t.Fatal("Validate() = nil, want a rejection of an id this Workspace already has")
	}
	if !strings.Contains(err.Error(), "mch_user") {
		t.Errorf("Validate() error = %v, want it to name the colliding machine id", err)
	}
}

// TestExistingStateFor_populatesApplicationsForExtend is the regression test for a bug that
// predates this file's Workspace-isolation work: existingStateFor declared its Applications map
// but never filled it in, so aiassist.Validate's extend_application path -- which looks up
// existing.Applications[change.TargetAppID] -- rejected every extension with "application X is
// not installed in this workspace" even when it plainly was. Found chasing a real conversation
// where the assistant tried exactly this path and could never get past it.
func TestExistingStateFor_populatesApplicationsForExtend(t *testing.T) {
	ws := domain.Workspace{
		Slug:       "dokter-kecil",
		MachineIDs: []string{"mch_user", "mch_document"},
		Machines: []*domain.Machine{
			{ID: "mch_user"},
			{ID: "mch_document", Fields: []domain.Field{
				{ID: "fld_status", Type: domain.FieldTypeStatus, Options: []string{"Draft", "Under Review"}},
			}},
		},
		Applications: []domain.Application{{
			ID: "app_document_tracking", Name: "Document Tracking",
			Roles: []string{"author", "reviewer"}, Machines: []string{"mch_document"},
		}},
	}

	state := existingStateFor(ws)

	target, ok := state.Applications["app_document_tracking"]
	if !ok {
		t.Fatal("existingStateFor() did not populate Applications[app_document_tracking] -- extend_application can never validate against an installed application")
	}
	if len(target.Roles) != 2 || target.Roles[0] != "author" {
		t.Errorf("Applications[app_document_tracking].Roles = %v, want [author reviewer]", target.Roles)
	}
	if _, ok := target.Machines["mch_document"]; !ok {
		t.Error("Applications[app_document_tracking].Machines is missing mch_document, which this Application claims")
	}

	extension := aiassist.GeneratedChange{
		Kind: aiassist.KindExtendApplication, TargetAppID: "app_document_tracking",
		Additions: []aiassist.MetadataAddition{{NewRole: "approver"}},
	}
	if err := aiassist.Validate(extension, state); err != nil {
		t.Errorf("Validate(extend an installed application) = %v, want nil", err)
	}
}

// TestPublishNewApplication_grantsThePublisherTheirChosenRole is the regression test for the other
// half of the same 2026-09-27 incident: publishing a brand-new Application used to grant nobody
// any role in it, including its own creator, so the redirect straight into
// requireApplicationAccess landed on a 403 ("you have no role in <name>"). The conversation now
// asks which of its own declared roles the publisher will hold (GeneratedApplication.
// PublisherRole, validated to be one of Roles); publishing must act on that answer.
func TestPublishNewApplication_grantsThePublisherTheirChosenRole(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "publish_role_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Publish Role Test", "publish-role-test-workspace")
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

	// A minimal real manifest for aiassist.Write to append into -- publishNewApplication reads
	// cfg.MetadataPath/<slug>.yaml directly, so this has to exist on disk, not just in the DB.
	metadataDir := t.TempDir()
	// A real, loadable manifest: aiassist.Write load-verifies what it wrote through the actual
	// metadata loader and rolls back if it does not load, so a fixture referencing a file that
	// does not exist would (correctly) be refused.
	if err := os.WriteFile(filepath.Join(metadataDir, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(metadataDir, ws.Slug+".yaml")
	if err := os.WriteFile(manifestPath, []byte("workspace: "+ws.Slug+"\nmachines:\n  - user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	change := aiassist.GeneratedChange{
		Kind: aiassist.KindNewApplication,
		Application: &aiassist.GeneratedApplication{
			ID: "app_publish_role_test", Name: "Publish Role Test App",
			Roles: []string{"author", "reviewer"}, PublisherRole: "reviewer",
			Machines: []aiassist.GeneratedMachine{{
				ID: "mch_publish_role_doc", Name: "Document",
				Fields: []aiassist.GeneratedField{{ID: "fld_title", Name: "Title", Type: "text", Required: true}},
			}},
		},
	}
	reply := aiassist.Reply{Message: "Here is what I'll build.", Change: &change}
	content, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateAISession(wsCtx, ws.ID, user.ID, aiassist.KindNewApplication)
	if err != nil {
		t.Fatalf("CreateAISession: %v", err)
	}
	if err := store.AppendAISessionTurn(wsCtx, session.ID, "model", string(content)); err != nil {
		t.Fatalf("AppendAISessionTurn: %v", err)
	}
	if err := store.UpdateAISessionStatus(wsCtx, session.ID, data.AISessionStatusGenerated); err != nil {
		t.Fatalf("UpdateAISessionStatus: %v", err)
	}

	cfg := config.Config{SessionSecret: "publish-role-test-secret", MetadataPath: metadataDir}
	testWorkspace := domain.Workspace{Slug: ws.Slug, MachineIDs: []string{domain.UserMachineID}}
	reqCtx := rendering.WithCurrentWorkspace(wsCtx, testWorkspace, "Publish Role Test", false)

	r := chi.NewRouter()
	r.Post("/new-application/{session}/publish", publishNewApplication(store, nil, cfg, func() error { return nil }))
	req := httptest.NewRequest(http.MethodPost, "/new-application/"+session.ID+"/publish", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, user.ID, 0)})
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publish status = %d, want 303; body: %s", rec.Code, rec.Body.String())
	}

	_, direct, _, err := store.ActorMembership(ctx, ws.ID, user.ID)
	if err != nil {
		t.Fatalf("ActorMembership: %v", err)
	}
	if got := direct["app_publish_role_test"]; got != "reviewer" {
		t.Errorf("direct app role = %q, want %q (the conversation's own publisher_role answer)", got, "reviewer")
	}
}

// recordingAIClient is a fake aiassist.Client that captures the history it was handed and returns
// a fixed reply -- enough to assert what a failed publish tells the assistant.
type recordingAIClient struct {
	sawTurns []aiassist.Turn
	reply    aiassist.Reply
}

func (c *recordingAIClient) Generate(_ context.Context, _ string, turns []aiassist.Turn) (aiassist.Reply, error) {
	c.sawTurns = turns
	return c.reply, nil
}

// TestPublishNewApplication_failureReturnsToTheConversation is the owner's own point (2026-09-27):
// a publish that fails is almost always a *fixable* proposal, and the only participant who can fix
// it is the assistant -- the person who clicked Publish never wrote the metadata. Ending on
// http.Error(422, "1 issue(s): ...") made a dead end out of something the conversation was already
// equipped to solve.
//
// The proposal here validates when generated and stops validating before publish, which is the
// real shape of this: the Workspace gained the very Machine id it proposes in between.
func TestPublishNewApplication_failureReturnsToTheConversation(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "publish_failure_test@example.com"

	ws, err := store.CreateWorkspace(ctx, "Publish Failure Test", "publish-failure-test-workspace")
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
			ID: "app_publish_failure", Name: "Publish Failure App",
			Machines: []aiassist.GeneratedMachine{{
				ID: "mch_already_here", Name: "Thing",
				Fields: []aiassist.GeneratedField{{ID: "fld_title", Name: "Title", Type: "text", Required: true}},
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
		t.Fatalf("AppendAISessionTurn: %v", err)
	}
	if err := store.UpdateAISessionStatus(wsCtx, session.ID, data.AISessionStatusGenerated); err != nil {
		t.Fatalf("UpdateAISessionStatus: %v", err)
	}

	// The Workspace now already has the Machine the proposal wants to create, so Validate refuses
	// it at publish time even though it passed when generated.
	occupied := domain.Workspace{Slug: ws.Slug, MachineIDs: []string{"mch_already_here"}}
	cfg := config.Config{SessionSecret: "publish-failure-test-secret", MetadataPath: t.TempDir()}
	ai := &recordingAIClient{reply: aiassist.Reply{Message: "Understood -- renaming it."}}

	r := chi.NewRouter()
	r.Post("/new-application/{session}/publish", publishNewApplication(store, ai, cfg, func() error { return nil }))
	req := httptest.NewRequest(http.MethodPost, "/new-application/"+session.ID+"/publish", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, cfg, user.ID, 0)})
	req = req.WithContext(rendering.WithCurrentWorkspace(wsCtx, occupied, "Publish Failure Test", false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publish status = %d, want 303 back to the conversation; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/new-application?session="+session.ID {
		t.Errorf("Location = %q, want the conversation this proposal came from", got)
	}

	// The assistant must actually have been told, in its own history, what went wrong.
	var reported string
	for _, turn := range ai.sawTurns {
		if turn.Role == "user" && strings.Contains(turn.Text, "mch_already_here") {
			reported = turn.Text
		}
	}
	if reported == "" {
		t.Fatalf("the assistant was not told what failed; it saw %d turns", len(ai.sawTurns))
	}
	if !strings.Contains(reported, "Publishing this failed") {
		t.Errorf("the reported turn does not say publishing failed: %q", reported)
	}

	// And the session is open again, so the broken proposal is no longer offered as a draft.
	after, err := store.GetAISession(wsCtx, ws.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != data.AISessionStatusOpen {
		t.Errorf("session status = %q, want %q -- the failed proposal must stop being offerable", after.Status, data.AISessionStatusOpen)
	}
}
