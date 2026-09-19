package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
)

func csrfTestConfig() config.Config {
	return config.Config{SecureCookies: false}
}

// TestCSRFProtect_getMintsCookieAndPassesThrough covers the "safe method" half: a GET is never
// checked, but it must still come away with a token attached, since that's what a subsequent
// render call reads to embed into the page.
func TestCSRFProtect_getMintsCookieAndPassesThrough(t *testing.T) {
	var gotToken string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = authorization.CSRFTokenFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	csrfProtect(csrfTestConfig())(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotToken == "" {
		t.Error("no CSRF token reached the inner handler's context")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != authorization.CSRFCookieName || cookies[0].Value != gotToken {
		t.Errorf("cookies = %+v, want one %s cookie matching the context token", cookies, authorization.CSRFCookieName)
	}
}

// TestCSRFProtect_postWithoutTokenRejected is the core regression test for L1.
func TestCSRFProtect_postWithoutTokenRejected(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	csrfProtect(csrfTestConfig())(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a POST with no CSRF cookie or token at all", rec.Code)
	}
}

// TestCSRFProtect_postWithMatchingFormTokenPasses covers the plain-form path (login, register,
// ... -- csrfHiddenInput's own hidden field).
func TestCSRFProtect_postWithMatchingFormTokenPasses(t *testing.T) {
	var reached bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("csrf_token=tok123&username=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "tok123"})
	rec := httptest.NewRecorder()
	csrfProtect(csrfTestConfig())(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !reached {
		t.Errorf("status = %d, reached = %v, want 200/true for a matching cookie+form token", rec.Code, reached)
	}
}

// TestCSRFProtect_postWithMismatchedFormTokenRejected confirms a tampered/forged token, not just
// an absent one, is rejected.
func TestCSRFProtect_postWithMismatchedFormTokenRejected(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("csrf_token=wrong-token&username=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "tok123"})
	rec := httptest.NewRecorder()
	csrfProtect(csrfTestConfig())(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a form token that doesn't match the cookie", rec.Code)
	}
}

// TestCSRFProtect_postWithMatchingHeaderTokenPasses covers the HTMX path (pageShell's page-wide
// hx-headers), including a multipart body -- the header must be checked without ever calling
// r.FormValue on a multipart request (see csrfProtect's own doc comment for why).
func TestCSRFProtect_postWithMatchingHeaderTokenPasses(t *testing.T) {
	var reached bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader("--boundary--\r\n"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	req.Header.Set("X-CSRF-Token", "tok123")
	req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "tok123"})
	rec := httptest.NewRecorder()
	csrfProtect(csrfTestConfig())(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !reached {
		t.Errorf("status = %d, reached = %v, want 200/true for a matching header token on a multipart request", rec.Code, reached)
	}
}

// TestCSRFProtect_multipartWithoutHeaderRejectedWithoutConsumingBody confirms a multipart request
// missing the header is rejected outright, never falling back to parsing the body (which would
// both be wrong -- csrf_token isn't expected in a multipart body in this app -- and would bypass
// the app's own upload size limit by parsing with Go's default instead of maxUploadBytes).
func TestCSRFProtect_multipartWithoutHeaderRejectedWithoutConsumingBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader("--boundary--\r\n"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "tok123"})
	rec := httptest.NewRecorder()
	csrfProtect(csrfTestConfig())(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a multipart POST with no X-CSRF-Token header", rec.Code)
	}
}
