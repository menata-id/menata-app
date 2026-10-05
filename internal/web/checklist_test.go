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

func checklistWrite(t *testing.T, h http.Handler, cookie, method, path string, fields map[string]string, currentURL string) *httptest.ResponseRecorder {
	t.Helper()
	tok := csrfTokenFor(t, h, "/machines/mch_task/records")
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", tok.value)
	req.Header.Set("HX-Request", "true")
	if currentURL != "" {
		req.Header.Set("HX-Current-URL", currentURL)
	}
	req.AddCookie(tok.cookie)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// A Task's checklist is its child Machine's records drawn as a to-do list: the add form POSTs the child's own
// create route from the Task's page (so it is answered with HX-Refresh, there being no Machine body to swap in),
// and the circle is the completion PATCH. The count on the page follows both.
func TestTaskDetailChecklistAddsAndCompletesItems(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "checklist", "default")
	task, err := store.CreateRecord(ctx, "mch_task", map[string]any{"fld_title": "Call sheet", "fld_assignee": actorID, "fld_status": "todo"})
	if err != nil {
		t.Fatal(err)
	}
	page := "http://x/machines/mch_task/records/" + task.ID
	detail := func() string {
		req := httptest.NewRequest(http.MethodGet, "/machines/mch_task/records/"+task.ID, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("detail = %d\n%s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	if body := detail(); !strings.Contains(body, "Add an item") || !strings.Contains(body, "0 of 0") {
		t.Fatalf("an empty checklist should still offer the add input and say 0 of 0")
	}
	for _, text := range []string{"Confirm clinic", "Lock vendor"} {
		rec := checklistWrite(t, h, cookie, http.MethodPost, "/machines/mch_checklist_item/records", map[string]string{"fld_task": task.ID, "fld_text": text}, page)
		if rec.Code != http.StatusOK || rec.Header().Get("HX-Refresh") != "true" {
			t.Fatalf("add item from the Task page = %d, HX-Refresh %q", rec.Code, rec.Header().Get("HX-Refresh"))
		}
	}
	body := detail()
	if !strings.Contains(body, "Confirm clinic") || !strings.Contains(body, "Lock vendor") || !strings.Contains(body, "0 of 2") {
		t.Fatalf("both items should be listed with 0 of 2 done")
	}

	items, err := store.ListRecords(ctx, "mch_checklist_item")
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %d, %v", len(items), err)
	}
	if items[0].Values["fld_status"] != "todo" {
		t.Errorf("a new item starts as todo, got %v", items[0].Values["fld_status"])
	}
	rec := checklistWrite(t, h, cookie, http.MethodPatch, "/machines/mch_checklist_item/records/"+items[0].ID, map[string]string{"fld_status": "done"}, page)
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("complete item = %d, HX-Refresh %q", rec.Code, rec.Header().Get("HX-Refresh"))
	}
	if body := detail(); !strings.Contains(body, "1 of 2") {
		t.Errorf("completing one item should read 1 of 2")
	}
}
