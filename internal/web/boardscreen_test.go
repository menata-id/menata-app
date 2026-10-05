package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/authorization"
)

// The board's own writes and its sibling View, through the real router against the real default
// manifest: a card composed inside a column lands in that column's List, and the response is the
// board again (the View is recovered from HX-Current-URL), not the generic Machine page.
func TestBoardComposerCreatesACardInItsColumn(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "boardcomposer", "default")
	list, err := store.CreateRecord(ctx, "mch_list", map[string]any{"fld_name": "Shooting"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRecord(ctx, "mch_task", map[string]any{"fld_title": "Seeded", "fld_list": list.ID, "fld_assignee": actorID}); err != nil {
		t.Fatal(err)
	}
	session := &http.Cookie{Name: authorization.SessionCookieName, Value: cookie}

	tok := csrfTokenFor(t, h, "/machines/mch_task/records")
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("fld_title", "Composer card")
	mw.WriteField("fld_list", list.ID)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/machines/mch_task/records", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", tok.value)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://x/machines/mch_task")
	req.AddCookie(tok.cookie)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("composer POST = %d\n%s", rec.Code, out)
	}
	for _, want := range []string{"Composer card", "2 cards · grouped by List", `aria-label="Shooting"`} {
		if !strings.Contains(out, want) {
			t.Errorf("response after composing is missing %q", want)
		}
	}

	get := httptest.NewRequest(http.MethodGet, "/machines/mch_task?view=vw_task_table", nil)
	get.AddCookie(session)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, get)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<table") || strings.Contains(rec.Body.String(), "grouped by") {
		t.Errorf("Table view = %d, want a 200 table with no board summary", rec.Code)
	}
}

