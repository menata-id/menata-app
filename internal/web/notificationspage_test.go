package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
)

// TestNotificationsIsARuntimePage: /notifications is the runtime's declared page (metadata/runtimepages), and it shows the
// viewer their own notifications only, each with the words it carries, the route it leads to, when it was written and a
// mark when it is unread; the Mark all as read Button is drawn while something is unread and the route it posts to marks
// the viewer's and nobody else's.
func TestNotificationsIsARuntimePage(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "notifpage", "default")
	other, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "notifpage_other@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", other.ID) })
	mk := func(recipient, msg, link, read string) string {
		rec, err := store.CreateRecord(ctx, "mch_notification", map[string]any{"fld_recipient": recipient, "fld_message": msg, "fld_link": link, "fld_read": read})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_notification", rec.ID) })
		return rec.ID
	}
	mk(actorID, "NP-mine-unread", "/home", "unread")
	alreadyRead := mk(actorID, "NP-mine-read", "/home", "read")
	before, _ := store.GetRecord(ctx, "mch_notification", alreadyRead)
	mk(actorID, "NP-mine-offsite", "https://evil.example/x", "read")
	theirs := mk(other.ID, "NP-theirs", "/home", "unread")

	body := getPage(t, h, cookie, "/notifications")
	for _, want := range []string{"NP-mine-unread", "NP-mine-read", `href="/home"`, "Mark all as read", ">New<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the notifications page does not contain %q", want)
		}
	}
	if strings.Contains(body, "NP-theirs") {
		t.Error("the page shows another person's notification")
	}
	if strings.Contains(body, "evil.example") {
		t.Error("a stored link that leaves the app was drawn as a destination")
	}
	if !strings.Contains(body, "NP-mine-offsite") {
		t.Error("a notification whose link was refused lost its words")
	}
	if strings.Count(body, ">New<") != 1 {
		t.Errorf("%d unread marks, want exactly the one unread notification's", strings.Count(body, ">New<"))
	}

	// Open to every member, not only to an admin, and to nobody who is not one.
	wsID, _ := data.WorkspaceScope(ctx)
	member, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "notifpage_member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", member.ID) })
	if err := store.AddMember(context.Background(), wsID, member.ID, "notifpage_member@example.com", "member", ""); err != nil {
		t.Fatal(err)
	}
	stranger, err := store.CreateRecord(ctx, "mch_user", map[string]any{"fld_email": "notifpage_stranger@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_user", stranger.ID) })
	for who, tc := range map[string]struct {
		id   string
		want int
	}{"a plain member": {member.ID, http.StatusOK}, "someone with no membership": {stranger.ID, http.StatusForbidden}} {
		req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: sessionCookieValueForTest(t, config.Config{SessionSecret: "notifpage-secret"}, tc.id, 0)})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s opening /notifications = %d, want %d", who, w.Code, tc.want)
		}
	}

	post := func(ck string) int {
		req := httptest.NewRequest(http.MethodPost, "/machines/mch_notification/transition-all?dataset=ds_my_notifications&to=read", nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: ck})
		req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "t"})
		req.Header.Set("X-CSRF-Token", "t")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post(cookie); code != http.StatusOK {
		t.Fatalf("mark all as read = %d, want 200", code)
	}
	body = getPage(t, h, cookie, "/notifications")
	if strings.Contains(body, "Mark all as read") || strings.Contains(body, ">New<") {
		t.Error("everything is read and the page still offers to mark it so")
	}
	if after, _ := store.GetRecord(ctx, "mch_notification", alreadyRead); after == nil || before == nil || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Error("marking all read rewrote a notification that was already read")
	}
	if rec, _ := store.GetRecord(ctx, "mch_notification", theirs); rec == nil || rec.Values["fld_read"] != "unread" {
		t.Error("marking the viewer's notifications read touched another person's")
	}
}

// TestTransitionAllRefuses: the bulk route names a Dataset and a status and nothing else, so it refuses what cannot be
// meant -- a Dataset of another Machine, a Dataset nobody declares, a status the Machine's Field cannot hold -- and moves
// nothing when it does.
func TestTransitionAllRefuses(t *testing.T) {
	h, cookie, ctx, store, _, _, actorID := routerSetupFor(t, "bulkrefuse", "default")
	rec, err := store.CreateRecord(ctx, "mch_notification", map[string]any{"fld_recipient": actorID, "fld_message": "BR", "fld_read": "unread"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.DeleteRecord(ctx, "mch_notification", rec.ID) })
	for name, tc := range map[string]struct {
		path string
		want int
	}{
		"a dataset of another machine": {"/machines/mch_notification/transition-all?dataset=ds_helpdesk_tickets_table&to=read", http.StatusNotFound},
		"a dataset nobody declares":    {"/machines/mch_notification/transition-all?dataset=ds_nobody&to=read", http.StatusNotFound},
		"a status the field lacks":     {"/machines/mch_notification/transition-all?dataset=ds_my_notifications&to=archived", http.StatusUnprocessableEntity},
		"a machine nobody installs":    {"/machines/mch_nobody/transition-all?dataset=ds_my_notifications&to=read", http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: cookie})
		req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "t"})
		req.Header.Set("X-CSRF-Token", "t")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s = %d, want %d", name, w.Code, tc.want)
		}
	}
	if got, _ := store.GetRecord(ctx, "mch_notification", rec.ID); got == nil || got.Values["fld_read"] != "unread" {
		t.Error("a refused request moved a record")
	}
}
