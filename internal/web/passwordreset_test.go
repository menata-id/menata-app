package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
)

func passwordResetTestConfig() config.Config {
	return config.Config{SessionSecret: "passwordreset-test-secret", AppBaseURL: "http://localhost:8080"}
}

func TestSubmitForgotPassword_existingEmail_sendsEmail(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "forgot_existing_test@example.com"
	newTestMember(t, pool, store, "Forgot Existing Test", "forgot-existing-test-workspace", email)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := passwordResetTestConfig()
	mailer := &fakeMailer{}
	req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(url.Values{"email": {email}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	submitForgotPassword(store, mailer, cfg)(rec, req)

	if len(mailer.sent) != 1 {
		t.Fatalf("mailer.sent = %d messages, want 1", len(mailer.sent))
	}
	if mailer.sent[0].to != email {
		t.Errorf("sent to %q, want %q", mailer.sent[0].to, email)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestSubmitForgotPassword_unknownEmail_sameResponseNoEmail checks that requesting a reset for an
// email with no account produces the exact same response as a real one, and never sends mail --
// otherwise this endpoint would let a caller enumerate registered emails (see the comment on
// submitForgotPassword).
func TestSubmitForgotPassword_unknownEmail_sameResponseNoEmail(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const knownEmail = "forgot_known_test@example.com"
	newTestMember(t, pool, store, "Forgot Known Test", "forgot-known-test-workspace", knownEmail)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, knownEmail, hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := passwordResetTestConfig()

	post := func(email string) (int, string, int) {
		mailer := &fakeMailer{}
		req := httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(url.Values{"email": {email}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		submitForgotPassword(store, mailer, cfg)(rec, req)
		return rec.Code, rec.Body.String(), len(mailer.sent)
	}

	knownCode, knownBody, knownSent := post(knownEmail)
	unknownCode, unknownBody, unknownSent := post("forgot_unknown_test@example.com")

	if knownSent != 1 {
		t.Errorf("known email: mailer.sent = %d, want 1", knownSent)
	}
	if unknownSent != 0 {
		t.Errorf("unknown email: mailer.sent = %d, want 0", unknownSent)
	}
	if knownCode != unknownCode || knownBody != unknownBody {
		t.Error("response differs between a known and an unknown email, want an identical generic response")
	}
}

func TestSubmitResetPassword_validToken_updatesPasswordVerifiesAndLogsIn(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "reset_valid_test@example.com"
	newTestMember(t, pool, store, "Reset Valid Test", "reset-valid-test-workspace", email)

	oldHash, err := authorization.HashPassword("the-old-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// Starts unverified -- completing a reset must also verify it (submitResetPassword's own
	// comment on closing the gap for someone who registered but never verified).
	if err := store.CreateCredential(ctx, email, oldHash, false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := passwordResetTestConfig()
	token := authorization.NewPasswordResetToken(cfg.SessionSecret, email)
	form := url.Values{"token": {token}, "password": {"a-brand-new-password"}}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	submitResetPassword(store, cfg)(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (redirect from completeLogin)", rec.Code, http.StatusSeeOther)
	}
	if _, ok := sessionUserIDFromResponse(rec, cfg.SessionSecret); !ok {
		t.Error("no session cookie set after a valid /reset-password submit, want the visitor signed in")
	}

	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !cred.EmailVerified {
		t.Error("EmailVerified = false after a completed password reset, want true")
	}
	if !authorization.VerifyPassword("a-brand-new-password", cred.PasswordHash) {
		t.Error("new password does not verify against the stored hash")
	}
	if authorization.VerifyPassword("the-old-password", cred.PasswordHash) {
		t.Error("old password still verifies against the stored hash, want it replaced")
	}
}

func TestSubmitResetPassword_invalidToken_rejectedWithoutChangingCredential(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "reset_invalid_test@example.com"
	newTestMember(t, pool, store, "Reset Invalid Test", "reset-invalid-test-workspace", email)

	oldHash, err := authorization.HashPassword("the-old-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, oldHash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := passwordResetTestConfig()
	form := url.Values{"token": {"garbage"}, "password": {"a-brand-new-password"}}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	submitResetPassword(store, cfg)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !authorization.VerifyPassword("the-old-password", cred.PasswordHash) {
		t.Error("password hash changed after an invalid /reset-password token, want it untouched")
	}
}

func TestSubmitResetPassword_tokenTaggedForVerificationIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "reset_wrongtag_test@example.com"
	newTestMember(t, pool, store, "Reset Wrong Tag Test", "reset-wrongtag-test-workspace", email)

	hash, err := authorization.HashPassword("the-old-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := passwordResetTestConfig()
	// An email-verification token must not double as a password-reset token, even for the same
	// email and secret -- see authorization.TestTokenTags_notInterchangeable.
	verifyToken := authorization.NewEmailVerificationToken(cfg.SessionSecret, email)
	form := url.Values{"token": {verifyToken}, "password": {"a-brand-new-password"}}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	submitResetPassword(store, cfg)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitResetPassword_shortPassword_rerendersFormWithoutChanging(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "reset_short_test@example.com"
	newTestMember(t, pool, store, "Reset Short Test", "reset-short-test-workspace", email)

	oldHash, err := authorization.HashPassword("the-old-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, oldHash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := passwordResetTestConfig()
	token := authorization.NewPasswordResetToken(cfg.SessionSecret, email)
	form := url.Values{"token": {token}, "password": {"short"}}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	submitResetPassword(store, cfg)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (form re-rendered with an error, not redirected)", rec.Code, http.StatusOK)
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !authorization.VerifyPassword("the-old-password", cred.PasswordHash) {
		t.Error("password hash changed after a too-short /reset-password submit, want it untouched")
	}
}

// TestSubmitResetPassword_invalidatesExistingSessions is the regression test for security audit
// 2026-09-19's M2: a session cookie issued before a password reset must stop being trusted once
// the reset completes, even though its own signature is still perfectly valid -- the whole point
// of tracking a server-side generation instead of relying on the cookie alone.
func TestSubmitResetPassword_invalidatesExistingSessions(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "reset_invalidate_test@example.com"
	userID := newTestMember(t, pool, store, "Reset Invalidate Test", "reset-invalidate-test-workspace", email)

	oldHash, err := authorization.HashPassword("the-old-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, oldHash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := passwordResetTestConfig()
	genBefore, err := store.CurrentSessionGeneration(ctx, userID)
	if err != nil {
		t.Fatalf("CurrentSessionGeneration (before): %v", err)
	}

	token := authorization.NewPasswordResetToken(cfg.SessionSecret, email)
	form := url.Values{"token": {token}, "password": {"a-brand-new-password"}}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	submitResetPassword(store, cfg)(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	genAfter, err := store.CurrentSessionGeneration(ctx, userID)
	if err != nil {
		t.Fatalf("CurrentSessionGeneration (after): %v", err)
	}
	if genAfter == genBefore {
		t.Errorf("session generation unchanged (%d) after a completed password reset, want it bumped", genAfter)
	}

	// A cookie issued under the pre-reset generation must now be rejected by requireAuth, even
	// with a perfectly valid signature.
	staleCookie := sessionCookieValueForTest(t, cfg, userID, genBefore)
	authedReq := httptest.NewRequest(http.MethodGet, "/home", nil)
	authedReq.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: staleCookie})
	authedRec := httptest.NewRecorder()
	requireAuth(store, "", cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(authedRec, authedReq)
	if authedRec.Code == http.StatusOK {
		t.Error("requireAuth let a pre-reset session cookie through, want it rejected")
	}
}

func TestSubmitResetPassword_noCredentialRow_rejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	const email = "reset_nocredential_test@example.com"
	newTestMember(t, pool, store, "Reset No Credential Test", "reset-nocredential-test-workspace", email)
	// Deliberately no CreateCredential call: this member exists but has never set a password,
	// exactly like SetCredential's own doc comment describes -- a reset token is only ever issued
	// for an email GetCredential already found, so this path must fail loudly rather than create
	// a brand-new credential no one asked for.

	cfg := passwordResetTestConfig()
	token := authorization.NewPasswordResetToken(cfg.SessionSecret, email)
	form := url.Values{"token": {token}, "password": {"a-brand-new-password"}}
	req := httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	submitResetPassword(store, cfg)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
