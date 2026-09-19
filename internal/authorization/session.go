package authorization

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

// SessionCookieName is the cookie carrying the signed session value.
const SessionCookieName = "menata_session"

// sign returns the HMAC-SHA256 of subject, keyed by secret.
func sign(secret, subject string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(subject))
	return hex.EncodeToString(mac.Sum(nil))
}

// encodeCookie packs subject and its signature into one cookie value: "<subject>.<hex hmac>".
// Signing the subject itself (rather than a fixed literal, ROADMAP.md Phase 2's original design)
// is what lets a session actually identify a real mch_user record (Phase 7) -- no server-side
// session store is needed either way, since the signature alone proves the subject wasn't
// tampered with.
func encodeCookie(secret, subject string) string {
	return subject + "." + sign(secret, subject)
}

// decodeCookie verifies value's signature and returns the subject it names.
func decodeCookie(value, secret string) (subject string, ok bool) {
	idx := strings.LastIndex(value, ".")
	if idx < 0 {
		return "", false
	}
	subject, sig := value[:idx], value[idx+1:]
	if subject == "" {
		return "", false
	}
	expected := sign(secret, subject)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return "", false
	}
	return subject, true
}

// CheckCredentials compares username/password against the configured admin credential in
// constant time, so a wrong-length guess can't be distinguished from a wrong-content one.
func CheckCredentials(username, password, wantUsername, wantPassword string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(wantUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(wantPassword)) == 1
	return userOK && passOK
}

// SetSessionCookie sets a signed session cookie naming subject -- the mch_user record ID this
// login resolves to (ROADMAP.md Phase 7), or a placeholder identity if none is configured yet.
func SetSessionCookie(w http.ResponseWriter, secret, subject string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    encodeCookie(secret, subject),
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})
}

// ClearSessionCookie removes the session cookie (logout).
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// CurrentUserID returns the subject named by a valid session cookie -- today, always the single
// configured admin identity (config.AdminUserID); a real per-user login is a later, separate
// step once a second real user forces it (ROADMAP.md Phase 2's original deferral, still true).
func CurrentUserID(r *http.Request, secret string) (string, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return "", false
	}
	return decodeCookie(cookie.Value, secret)
}

// IsAuthenticated reports whether the request carries a valid session cookie.
func IsAuthenticated(r *http.Request, secret string) bool {
	_, ok := CurrentUserID(r, secret)
	return ok
}

// PendingWorkspaceCookieName carries a verified email between a real-credential login and Choose
// Workspace (ROADMAP.md Phase 21 Step 4), for the one login whose password check succeeds but
// names more than one Workspace membership -- the real session cookie isn't set yet at that
// point, since which mch_user record to sign in as depends on which Workspace gets picked.
const PendingWorkspaceCookieName = "menata_pending_email"

// SetPendingEmailCookie names email as a password check that has already succeeded, short-lived
// (5 minutes -- long enough to pick a Workspace, short enough that an abandoned attempt doesn't
// linger) and signed the same way the real session cookie is.
func SetPendingEmailCookie(w http.ResponseWriter, secret, email string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     PendingWorkspaceCookieName,
		Value:    encodeCookie(secret, email),
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(5 * time.Minute),
	})
}

// ClearPendingEmailCookie removes the pending-choice cookie once Choose Workspace completes.
func ClearPendingEmailCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     PendingWorkspaceCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// PendingEmail returns the email a prior login verified, if a valid pending-choice cookie is
// present.
func PendingEmail(r *http.Request, secret string) (string, bool) {
	cookie, err := r.Cookie(PendingWorkspaceCookieName)
	if err != nil {
		return "", false
	}
	return decodeCookie(cookie.Value, secret)
}
