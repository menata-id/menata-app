package authorization

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifySession(t *testing.T) {
	valid := signSession("s3cret")

	if !VerifySession(valid, "s3cret") {
		t.Error("VerifySession() = false for a value signed with the same secret, want true")
	}
	if VerifySession(valid, "different-secret") {
		t.Error("VerifySession() = true for a value signed with a different secret, want false")
	}
	if VerifySession("", "s3cret") {
		t.Error("VerifySession() = true for an empty value, want false")
	}
	if VerifySession("garbage", "s3cret") {
		t.Error("VerifySession() = true for an unsigned value, want false")
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
	SetSessionCookie(rec, "s3cret", false)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if !IsAuthenticated(req, "s3cret") {
		t.Error("IsAuthenticated() = false after SetSessionCookie with the matching secret, want true")
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
