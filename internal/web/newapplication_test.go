package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
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
	showNewApplication(nil, store, nil, cfg)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `value="Leave requests"`) {
		t.Errorf("body did not prefill the idea into the message input: %s", rec.Body.String())
	}
}
