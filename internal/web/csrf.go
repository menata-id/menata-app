package web

import (
	"net/http"
	"strings"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
)

// csrfProtect is a global, double-submit-cookie CSRF gate (security audit 2026-09-19, L1),
// registered once in Routes() alongside secureHeaders rather than per route group -- it covers the
// public POST routes (/login, /register, ...) and the pr/ar groups (requireAuth/
// requireWorkspaceAdmin) uniformly, without being registered more than once.
//
// Every request, safe or not, gets EnsureCSRFCookie's token attached to its context
// (authorization.WithCSRFToken) so whatever page a GET renders can embed it (machine.templ's
// pageShell for HTMX's hx-headers, and the shared csrfHiddenInput snippet for plain forms) before
// that page's own POST ever comes back.
//
// GET/HEAD/OPTIONS never mutate anything, so they're not checked, only given a token. For a
// mutating method, the submitted token is read from the X-CSRF-Token header first (what
// pageShell's page-wide hx-headers sends for every HTMX request) and only falls back to the
// csrf_token form field for a non-multipart body -- a multipart request is never read via
// r.FormValue here, because doing so would trigger Go's default-32MB ParseMultipartForm before the
// handler's own req.ParseMultipartForm(maxUploadBytes) (record.go) gets to apply the app's real
// 20MB limit. Every multipart POST/PUT in this app is HTMX-driven (documentsubmit.templ,
// detail.templ) and so always carries the header already -- a multipart request without it is
// correctly rejected, not silently allowed through.
func csrfProtect(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			token := authorization.EnsureCSRFCookie(w, req, cfg.SecureCookies)
			req = req.WithContext(authorization.WithCSRFToken(req.Context(), token))

			switch req.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, req)
				return
			}

			submitted := req.Header.Get("X-CSRF-Token")
			if submitted == "" && !strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data") {
				submitted = req.FormValue("csrf_token")
			}
			if !authorization.VerifyCSRFToken(token, submitted) {
				http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
