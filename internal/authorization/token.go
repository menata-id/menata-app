package authorization

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// newSignedToken packs value and an expiry into one signed, URL-safe-ish token: "<value>|
// <expiresUnix>.<hex hmac>", reusing the same unexported sign this package's session cookies
// already use. Unlike a cookie, an emailed link has nothing to carry an Expires attribute in, so
// the expiry has to travel inside the signed value itself.
func newSignedToken(secret, value string, ttl time.Duration) string {
	payload := value + "|" + strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	return payload + "." + sign(secret, payload)
}

// verifySignedToken checks token's signature and expiry, returning the value it names.
func verifySignedToken(secret, token string) (value string, ok bool) {
	idx := strings.LastIndex(token, ".")
	if idx < 0 {
		return "", false
	}
	payload, sig := token[:idx], token[idx+1:]
	if subtle.ConstantTimeCompare([]byte(sig), []byte(sign(secret, payload))) != 1 {
		return "", false
	}

	sepIdx := strings.LastIndex(payload, "|")
	if sepIdx < 0 {
		return "", false
	}
	value, expiresStr := payload[:sepIdx], payload[sepIdx+1:]
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}
	return value, true
}

// Token type tags prevent a verification token from being replayed as a password-reset token or
// vice versa -- each is signed over a differently-prefixed value, so a signature that verifies
// under one tag simply won't be found under the other.
const (
	tokenTagVerify = "verify:"
	tokenTagReset  = "reset:"
	tokenTagInvite = "invite:"
)

// NewEmailVerificationToken builds a 24-hour link token for a newly-registered email (ROADMAP.md
// Phase 21 round 2, Step D).
func NewEmailVerificationToken(secret, email string) string {
	return newSignedToken(secret, tokenTagVerify+email, 24*time.Hour)
}

// VerifyEmailVerificationToken returns the email a verification token names, if valid and unexpired.
func VerifyEmailVerificationToken(secret, token string) (email string, ok bool) {
	value, ok := verifySignedToken(secret, token)
	if !ok || !strings.HasPrefix(value, tokenTagVerify) {
		return "", false
	}
	return strings.TrimPrefix(value, tokenTagVerify), true
}

// PasswordResetStamp fingerprints the password a reset link was issued against. It is carried inside the
// signed token, so a link stops working the moment the password changes -- by this very link or any other
// route -- without the server storing anything per link (capability-lifecycle.md §3b, K04). A hash of the
// stored hash, not the hash itself, because the token travels in an email.
func PasswordResetStamp(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return hex.EncodeToString(sum[:8])
}

// NewPasswordResetToken builds a 30-minute, single-use link token for a password-reset request (Step E).
// passwordHash is the credential's current hash: the token is valid only while it still is.
func NewPasswordResetToken(secret, email, passwordHash string) string {
	return newSignedToken(secret, tokenTagReset+email+"|"+PasswordResetStamp(passwordHash), 30*time.Minute)
}

// VerifyPasswordResetToken returns the email a reset token names and the stamp of the password it was issued
// against, if valid and unexpired. The caller must compare stamp with PasswordResetStamp of the credential's
// current hash; a token whose stamp differs has already been used (or the password changed since).
func VerifyPasswordResetToken(secret, token string) (email, stamp string, ok bool) {
	value, ok := verifySignedToken(secret, token)
	if !ok || !strings.HasPrefix(value, tokenTagReset) {
		return "", "", false
	}
	rest := strings.TrimPrefix(value, tokenTagReset)
	idx := strings.LastIndex(rest, "|")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// NewInviteToken builds a 7-day link token for a Workspace member invitation (security audit
// 2026-09-19, H1) -- longer-lived than the other two tokens since an invite commonly sits unread
// over a weekend. workspaceID is bound into the signed value alongside email, not just carried
// separately, so a token minted for one Workspace's invite can't be replayed against a different
// membership row of the same email (e.g. a second, unrelated Workspace inviting the same address
// later).
func NewInviteToken(secret, email, workspaceID string) string {
	return newSignedToken(secret, tokenTagInvite+email+"|"+workspaceID, 7*24*time.Hour)
}

// VerifyInviteToken returns the email and Workspace an invite token names, if valid and unexpired.
func VerifyInviteToken(secret, token string) (email, workspaceID string, ok bool) {
	value, ok := verifySignedToken(secret, token)
	if !ok || !strings.HasPrefix(value, tokenTagInvite) {
		return "", "", false
	}
	rest := strings.TrimPrefix(value, tokenTagInvite)
	idx := strings.LastIndex(rest, "|")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}
