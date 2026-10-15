package web

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
)

// TestPostNewApplicationMessage_everyNamedGapIsRecordedAndLogged is roadmap stage 1, WG. The fixtures
// are the four request kinds the 2026-10-15 conversation review found leaking (notification, a board
// view, multi-level approval, a per-branch percentage rollup). The reply carries a valid change
// AND several gaps: before WG the second was impossible (one gap, and the prompt called it mutually
// exclusive with a change), which is how the table stopped growing on 2026-09-30.
func TestPostNewApplicationMessage_everyNamedGapIsRecordedAndLogged(t *testing.T) {
	s := newAssistantRouteSetup(t, "Assistant Gaps", "assistant-gaps-workspace", "assistant_gaps@example.com")

	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	client := &fakeAIClient{reply: aiassist.Reply{
		Message: "Aplikasinya saya buat; empat hal belum bisa.",
		Change:  validGeneratedChange("Desain Request"),
		CapabilityGaps: []aiassist.CapabilityGap{
			{Requested: "notifikasi ke pemegang peran", Kind: aiassist.GapAssistant, Note: "email semua koordinator"},
			{Requested: "tampilan papan per status", Kind: aiassist.GapAssistant, Note: "kanban"},
			{Requested: "persetujuan bertingkat generik", Kind: aiassist.GapRuntime, Note: "atasan lalu direktur"},
			{Requested: "rekap persen per cabang", Kind: "bukan-jenis-yang-sah", Note: "model salah menamai jenis"},
		},
	}}
	rec := s.postMessage(t, client, "", "buat desain request dengan notifikasi dan papan")
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("post message = %d; body=%s", rec.Code, firstChars(rec.Body.String()))
	}

	rows, err := s.pool.Query(s.wsCtx, `SELECT requested_capability, kind FROM ai_capability_gaps WHERE workspace_id = $1`, s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var req, kind string
		if err := rows.Scan(&req, &kind); err != nil {
			t.Fatal(err)
		}
		got[req] = kind
	}
	want := map[string]string{
		"notifikasi ke pemegang peran":   "assistant",
		"tampilan papan per status":      "assistant",
		"persetujuan bertingkat generik": "runtime",
		"rekap persen per cabang":        "unclassified", // a kind the model got wrong must not lose the gap
	}
	for req, kind := range want {
		if got[req] != kind {
			t.Errorf("gap %q recorded with kind %q, want %q (all rows: %v)", req, got[req], kind, got)
		}
	}
	for req, kind := range want {
		line := `AI-GAP kind=` + kind + ` `
		if !strings.Contains(logged.String(), line) || !strings.Contains(logged.String(), `requested=`+strconv.Quote(req)) {
			t.Errorf("no AI-GAP log line for %q (kind %s) in:\n%s", req, kind, logged.String())
		}
	}
	// The conversation shown to the person names them all, not only the first.
	session := s.sessionFromRedirect(t, rec)
	view, err := buildConversationView(session)
	if err != nil {
		t.Fatal(err)
	}
	if shown := view.Turns[len(view.Turns)-1].CapabilityGap; !strings.Contains(shown, "tampilan papan per status") || !strings.Contains(shown, "rekap persen per cabang") {
		t.Errorf("the conversation names %q, want every gap", shown)
	}
}

// TestPostNewApplicationMessage_aRuntimeRefusalIsLogged: metadata the assistant produced that the
// runtime refuses is evidence of a gap in capability, guide or grammar whether or not the model says
// so. A refusal the model then fixes is logged "recovered" and one it does not "unresolved", so the
// reader can tell a prompt problem from a grammar or capability problem.
func TestPostNewApplicationMessage_aRuntimeRefusalIsLogged(t *testing.T) {
	s := newAssistantRouteSetup(t, "Assistant Reject", "assistant-reject-workspace", "assistant_reject@example.com")
	bad := &aiassist.GeneratedChange{Kind: aiassist.KindUpdateApplication, TargetAppID: "app_nope"}

	capture := func(t *testing.T, client aiassist.Client) string {
		t.Helper()
		var logged bytes.Buffer
		log.SetOutput(&logged)
		defer log.SetOutput(os.Stderr)
		if rec := s.postMessage(t, client, "", "build it"); rec.Code != http.StatusSeeOther {
			t.Fatalf("post = %d", rec.Code)
		}
		return logged.String()
	}

	t.Run("recovered", func(t *testing.T) {
		out := capture(t, &sequenceAIClient{replies: []aiassist.Reply{
			{Message: "Ready.", Change: bad},
			{Message: "Fixed.", Change: validGeneratedChange("Leave Requests")},
		}})
		for _, want := range []string{"AI-REJECT stage=check final=pending attempt=1", "not installed", "AI-REJECT stage=check final=recovered attempt=2"} {
			if !strings.Contains(out, want) {
				t.Errorf("log lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "unresolved") {
			t.Errorf("a recovered refusal was logged unresolved:\n%s", out)
		}
	})
	t.Run("unresolved", func(t *testing.T) {
		out := capture(t, &sequenceAIClient{replies: []aiassist.Reply{{Message: "Ready.", Change: bad}}})
		for _, want := range []string{"final=pending attempt=1", "final=unresolved attempt=2"} {
			if !strings.Contains(out, want) {
				t.Errorf("log lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "recovered") {
			t.Errorf("an unresolved refusal was logged recovered:\n%s", out)
		}
	})
}
