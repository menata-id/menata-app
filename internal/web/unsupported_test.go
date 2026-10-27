package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/data"
)

func modelTurn(t *testing.T, reply aiassist.Reply) data.AISessionTurn {
	t.Helper()
	b, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	return data.AISessionTurn{Role: "model", Content: string(b)}
}

// What the conversation said it cannot build is listed once each, in the order it was named, and only from the assistant's replies.
func TestUnsupportedOfListsEachGapOnceInOrder(t *testing.T) {
	session := &data.AISession{Turns: []data.AISessionTurn{
		{Role: "user", Content: "Staff request leave"},
		modelTurn(t, aiassist.Reply{Message: "ok", CapabilityGaps: []aiassist.CapabilityGap{{Requested: "Half-day leave"}, {Requested: "Quota refilled each 1 January"}}}),
		{Role: "user", Content: "also half-day"},
		modelTurn(t, aiassist.Reply{Message: "still", CapabilityGaps: []aiassist.CapabilityGap{{Requested: "  half-day LEAVE "}, {Requested: "Email reminders"}}}),
		modelTurn(t, aiassist.Reply{Message: "nothing missing"}),
	}}
	want := []string{"Half-day leave", "Quota refilled each 1 January", "Email reminders"}
	if got := unsupportedOf(session); !slices.Equal(got, want) {
		t.Errorf("unsupportedOf = %q, want %q", got, want)
	}
	if got := unsupportedOf(&data.AISession{}); len(got) != 0 {
		t.Errorf("an empty conversation lists %q", got)
	}
}

// The review says what the application cannot do before the person publishes it, from the conversation's own replies.
func TestReviewNamesWhatCannotBeBuiltYet(t *testing.T) {
	s := newAssistantRouteSetup(t, "Unsupported Review Test", "unsupported-review-test-workspace", "unsupported_review_test@example.com")
	session, err := s.store.CreateAISession(s.wsCtx, s.workspaceID, "usr_test_actor", aiassist.KindNewApplication)
	if err != nil {
		t.Fatal(err)
	}
	reply := aiassist.Reply{Message: "Here is what I'll build.", Change: validGeneratedChange("Leave Requests"),
		CapabilityGaps: []aiassist.CapabilityGap{{Requested: "Half-day leave", Note: "days only"}}}
	turn := modelTurn(t, reply)
	if err := s.store.AppendAISessionTurn(s.wsCtx, session.ID, turn.Role, turn.Content); err != nil {
		t.Fatal(err)
	}
	if err := s.store.UpdateAISessionStatus(s.wsCtx, s.workspaceID, session.ID, data.AISessionStatusGenerated); err != nil {
		t.Fatal(err)
	}
	// The conversation's own panel says the same, from the same replies.
	cfg := s.cfg
	cfg.GeminiAPIKey = "test-key" // the page is served only where the assistant is configured; nothing here calls it
	r := chi.NewRouter()
	r.Get("/new-application", showNewApplication(s.store, nil, cfg))
	conv := httptest.NewRecorder()
	r.ServeHTTP(conv, httptest.NewRequest(http.MethodGet, "/new-application?session="+session.ID, nil).WithContext(s.wsCtx))
	if conv.Code != http.StatusOK || !strings.Contains(conv.Body.String(), "Half-day leave") || !strings.Contains(conv.Body.String(), "What it can&#39;t do yet") {
		t.Errorf("conversation = %d; lacks the unsupported block", conv.Code)
	}
	rec := s.getReview(t, session.ID)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "What it can&#39;t do yet") || !strings.Contains(body, "Half-day leave") {
		t.Errorf("review = %d; lacks the unsupported block:\n%.600s", rec.Code, body)
	}
}
