package authorization

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeCookie(t *testing.T) {
	valid := encodeCookie("s3cret", "rec_user1", 3)

	if subject, generation, ok := decodeCookie(valid, "s3cret"); !ok || subject != "rec_user1" || generation != 3 {
		t.Errorf("decodeCookie() = (%q, %d, %v), want (rec_user1, 3, true)", subject, generation, ok)
	}
	if _, _, ok := decodeCookie(valid, "different-secret"); ok {
		t.Error("decodeCookie() ok = true for a value signed with a different secret, want false")
	}
	if _, _, ok := decodeCookie("", "s3cret"); ok {
		t.Error("decodeCookie() ok = true for an empty value, want false")
	}
	if _, _, ok := decodeCookie("garbage", "s3cret"); ok {
		t.Error("decodeCookie() ok = true for an unsigned value, want false")
	}
	// A forged subject with no matching signature must not verify, even though it contains a ".".
	if _, _, ok := decodeCookie("rec_attacker|0.deadbeef", "s3cret"); ok {
		t.Error("decodeCookie() ok = true for a forged subject/signature pair, want false")
	}
}

func TestCheckCredentials(t *testing.T) {
	cases := []struct {
		name, user, pass string
		want             bool
	}{
		{"correct", "admin", "hunter2", true},
		{"wrong password", "admin", "wrong", false},
		{"wrong username", "nobody", "hunter2", false},
		{"both empty", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckCredentials(c.user, c.pass, "admin", "hunter2"); got != c.want {
				t.Errorf("CheckCredentials(%q, %q) = %v, want %v", c.user, c.pass, got, c.want)
			}
		})
	}
}

func TestSetSessionCookie_roundTrip(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, "s3cret", "rec_user1", 0, false)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if !IsAuthenticated(req, "s3cret") {
		t.Error("IsAuthenticated() = false after SetSessionCookie with the matching secret, want true")
	}
	if userID, ok := CurrentUserID(req, "s3cret"); !ok || userID != "rec_user1" {
		t.Errorf("CurrentUserID() = (%q, %v), want (rec_user1, true)", userID, ok)
	}
}

func TestCurrentSession_returnsGeneration(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, "s3cret", "rec_user1", 5, false)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	subject, generation, ok := CurrentSession(req, "s3cret")
	if !ok || subject != "rec_user1" || generation != 5 {
		t.Errorf("CurrentSession() = (%q, %d, %v), want (rec_user1, 5, true)", subject, generation, ok)
	}
}

func TestIsAuthenticated_noCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if IsAuthenticated(req, "s3cret") {
		t.Error("IsAuthenticated() = true with no cookie set, want false")
	}
}

func TestPendingEmailCookie_roundTrip(t *testing.T) {
	rec := httptest.NewRecorder()
	SetPendingEmailCookie(rec, "s3cret", "person@example.com", false)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	email, ok := PendingEmail(req, "s3cret")
	if !ok || email != "person@example.com" {
		t.Errorf("PendingEmail() = (%q, %v), want (person@example.com, true)", email, ok)
	}
}

func TestPendingEmail_noCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := PendingEmail(req, "s3cret"); ok {
		t.Error("PendingEmail() ok = true with no cookie set, want false")
	}
}

func TestClearPendingEmailCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearPendingEmailCookie(rec, false)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("ClearPendingEmailCookie() cookies = %+v, want one cookie with MaxAge < 0", cookies)
	}
}

func TestClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearSessionCookie(rec, false)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatalf("ClearSessionCookie() cookies = %+v, want one cookie with MaxAge < 0", cookies)
	}
}
