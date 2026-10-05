package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"menata.app/internal/authorization"
)

// "Copy…" makes a new record from an existing one through the generic create pipeline: the copy carries the
// source's Fields except the ones the runtime writes itself, takes the name the form gave it, and the browser is
// sent to it. The source is untouched; what is not copyable is refused rather than half-copied.
func TestCopyRecordMakesANewRecordFromTheSource(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "copyrec", "default")
	source, err := store.CreateRecord(ctx, "mch_task", map[string]any{
		"fld_title": "Call sheet", "fld_status": "in_progress", "fld_assignee": actorID, "fld_due_date": "2026-10-09", "fld_description": "Confirm every crew member",
	})
	if err != nil {
		t.Fatal(err)
	}
	tok := csrfTokenFor(t, h, "/machines/mch_task/records")
	send := func(machine, id string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/machines/"+machine+"/records/"+id+"/copy", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := send("mch_task", source.ID, url.Values{"fld_title": {"Call sheet v2"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("copy = %d\n%s", rec.Code, rec.Body.String())
	}
	tasks, err := store.ListRecords(ctx, "mch_task")
	if err != nil || len(tasks) != 2 {
		t.Fatalf("tasks = %d, %v; want the source and one copy", len(tasks), err)
	}
	var made = tasks[0]
	if made.ID == source.ID {
		made = tasks[1]
	}
	if want := "/machines/mch_task/records/" + made.ID; rec.Header().Get("HX-Redirect") != want {
		t.Errorf("HX-Redirect = %q, want %q (the copy's own page)", rec.Header().Get("HX-Redirect"), want)
	}
	for field, want := range map[string]any{
		"fld_title": "Call sheet v2", "fld_status": "in_progress", "fld_assignee": actorID, "fld_due_date": "2026-10-09", "fld_description": "Confirm every crew member",
	} {
		if made.Values[field] != want {
			t.Errorf("copy %s = %v, want %v", field, made.Values[field], want)
		}
	}
	if kept, err := store.GetRecord(ctx, "mch_task", source.ID); err != nil || kept.Values["fld_title"] != "Call sheet" {
		t.Errorf("the source = %v, %v; want it untouched", kept, err)
	}

	// A blank name keeps the source's rather than writing a record with no title.
	if rec := send("mch_task", source.ID, url.Values{"fld_title": {"  "}}); rec.Code != http.StatusOK {
		t.Fatalf("copy with a blank name = %d\n%s", rec.Code, rec.Body.String())
	}

	// An append-only Machine is an audit trail, and a Machine the engines write is not the generic route's.
	comment, err := store.CreateRecord(ctx, "mch_comment", map[string]any{"fld_task": source.ID, "fld_body": "hi", "fld_author": actorID})
	if err != nil {
		t.Fatal(err)
	}
	if rec := send("mch_comment", comment.ID, url.Values{"fld_body": {"hi"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("copying an append-only comment = %d, want 422", rec.Code)
	}
	if rec := send("mch_task", "rec_does_not_exist", url.Values{"fld_title": {"x"}}); rec.Code != http.StatusNotFound {
		t.Errorf("copying a record that does not exist = %d, want 404", rec.Code)
	}
}
