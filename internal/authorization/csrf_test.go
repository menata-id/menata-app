package authorization

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureCSRFCookie_mintsWhenAbsent(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	token := EnsureCSRFCookie(rec, req, false)
	if token == "" {
		t.Fatal("EnsureCSRFCookie() = \"\", want a non-empty token")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != CSRFCookieName || cookies[0].Value != token {
		t.Errorf("cookies = %+v, want one %s cookie carrying %q", cookies, CSRFCookieName, token)
	}
}

func TestEnsureCSRFCookie_reusesExisting(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "already-issued-token"})

	token := EnsureCSRFCookie(rec, req, false)
	if token != "already-issued-token" {
		t.Errorf("EnsureCSRFCookie() = %q, want the existing cookie's value unchanged", token)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("EnsureCSRFCookie() set a new cookie although a valid one was already present")
	}
}

func TestVerifyCSRFToken(t *testing.T) {
	cases := []struct {
		name, cookie, submitted string
		want                    bool
	}{
		{"match", "tok123", "tok123", true},
		{"mismatch", "tok123", "tok456", false},
		{"empty cookie", "", "tok123", false},
		{"empty submitted", "tok123", "", false},
		{"both empty", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VerifyCSRFToken(c.cookie, c.submitted); got != c.want {
				t.Errorf("VerifyCSRFToken(%q, %q) = %v, want %v", c.cookie, c.submitted, got, c.want)
			}
		})
	}
}

func TestCSRFTokenContext_roundTrip(t *testing.T) {
	ctx := WithCSRFToken(context.Background(), "a-token")
	if got := CSRFTokenFromContext(ctx); got != "a-token" {
		t.Errorf("CSRFTokenFromContext() = %q, want a-token", got)
	}
}

func TestCSRFTokenContext_absent(t *testing.T) {
	if got := CSRFTokenFromContext(context.Background()); got != "" {
		t.Errorf("CSRFTokenFromContext(no value set) = %q, want \"\"", got)
	}
}