// A card's labels come out of its own `card_tags:` declaration, through the real router: the chip carries
// the label's name and palette colour and sits above its card's title; the untagged card draws nothing.
func TestBoardDrawsTagChipsFromCardTags(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "boardtags", "default")
	list, err := store.CreateRecord(ctx, "mch_list", map[string]any{"fld_name": "Shooting"})
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := store.CreateRecord(ctx, "mch_task", map[string]any{"fld_title": "Tagged card", "fld_list": list.ID, "fld_assignee": actorID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRecord(ctx, "mch_task", map[string]any{"fld_title": "Plain card", "fld_list": list.ID, "fld_assignee": actorID}); err != nil {
		t.Fatal(err)
	}
	label, err := store.CreateRecord(ctx, "mch_label", map[string]any{"fld_name": "Urgent-tag", "fld_color": "rose"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRecord(ctx, "mch_card_label", map[string]any{"fld_task": tagged.ID, "fld_label": label.ID}); err != nil {
		t.Fatal(err)
	}

	get := httptest.NewRequest(http.MethodGet, "/machines/mch_task", nil)
	get.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, get)
	out := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("board = %d\n%s", rec.Code, out)
	}
	if got := strings.Count(out, "Urgent-tag"); got != 1 {
		t.Errorf("label drawn %d times, want once (on the tagged card only)", got)
	}
	if !strings.Contains(out, "text-rose-700") {
		t.Errorf("chip is not drawn in the label's palette colour")
	}
	if strings.Index(out, "Urgent-tag") > strings.Index(out, "Tagged card") {
		t.Errorf("the chip belongs above its card's title")
	}
}

func patchCard(t *testing.T, h http.Handler, cookie, id string, fields map[string]string, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	tok := csrfTokenFor(t, h, "/machines/mch_task/records")
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPatch, "/machines/mch_task/records/"+id, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if csrf {
		req.Header.Set("X-CSRF-Token", tok.value)
	}
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://x/machines/mch_task")
	req.AddCookie(tok.cookie)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// A card's circle and quick edit go through PATCH, which changes the Fields it names and keeps every other
// one -- the thing the whole-form PUT cannot do -- and answers with the board.
func TestBoardCardPatchChangesOnlyWhatItNames(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "boardpatch", "default")
	list, err := store.CreateRecord(ctx, "mch_list", map[string]any{"fld_name": "Shooting"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := store.CreateRecord(ctx, "mch_task", map[string]any{"fld_title": "Draft", "fld_list": list.ID, "fld_assignee": actorID, "fld_status": "todo", "fld_due_date": "2026-10-12"})
	if err != nil {
		t.Fatal(err)
	}

	rec := patchCard(t, h, cookie, card.ID, map[string]string{"fld_status": "done"}, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `aria-pressed="true"`) {
		t.Fatalf("completing = %d, want the board with a finished card\n%s", rec.Code, rec.Body.String())
	}
	got, err := store.GetRecord(ctx, "mch_task", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{"fld_status": "done", "fld_title": "Draft", "fld_list": list.ID, "fld_assignee": actorID, "fld_due_date": "2026-10-12"} {
		if got.Values[k] != want {
			t.Errorf("after completing, %s = %v, want %v", k, got.Values[k], want)
		}
	}

	rec = patchCard(t, h, cookie, card.ID, map[string]string{"fld_title": "Renamed"}, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Renamed") {
		t.Fatalf("renaming = %d\n%s", rec.Code, rec.Body.String())
	}
	if got, _ = store.GetRecord(ctx, "mch_task", card.ID); got.Values["fld_title"] != "Renamed" || got.Values["fld_status"] != "done" {
		t.Errorf("rename lost another Field: %+v", got.Values)
	}

	if rec := patchCard(t, h, cookie, card.ID, map[string]string{"nonsense": "x"}, true); rec.Code != http.StatusBadRequest {
		t.Errorf("a PATCH naming no Field of the Machine = %d, want 400", rec.Code)
	}
	tok := csrfTokenFor(t, h, "/machines/mch_task/records")
	anon := httptest.NewRequest(http.MethodPatch, "/machines/mch_task/records/"+card.ID, strings.NewReader("fld_status=done"))
	anon.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	anon.Header.Set("X-CSRF-Token", tok.value)
	anon.AddCookie(tok.cookie)
	anonRec := httptest.NewRecorder()
	h.ServeHTTP(anonRec, anon)
	if anonRec.Code == http.StatusOK {
		t.Errorf("a PATCH with no session = %d, want a refusal", anonRec.Code)
	}
	if rec := patchCard(t, h, cookie, card.ID, map[string]string{"fld_status": "done"}, false); rec.Code != http.StatusForbidden {
		t.Errorf("a PATCH without a CSRF token = %d, want 403", rec.Code)
	}
}

// Moving a card is one PATCH: the group Field and a 1-based `position` among that list's cards. Both the drag
// script and the Move panel send exactly this, so the order a drop produces is asserted here, against the
// real router and the stored order, not against what the board happens to draw afterwards.
func TestBoardCardMoveWritesListAndPosition(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "boardmove", "default")
	mk := func(name string) string {
		r, err := store.CreateRecord(ctx, "mch_list", map[string]any{"fld_name": name})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	backlog, shooting := mk("Backlog"), mk("Shooting")
	card := func(title, list string) string {
		r, err := store.CreateRecord(ctx, "mch_task", map[string]any{"fld_title": title, "fld_list": list, "fld_assignee": actorID, "fld_status": "todo"})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	a, b := card("A", backlog), card("B", backlog)
	_, d := card("C", shooting), card("D", shooting)
	order := func(list string) string {
		rs, err := store.ListRecords(ctx, "mch_task")
		if err != nil {
			t.Fatal(err)
		}
		var s string
		for _, r := range rs {
			if r.Values["fld_list"] == list {
				s += r.Values["fld_title"].(string)
			}
		}
		return s
	}
	move := func(id, list, position string) *httptest.ResponseRecorder {
		return patchCard(t, h, cookie, id, map[string]string{"fld_list": list, "position": position}, true)
	}

	if rec := move(b, shooting, "2"); rec.Code != http.StatusOK {
		t.Fatalf("move = %d\n%s", rec.Code, rec.Body.String())
	}
	if got := order(shooting); got != "CBD" {
		t.Errorf("B dropped at position 2 of Shooting: %s, want CBD", got)
	}
	if got := order(backlog); got != "A" {
		t.Errorf("Backlog after B left: %s, want A", got)
	}
	move(a, shooting, "1")
	if got := order(shooting); got != "ACBD" {
		t.Errorf("A at position 1: %s, want ACBD", got)
	}
	move(a, shooting, "99")
	if got := order(shooting); got != "CBDA" {
		t.Errorf("A at position 99 (past the end): %s, want CBDA", got)
	}
	move(d, shooting, "1")
	if got := order(shooting); got != "DCBA" {
		t.Errorf("D reordered inside its own list: %s, want DCBA", got)
	}

	if rec := move(d, shooting, "second"); rec.Code != http.StatusBadRequest {
		t.Errorf("a non-numeric position = %d, want 400", rec.Code)
	}
	if rec := move(d, backlog, "0"); rec.Code != http.StatusBadRequest {
		t.Errorf("position 0 = %d, want 400", rec.Code)
	}
	if got, _ := store.GetRecord(ctx, "mch_task", d); got.Values["fld_list"] != shooting || got.Values["position"] != nil {
		t.Errorf("position leaked into the record's values: %+v", got.Values)
	}
}
