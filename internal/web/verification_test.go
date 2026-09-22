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

// fakeMailer records every Send call instead of delivering anything, so a test can assert
// whether an email would have gone out without needing real SMTP or the log-based fallback.
type fakeMailer struct {
	sent []struct{ to, subject, body string }
}

func (m *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	m.sent = append(m.sent, struct{ to, subject, body string }{to, subject, body})
	return nil
}

func verificationTestConfig() config.Config {
	return config.Config{SessionSecret: "verification-test-secret", AppBaseURL: "http://localhost:8080"}
}

// sessionUserIDFromResponse replays rec's Set-Cookie headers onto a fresh request, since
// authorization.CurrentUserID reads a cookie off a *http.Request, not the *http.Response that
// carried it -- the same round trip a browser would do between this response and its next request.
func sessionUserIDFromResponse(rec *httptest.ResponseRecorder, secret string) (string, bool) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return authorization.CurrentUserID(req, secret)
}

func TestShowVerifyEmail_validToken_marksVerifiedAndLogsIn(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "verify_valid_test@example.com"
	newTestMember(t, pool, store, "Verify Valid Test", "verify-valid-test-workspace", email)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, "Test Person", hash, false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := verificationTestConfig()
	token := authorization.NewEmailVerificationToken(cfg.SessionSecret, email)
	req := httptest.NewRequest(http.MethodGet, "/verify-email?token="+url.QueryEscape(token), nil)
	rec := httptest.NewRecorder()

	showVerifyEmail(store, cfg)(rec, req)

	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !cred.EmailVerified {
		t.Error("EmailVerified = false after a valid /verify-email visit, want true")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (redirect from completeLogin)", rec.Code, http.StatusSeeOther)
	}
	if _, ok := sessionUserIDFromResponse(rec, cfg.SessionSecret); !ok {
		t.Error("no session cookie set after a valid /verify-email visit, want the visitor signed in")
	}
}

func TestShowVerifyEmail_invalidToken_rejectedWithoutVerifying(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "verify_invalid_test@example.com"
	newTestMember(t, pool, store, "Verify Invalid Test", "verify-invalid-test-workspace", email)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, "Test Person", hash, false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := verificationTestConfig()
	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=garbage", nil)
	rec := httptest.NewRecorder()

	showVerifyEmail(store, cfg)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if cred.EmailVerified {
		t.Error("EmailVerified = true after an invalid /verify-email token, want false")
	}
}

func TestShowVerifyEmail_tokenTaggedForPasswordResetIsRejected(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "verify_wrongtag_test@example.com"
	newTestMember(t, pool, store, "Verify Wrong Tag Test", "verify-wrongtag-test-workspace", email)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, "Test Person", hash, false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := verificationTestConfig()
	// A password-reset token must not double as a verification token, even for the same email
	// and secret -- see authorization.TestTokenTags_notInterchangeable.
	resetToken := authorization.NewPasswordResetToken(cfg.SessionSecret, email)
	req := httptest.NewRequest(http.MethodGet, "/verify-email?token="+url.QueryEscape(resetToken), nil)
	rec := httptest.NewRecorder()

	showVerifyEmail(store, cfg)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubmitResendVerification_unverifiedEmail_sendsEmail(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const email = "resend_unverified_test@example.com"
	newTestMember(t, pool, store, "Resend Unverified Test", "resend-unverified-test-workspace", email)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, email, "Test Person", hash, false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := verificationTestConfig()
	mailer := &fakeMailer{}
	req := httptest.NewRequest(http.MethodPost, "/resend-verification", strings.NewReader(url.Values{"email": {email}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	submitResendVerification(store, mailer, cfg)(rec, req)

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

// TestSubmitResendVerification_noEnumeration checks the same generic response and no email sent
// for both an already-verified account and an email with no account at all -- a different
// response either way would let a caller enumerate registered/unverified emails (see the comment
// on submitResendVerification).
func TestSubmitResendVerification_noEnumeration(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	const verifiedEmail = "resend_verified_test@example.com"
	newTestMember(t, pool, store, "Resend Verified Test", "resend-verified-test-workspace", verifiedEmail)

	hash, err := authorization.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := store.CreateCredential(ctx, verifiedEmail, "Test Person", hash, true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	cfg := verificationTestConfig()

	cases := []struct {
		name  string
		email string
	}{
		{"already verified", verifiedEmail},
		{"unknown email", "resend_unknown_test@example.com"},
	}

	var bodies []string
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mailer := &fakeMailer{}
			req := httptest.NewRequest(http.MethodPost, "/resend-verification", strings.NewReader(url.Values{"email": {c.email}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()

			submitResendVerification(store, mailer, cfg)(rec, req)

			if len(mailer.sent) != 0 {
				t.Errorf("mailer.sent = %d messages, want 0", len(mailer.sent))
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			bodies = append(bodies, rec.Body.String())
		})
	}
	if bodies[0] != bodies[1] {
		t.Error("response body differs between an already-verified email and an unknown one, want an identical generic response")
	}
}
