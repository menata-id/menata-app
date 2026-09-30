package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// validGeneratedChange is a proposal that **passes aiassist.Validate** -- mirroring
// internal/aiassist's own validLeaveRequestChange, since that one is unexported.
//
// It was not valid until 2026-09-29, and that mattered: the change this fixture built had no Machines,
// which Validate rejects ("an application needs at least one machine"). Every test using the fixture
// still passed, because they read the session's *status* -- which the fixture sets directly -- and none
// of them validated. So the review screen's own re-validation, the thing its doc comment says it exists
// for, had never been exercised by anything. Found by the first test that called it.
func validGeneratedChange(name string) *aiassist.GeneratedChange {
	return &aiassist.GeneratedChange{
		Kind: aiassist.KindNewApplication,
		Application: &aiassist.GeneratedApplication{
			ID: "app_leave_permits", Name: name, Description: "Generated from your description.",
			Roles: []string{"Employee", "Supervisor"}, PublisherRole: "Supervisor",
			Navigation: []aiassist.GeneratedMenuItem{{Label: "Leave Permits", MachineID: "mch_leave_permit"}},
			Machines: []aiassist.GeneratedMachine{{
				ID: "mch_leave_permit", Name: "Leave Permit",
				Fields: []aiassist.GeneratedField{
					{ID: "fld_title", Name: "Title", Type: "text", Required: true},
					{ID: "fld_status", Name: "Status", Type: "status", Required: true, Options: []string{"submitted", "approved"}},
				},
				Permissions: []aiassist.GeneratedPermission{
					{ID: "prm_create", Action: "create", Roles: []string{"Employee"}},
				},
				Transitions: []aiassist.GeneratedTransition{
					{ID: "trn_approve", Name: "Approve", Field: "fld_status", From: "submitted", To: "approved"},
				},
			}},
		},
	}
}

