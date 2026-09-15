package authorization

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

// SessionCookieName is the cookie carrying the signed session value.
const SessionCookieName = "menata_session"

// sessionSubject is the sole identity Phase 2 recognizes. Real per-user identity is deferred
// (ROADMAP.md Phase 2 design note) until a second real user forces it.
const sessionSubject = "admin"

// signSession returns the HMAC-SHA256 of the session subject, keyed by secret. Verifying a
// cookie means recomputing this and comparing in constant time -- no server-side session store
// is needed for a single fixed subject.
func signSession(secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sessionSubject))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySession reports whether value is a valid signed session for secret.
func VerifySession(value, secret string) bool {
	if value == "" {
		return false
	}
	expected := signSession(secret)
	return subtle.ConstantTimeCompare([]byte(value), []byte(expected)) == 1
}

// CheckCredentials compares username/password against the configured admin credential in
// constant time, so a wrong-length guess can't be distinguished from a wrong-content one.
func CheckCredentials(username, password, wantUsername, wantPassword string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(wantUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(wantPassword)) == 1
	return userOK && passOK
}

// SetSessionCookie sets a signed session cookie on the response.
func SetSessionCookie(w http.ResponseWriter, secret string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    signSession(secret),
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

// IsAuthenticated reports whether the request carries a valid session cookie.
func IsAuthenticated(r *http.Request, secret string) bool {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return false
	}
	return VerifySession(cookie.Value, secret)
}
