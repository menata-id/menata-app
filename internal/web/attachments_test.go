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

// A Task's attachments are its child Machine's records drawn as file rows: the add control uploads through the
// child's own create route (answered with HX-Refresh from the Task page), Download is the /uploads link, and
// remove deletes the row's record.
func TestTaskDetailAttachmentsUploadAndRemove(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "attachments", "default")
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
	if body := detail(); !strings.Contains(body, "Add an attachment") {
		t.Fatal("a Task with no files should still offer the add control")
	}

	tok := csrfTokenFor(t, h, "/machines/mch_task/records")
	send := func(method, path string, fields map[string]string, filename string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for k, v := range fields {
			mw.WriteField(k, v)
		}
		if filename != "" {
			part, _ := mw.CreateFormFile("fld_file", filename)
			part.Write([]byte("%PDF-1.4\n" + strings.Repeat("x", 2048)))
		}
		mw.Close()
		req := httptest.NewRequest(method, path, &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", page)
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := send(http.MethodPost, "/machines/mch_attachment/records", map[string]string{"fld_task": task.ID}, "shooting-schedule.pdf")
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("upload = %d, HX-Refresh %q\n%s", rec.Code, rec.Header().Get("HX-Refresh"), rec.Body.String())
	}
	body := detail()
	for _, want := range []string{"shooting-schedule.pdf", ">PDF<", "/uploads/mch_attachment/fld_file/", "Download", "2 KB"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail after upload is missing %q", want)
		}
	}

	items, err := store.ListRecords(ctx, "mch_attachment")
	if err != nil || len(items) != 1 {
		t.Fatalf("attachments = %d, %v", len(items), err)
	}
	rec = send(http.MethodDelete, "/machines/mch_attachment/records/"+items[0].ID, nil, "")
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Redirect") != "" {
		t.Fatalf("remove = %d, HX-Redirect %q (a row's own delete must not navigate away)", rec.Code, rec.Header().Get("HX-Redirect"))
	}
	if body := detail(); strings.Contains(body, "shooting-schedule.pdf") {
		t.Error("the removed file is still listed")
	}
}
