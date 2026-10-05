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
