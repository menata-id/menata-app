package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/domain"
)

// A Task's comments are its child Machine's records drawn as a feed under a "Write a comment" box. The
// author is not an input: whatever a request says in fld_author, the stored value is the person acting, so a
// comment cannot be written in someone else's name -- and a later edit cannot move it either.
func TestTaskDetailCommentsStampTheActingUserAsAuthor(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "comments", "default")
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
	body := detail()
	for _, want := range []string{"Comments and activity", "Write a comment"} {
		if !strings.Contains(body, want) {
			t.Fatalf("a Task with no comments should still offer %q", want)
		}
	}
	if strings.Contains(body, `name="fld_author"`) {
		t.Fatal("the comment box offers an author input; the author must be stamped, never sent")
	}

	tok := csrfTokenFor(t, h, "/machines/mch_task/records")
	send := func(method, path string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", tok.value)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", page)
		req.AddCookie(tok.cookie)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := send(http.MethodPost, "/machines/mch_comment/records", url.Values{
		"fld_task": {task.ID}, "fld_body": {"Looks good, shipping it."}, "fld_author": {"usr_someone_else"},
	})
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("comment = %d, HX-Refresh %q\n%s", rec.Code, rec.Header().Get("HX-Refresh"), rec.Body.String())
	}
	items, err := store.ListRecords(ctx, "mch_comment")
	if err != nil || len(items) != 1 {
		t.Fatalf("comments = %d, %v", len(items), err)
	}
	if got := items[0].Values["fld_author"]; got != actorID {
		t.Fatalf("stored author = %v, want the acting user %q (a submitted fld_author must be ignored)", got, actorID)
	}
	if body := detail(); !strings.Contains(body, "Looks good, shipping it.") {
		t.Error("the comment is not in the Task's feed")
	}

	// mch_comment is append_only, so an edit is refused outright; the author must be untouched either way.
	for _, r := range []*httptest.ResponseRecorder{
		send(http.MethodPatch, "/machines/mch_comment/records/"+items[0].ID, url.Values{"fld_author": {"usr_someone_else"}}),
		send(http.MethodPut, "/machines/mch_comment/records/"+items[0].ID, url.Values{"fld_task": {task.ID}, "fld_body": {"edited"}, "fld_author": {"usr_someone_else"}}),
	} {
		if r.Code == http.StatusOK {
			t.Errorf("an edit of an append-only comment answered %d", r.Code)
		}
	}
	after, err := store.ListRecords(ctx, "mch_comment")
	if err != nil || len(after) != 1 {
		t.Fatalf("comments after edit = %d, %v", len(after), err)
	}
	if got := after[0].Values["fld_author"]; got != actorID {
		t.Errorf("author after an edit attempt = %v, want %q", got, actorID)
	}
}

// Where a Machine is not append_only, an edit still may not write its stamped Field: a submitted value is
// replaced by the stored one (or dropped when the record never had one).
func TestCarryForwardStoredKeepsAStampedFieldsStoredValue(t *testing.T) {
	_, _, ctx, store, _, _, actorID := routerSetupFor(t, "stampcarry", "default")
	rec, err := store.CreateRecord(ctx, "mch_comment", map[string]any{"fld_task": "tsk_x", "fld_body": "hi", "fld_author": actorID})
	if err != nil {
		t.Fatal(err)
	}
	m := &domain.Machine{ID: "mch_comment", Fields: []domain.Field{
		{ID: "fld_body", Type: domain.FieldTypeLongText},
		{ID: "fld_author", Type: domain.FieldTypePerson, Stamp: domain.FieldStampCurrentUser},
	}}
	values := map[string]any{"fld_body": "edited", "fld_author": "usr_someone_else"}
	if err := carryForwardStored(ctx, store, m, rec.ID, nil, values); err != nil {
		t.Fatal(err)
	}
	if values["fld_author"] != actorID || values["fld_body"] != "edited" {
		t.Errorf("values = %v, want the stored author %q and the edited body", values, actorID)
	}
	bare, err := store.CreateRecord(ctx, "mch_comment", map[string]any{"fld_task": "tsk_x", "fld_body": "no author"})
	if err != nil {
		t.Fatal(err)
	}
	values = map[string]any{"fld_body": "edited", "fld_author": "usr_someone_else"}
	if err := carryForwardStored(ctx, store, m, bare.ID, nil, values); err != nil {
		t.Fatal(err)
	}
	if _, ok := values["fld_author"]; ok {
		t.Errorf("a record that never had an author was given one by an edit: %v", values)
	}
}