// createGeneratedSession is the fixture every draft-Application test below needs: a session that
// has reached "generated" status, with a stored model turn carrying the identical shape
// latestChange (newapplication.go) decodes -- no Gemini call involved, matching production.
func createGeneratedSession(t *testing.T, ctx context.Context, store *data.Store, workspaceID, name string) *data.AISession {
	t.Helper()
	session, err := store.CreateAISession(ctx, workspaceID, "usr_test_actor", aiassist.KindNewApplication)
	if err != nil {
		t.Fatalf("CreateAISession: %v", err)
	}
	reply := aiassist.Reply{Message: "Here is what I'll build.", Change: validGeneratedChange(name)}
	content, err := json.Marshal(reply)
	if err != nil {
		t.Fatalf("marshal reply: %v", err)
	}
	if err := store.AppendAISessionTurn(ctx, session.ID, "model", string(content)); err != nil {
		t.Fatalf("AppendAISessionTurn: %v", err)
	}
	if err := store.UpdateAISessionStatus(ctx, workspaceID, session.ID, data.AISessionStatusGenerated); err != nil {
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
	if err := store.UpdateAISessionStatus(wsCtx, ws.ID, published.ID, data.AISessionStatusPublished); err != nil {
		t.Fatalf("UpdateAISessionStatus(published): %v", err)
	}
	discarded := createGeneratedSession(t, wsCtx, store, ws.ID, "Discarded Idea")
	if err := store.UpdateAISessionStatus(wsCtx, ws.ID, discarded.ID, data.AISessionStatusDiscarded); err != nil {
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
			Navigation: []aiassist.GeneratedMenuItem{{Label: "Pengajuan", MachineID: "mch_pengajuan"}},
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
			Navigation: []aiassist.GeneratedMenuItem{{Label: "Documents", MachineID: "mch_publish_role_doc"}},
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
	if err := store.UpdateAISessionStatus(wsCtx, ws.ID, session.ID, data.AISessionStatusGenerated); err != nil {
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
	if err := store.UpdateAISessionStatus(wsCtx, ws.ID, session.ID, data.AISessionStatusGenerated); err != nil {
		t.Fatalf("UpdateAISessionStatus: %v", err)
	}

	// The Workspace now already has the Machine the proposal wants to create, so Validate refuses
	// it at publish time even though it passed when generated. On disk, because publish reads the
	// installation from the manifest rather than from ctx.
	metadataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(metadataDir, "already_here.yaml"),
		[]byte("id: mch_already_here\nname: Thing\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadataDir, ws.Slug+".yaml"),
		[]byte("workspace: "+ws.Slug+"\nmachines:\n  - already_here.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	occupied := domain.Workspace{Slug: ws.Slug, MachineIDs: []string{"mch_already_here"}}
	cfg := config.Config{SessionSecret: "publish-failure-test-secret", MetadataPath: metadataDir}
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

// TestDiscardNewApplication_cannotReachAnotherWorkspacesSession is the defect this slice exists for,
// and it was written before the fix so that it could be watched failing.
//
// Four of the five AI-session handlers resolve the session through GetAISession(ctx, workspaceID, id)
// before touching it. discardNewApplication took the id straight from the URL and handed it to
// UpdateAISessionStatus, whose own statement was `WHERE id = $1` with no Workspace predicate -- so a
// Workspace admin could discard *another* Workspace's draft Application by id. The ids are random,
// which is not scoping: this repo has rejected "hard to guess" as a guard before, and Workspace
// scoping is the one invariant the whole data layer is built on.
func TestDiscardNewApplication_cannotReachAnotherWorkspacesSession(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const mineEmail, theirsEmail = "discard_mine@example.com", "discard_theirs@example.com"

	mine, err := store.CreateWorkspace(ctx, "Discard Mine", "discard-mine-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace(mine): %v", err)
	}
	cleanupAuthTest(t, pool, mine.ID, mineEmail)
	theirs, err := store.CreateWorkspace(ctx, "Discard Theirs", "discard-theirs-workspace")
	if err != nil {
		t.Fatalf("CreateWorkspace(theirs): %v", err)
	}
	cleanupAuthTest(t, pool, theirs.ID, theirsEmail)

	// The victim: a generated draft belonging to the *other* Workspace.
	victim := createGeneratedSession(t, data.WithWorkspaceScope(ctx, theirs.ID), store, theirs.ID, "Their Draft")

	// The attacker acts inside their own Workspace, as its admin, naming the other Workspace's id.
	req := httptest.NewRequest(http.MethodPost, "/new-application/"+victim.ID+"/discard", nil)
	req = req.WithContext(data.WithWorkspaceScope(ctx, mine.ID))
	r := chi.NewRouter()
	r.Post("/new-application/{session}/discard", discardNewApplication(store))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusSeeOther || rec.Code == http.StatusFound {
		t.Errorf("discarding another Workspace's session returned %d -- it must be refused, not redirected", rec.Code)
	}

	// The assertion that matters is the state, not the status code: a handler that answered 404 after
	// writing would pass the line above.
	after, err := store.GetAISession(data.WithWorkspaceScope(ctx, theirs.ID), theirs.ID, victim.ID)
	if err != nil {
		t.Fatalf("GetAISession(victim): %v", err)
	}
	if after.Status != data.AISessionStatusGenerated {
		t.Errorf("the other Workspace's session is now %q, want it untouched at %q -- a cross-Workspace write landed",
			after.Status, data.AISessionStatusGenerated)
	}
}

// fakeAIClient is the whole of what these tests need of aiassist.Client -- a one-method interface, so a
// stub rather than a mock library. reply is returned verbatim; err, when set, is what the assistant
// turn has to survive.
type fakeAIClient struct {
	reply  aiassist.Reply
	err    error
	prompt string
	turns  int
}

func (f *fakeAIClient) Generate(_ context.Context, systemPrompt string, turns []aiassist.Turn) (aiassist.Reply, error) {
	f.prompt, f.turns = systemPrompt, len(turns)
	return f.reply, f.err
}

// TestPostNewApplicationMessage_storesBothTurnsAndReachesGenerated is the conversation's own write
// path: the person's message and the assistant's reply are both appended, and a reply carrying a valid
// change moves the session to "generated" -- which is what puts it on Workspace Home as a draft.
func TestPostNewApplicationMessage_storesBothTurnsAndReachesGenerated(t *testing.T) {
	s := newAssistantRouteSetup(t, "Assistant Message", "assistant-message-workspace", "assistant_message@example.com")

	client := &fakeAIClient{reply: aiassist.Reply{
		Message: "Here is what I'll build.",
		Change:  validGeneratedChange("Leave Requests"),
	}}

	rec := s.postMessage(t, client, "", "I need leave requests with an approval")
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("post message = %d, want a redirect; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	if client.turns == 0 {
		t.Error("the client was called with no turns -- the person's own message never reached it")
	}

	session := s.sessionFromRedirect(t, rec)
	if len(session.Turns) != 2 {
		t.Fatalf("session holds %d turn(s), want 2 (the person's and the assistant's)", len(session.Turns))
	}
	if session.Turns[0].Role != "user" || !strings.Contains(session.Turns[0].Content, "leave requests") {
		t.Errorf("first turn = %+v, want the person's own message", session.Turns[0])
	}
	if session.Turns[1].Role != "model" {
		t.Errorf("second turn role = %q, want model", session.Turns[1].Role)
	}
	if session.Status != data.AISessionStatusGenerated {
		t.Errorf("status = %q, want generated -- a valid proposed change is what makes it a draft on Home", session.Status)
	}
}

// TestPostNewApplicationMessage_aClientFailureKeepsTheConversation is the arm that matters when the
// model is down: the person's message must not be lost, and they must be told to retry inside the
// conversation rather than shown an error page (runAssistantTurn's own doc comment).
func TestPostNewApplicationMessage_aClientFailureKeepsTheConversation(t *testing.T) {
	s := newAssistantRouteSetup(t, "Assistant Down", "assistant-down-workspace", "assistant_down@example.com")

	rec := s.postMessage(t, &fakeAIClient{err: context.DeadlineExceeded}, "", "I need leave requests")
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("post message with a failing client = %d, want a redirect back into the conversation; body=%s",
			rec.Code, firstChars(rec.Body.String()))
	}

	session := s.sessionFromRedirect(t, rec)
	if len(session.Turns) != 2 {
		t.Fatalf("session holds %d turn(s), want 2 -- the person's message must survive a model failure", len(session.Turns))
	}
	if !strings.Contains(session.Turns[1].Content, "again") {
		t.Errorf("the assistant's stored turn is %q, want a retry message", session.Turns[1].Content)
	}
	if session.Status == data.AISessionStatusGenerated {
		t.Error("status reached generated on a failed turn -- there is no proposal to review")
	}
}

// TestShowNewApplicationReview_rendersTheProposalAndRefusesAnotherWorkspaces covers the review screen's
// two halves: the proposal it is for, and the scoping every one of these handlers depends on.
func TestShowNewApplicationReview_rendersTheProposalAndRefusesAnotherWorkspaces(t *testing.T) {
	s := newAssistantRouteSetup(t, "Assistant Review", "assistant-review-workspace", "assistant_review@example.com")
	session := createGeneratedSession(t, s.wsCtx, s.store, s.workspaceID, "Leave Requests")

	rec := s.getReview(t, session.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("review = %d, want 200; body=%s", rec.Code, firstChars(rec.Body.String()))
	}
	if body := rec.Body.String(); !strings.Contains(body, "Leave Requests") {
		t.Errorf("the review screen does not name the proposed Application")
	}

	// A session belonging to somebody else's Workspace is not found here, the same property the discard
	// route was missing.
	other, err := s.store.CreateWorkspace(context.Background(), "Review Theirs", "assistant-review-theirs")
	if err != nil {
		t.Fatalf("CreateWorkspace(other): %v", err)
	}
	cleanupAuthTest(t, s.pool, other.ID, "assistant_review_theirs@example.com")
	theirs := createGeneratedSession(t, data.WithWorkspaceScope(context.Background(), other.ID), s.store, other.ID, "Their Draft")

	if rec := s.getReview(t, theirs.ID); rec.Code != http.StatusNotFound {
		t.Errorf("reviewing another Workspace's session = %d, want 404", rec.Code)
	}
}

// --- assistant route fixture -----------------------------------------------------------------------

type assistantRouteSetup struct {
	pool        *pgxpool.Pool
	store       *data.Store
	cfg         config.Config
	wsCtx       context.Context
	workspaceID string
}

func newAssistantRouteSetup(t *testing.T, name, slug, email string) *assistantRouteSetup {
	t.Helper()
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, name, slug)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	_, installed := loadRealMachines(t)
	wsCtx := rendering.WithCurrentWorkspace(data.WithWorkspaceScope(ctx, ws.ID), installed, name, false)
	return &assistantRouteSetup{pool: pool, store: store,
		cfg: config.Config{SessionSecret: slug + "-secret", MetadataPath: t.TempDir(), TemplatePath: realLibrary(t)}, wsCtx: wsCtx, workspaceID: ws.ID}
}

func (s *assistantRouteSetup) postMessage(t *testing.T, client aiassist.Client, sessionID, message string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"message": {message}}
	if sessionID != "" {
		form.Set("session", sessionID)
	}
	req := httptest.NewRequest(http.MethodPost, "/new-application/message", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(s.wsCtx)
	r := chi.NewRouter()
	r.Post("/new-application/message", postNewApplicationMessage(s.store, client, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func (s *assistantRouteSetup) getReview(t *testing.T, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/new-application/"+sessionID+"/review", nil)
	req = req.WithContext(s.wsCtx)
	r := chi.NewRouter()
	r.Get("/new-application/{session}/review", showNewApplicationReview(s.store, s.cfg))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// sessionFromRedirect reads the session back through the id the handler itself redirected to
// (/new-application?session=...), rather than searching the store for whatever is there. That is the
// stronger direction: it asserts the handler told the browser about the same session it wrote, which a
// store lookup would quietly paper over.
func (s *assistantRouteSetup) sessionFromRedirect(t *testing.T, rec *httptest.ResponseRecorder) *data.AISession {
	t.Helper()
	location := rec.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect %q: %v", location, err)
	}
	id := parsed.Query().Get("session")
	if id == "" {
		t.Fatalf("the redirect %q names no session -- the caller cannot get back to their conversation", location)
	}
	full, err := s.store.GetAISession(s.wsCtx, s.workspaceID, id)
	if err != nil {
		t.Fatalf("GetAISession(%s): %v", id, err)
	}
	return full
}

// sequenceAIClient answers each call with the next reply in order, and remembers every history it saw.
type sequenceAIClient struct {
	replies []aiassist.Reply
	seen    [][]aiassist.Turn
}

func (c *sequenceAIClient) Generate(_ context.Context, _ string, turns []aiassist.Turn) (aiassist.Reply, error) {
	c.seen = append(c.seen, turns)
	r := c.replies[0]
	if len(c.replies) > 1 {
		c.replies = c.replies[1:]
	}
	return r, nil
}

// TestPostNewApplicationMessage_anInvalidChangeGetsOneCorrectionRound is the 2026-09-30 conversation:
// the model twice said its change was ready while sending an extend_application with no additions,
// no Review button appeared, and nobody told the model why. Now it is told once and answers again.
func TestPostNewApplicationMessage_anInvalidChangeGetsOneCorrectionRound(t *testing.T) {
	s := newAssistantRouteSetup(t, "Assistant Correction", "assistant-correction-workspace", "assistant_correction@example.com")
	empty := &aiassist.GeneratedChange{Kind: aiassist.KindExtendApplication, TargetAppID: "app_nope"}

	t.Run("corrected on the second answer", func(t *testing.T) {
		client := &sequenceAIClient{replies: []aiassist.Reply{
			{Message: "Ready.", Change: empty},
			{Message: "Fixed.", Change: validGeneratedChange("Leave Requests")},
		}}
		rec := s.postMessage(t, client, "", "build it")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("post = %d; body=%s", rec.Code, firstChars(rec.Body.String()))
		}
		if len(client.seen) != 2 {
			t.Fatalf("the model was called %d times, want 2", len(client.seen))
		}
		last := client.seen[1][len(client.seen[1])-1]
		if last.Role != "user" || !strings.Contains(last.Text, "Automatic check") || !strings.Contains(last.Text, "not installed") {
			t.Errorf("the model was not told what failed: %+v", last)
		}
		session := s.latestSession(t)
		if session.Status != data.AISessionStatusGenerated {
			t.Errorf("status = %q, want generated once the corrected change validates", session.Status)
		}
	})

	t.Run("one round only", func(t *testing.T) {
		client := &sequenceAIClient{replies: []aiassist.Reply{{Message: "Ready.", Change: empty}}}
		if rec := s.postMessage(t, client, "", "build it"); rec.Code != http.StatusSeeOther {
			t.Fatalf("post = %d", rec.Code)
		}
		if len(client.seen) != 2 {
			t.Errorf("the model was called %d times, want exactly 2", len(client.seen))
		}
		if got := s.latestSession(t).Status; got == data.AISessionStatusGenerated {
			t.Error("an invalid change reached generated")
		}
	})
}

func (s *assistantRouteSetup) latestSession(t *testing.T) *data.AISession {
	t.Helper()
	var id string
	if err := s.pool.QueryRow(context.Background(), `SELECT id FROM ai_sessions WHERE workspace_id = $1 ORDER BY created_at DESC LIMIT 1`, s.workspaceID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	session, err := s.store.GetAISession(s.wsCtx, s.workspaceID, id)
	if err != nil {
		t.Fatal(err)
	}
	return session
}
