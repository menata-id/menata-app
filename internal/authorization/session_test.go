package authorization

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeCookie(t *testing.T) {
	valid := encodeCookie("s3cret", "rec_user1")

	if subject, ok := decodeCookie(valid, "s3cret"); !ok || subject != "rec_user1" {
		t.Errorf("decodeCookie() = (%q, %v), want (rec_user1, true)", subject, ok)
	}
	if _, ok := decodeCookie(valid, "different-secret"); ok {
		t.Error("decodeCookie() ok = true for a value signed with a different secret, want false")
	}
	if _, ok := decodeCookie("", "s3cret"); ok {
		t.Error("decodeCookie() ok = true for an empty value, want false")
	}
	if _, ok := decodeCookie("garbage", "s3cret"); ok {
		t.Error("decodeCookie() ok = true for an unsigned value, want false")
	}
	// A forged subject with no matching signature must not verify, even though it contains a ".".
	if _, ok := decodeCookie("rec_attacker.deadbeef", "s3cret"); ok {
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
	SetSessionCookie(rec, "s3cret", "rec_user1", false)

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

func TestIsAuthenticated_noCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if IsAuthenticated(req, "s3cret") {
		t.Error("IsAuthenticated() = true with no cookie set, want false")
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
