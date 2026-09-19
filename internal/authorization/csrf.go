package authorization

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

// CSRFCookieName carries a double-submit CSRF token (security audit 2026-09-19, L1) -- an
// explicit layer on top of SameSite=Lax (session.go), which already blocks most cross-site
// state-changing requests in a compliant browser but isn't defense-in-depth on its own.
const CSRFCookieName = "menata_csrf"

// newCSRFToken generates a fresh random token -- 32 bytes of crypto/rand, hex-encoded, the same
// entropy budget the app's other unguessable identifiers (upload storage keys, storage.Save) use.
func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means the OS entropy source is broken -- there is no safe fallback
		// value to hand back for a token whose entire job is being unguessable.
		panic("authorization: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// EnsureCSRFCookie returns r's existing CSRF token if it carries one, otherwise mints a new one
// and sets it on w. Called on every request (csrfProtect middleware, internal/web/csrf.go) so a
// token is always available to embed in whatever page gets rendered, before that page's own POST
// ever comes back.
func EnsureCSRFCookie(w http.ResponseWriter, r *http.Request, secure bool) string {
	if cookie, err := r.Cookie(CSRFCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	token := newCSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})
	return token
}

// VerifyCSRFToken reports whether submitted matches the token named by cookieValue -- constant-
// time, the same posture VerifyPassword and the session/invite token signatures already take, and
// false for an empty cookie value (a request with no CSRF cookie at all must never verify against
// an also-empty submitted value).
func VerifyCSRFToken(cookieValue, submitted string) bool {
	if cookieValue == "" || submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieValue), []byte(submitted)) == 1
}

// csrfContextKey is unexported so only this package can mint one -- the same collision-avoidance
// reasoning any context key needs, following the shape data.WithWorkspaceScope's own key already
// uses for a request-scoped value read deep in a call chain.
type csrfContextKey struct{}

// WithCSRFToken attaches token to ctx, read back by CSRFTokenFromContext -- the one place both
// internal/web (writes it, once per request, in csrfProtect) and internal/rendering (reads it,
// from machine.templ's pageShell and the shared csrfHiddenInput/csrfHeadersAttr snippets) meet.
// Lives here rather than in internal/web because internal/web already imports internal/rendering
// throughout (render(...) calls), so internal/rendering importing internal/web back to reach a
// context helper there would be a Go import cycle, not just a plane-boundary violation --
// internal/authorization is the one package both sides can already import without one.
func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfContextKey{}, token)
}

// CSRFTokenFromContext returns the token WithCSRFToken attached, or "" if none was (a template
// rendered outside the normal request path, e.g. a future test that builds a templ.Component
// directly without going through csrfProtect).
func CSRFTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(csrfContextKey{}).(string)
	return token
}
