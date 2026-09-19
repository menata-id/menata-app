package authorization

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
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

// encodeCookie packs subject, the session generation it was issued under (security audit
// 2026-09-19, M2), and their signature into one cookie value: "<subject>|<generation>.<hex hmac>".
// Signing the subject itself (rather than a fixed literal, ROADMAP.md Phase 2's original design)
// is what lets a session actually identify a real mch_user record (Phase 7); signing generation
// alongside it is what makes revocation possible without a server-side session store keyed by
// cookie value -- the signature alone still proves the payload wasn't tampered with, but
// requireAuth additionally checks the generation against data.Store.CurrentSessionGeneration to
// decide whether this particular issuance is still trusted.
func encodeCookie(secret, subject string, generation int) string {
	payload := subject + "|" + strconv.Itoa(generation)
	return payload + "." + sign(secret, payload)
}

// decodeCookie verifies value's signature and returns the subject and generation it names.
func decodeCookie(value, secret string) (subject string, generation int, ok bool) {
	idx := strings.LastIndex(value, ".")
	if idx < 0 {
		return "", 0, false
	}
	payload, sig := value[:idx], value[idx+1:]
	expected := sign(secret, payload)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return "", 0, false
	}

	sepIdx := strings.LastIndex(payload, "|")
	if sepIdx < 0 {
		return "", 0, false
	}
	subject, genStr := payload[:sepIdx], payload[sepIdx+1:]
	if subject == "" {
		return "", 0, false
	}
	generation, err := strconv.Atoi(genStr)
	if err != nil {
		return "", 0, false
	}
	return subject, generation, true
}

// CheckCredentials compares username/password against the configured admin credential in
// constant time, so a wrong-length guess can't be distinguished from a wrong-content one.
func CheckCredentials(username, password, wantUsername, wantPassword string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(wantUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(wantPassword)) == 1
	return userOK && passOK
}

// SetSessionCookie sets a signed session cookie naming subject -- the mch_user record ID this
// login resolves to (ROADMAP.md Phase 7), or a placeholder identity if none is configured yet --
// and generation, the session-revocation counter it was issued under (security audit 2026-09-19,
// M2). Callers fetch generation from data.Store.CurrentSessionGeneration(ctx, subject) at the
// moment they sign someone in, so a session issued after a bump always carries the new number.
func SetSessionCookie(w http.ResponseWriter, secret, subject string, generation int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    encodeCookie(secret, subject, generation),
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

// CurrentUserID returns the subject named by a valid session cookie, ignoring its generation --
// safe for every caller except requireAuth itself, since by the time any handler runs, requireAuth
// has already confirmed the cookie's generation is still current (CurrentSession below is what it
// uses to do that). Everywhere else just wants "who is this" for attribution, not to re-decide
// whether the session is still trusted.
func CurrentUserID(r *http.Request, secret string) (string, bool) {
	subject, _, ok := CurrentSession(r, secret)
	return subject, ok
}

// CurrentSession returns the subject and generation a valid session cookie names -- the one place
// both are needed together, requireAuth's own revocation check (security audit 2026-09-19, M2):
// it compares generation against data.Store.CurrentSessionGeneration(ctx, subject) and treats a
// mismatch as unauthenticated, even though the cookie's HMAC signature alone still verifies.
func CurrentSession(r *http.Request, secret string) (subject string, generation int, ok bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return "", 0, false
	}
	return decodeCookie(cookie.Value, secret)
}

// IsAuthenticated reports whether the request carries a valid, signature-checked session cookie.
// It does not check generation (see CurrentUserID's own doc comment) -- it has no production
// caller today (internal/web's own gate is requireAuth, built on CurrentSession instead), so if a
// future caller means to use this as an authentication gate rather than an informational check, it
// should call CurrentSession and compare generation the same way requireAuth does, not this.
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
// linger) and signed the same way the real session cookie is. It reuses encodeCookie's
// subject|generation shape with generation fixed at 0 -- this cookie isn't a session (nothing ever
// bumps a generation for an email, only for a real session subject) and PendingEmail below simply
// discards the field, but sharing one signed-payload primitive beats forking a second one for a
// cookie that otherwise needs exactly the same tamper-proofing.
func SetPendingEmailCookie(w http.ResponseWriter, secret, email string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     PendingWorkspaceCookieName,
		Value:    encodeCookie(secret, email, 0),
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
	email, _, ok := decodeCookie(cookie.Value, secret)
	return email, ok
}
