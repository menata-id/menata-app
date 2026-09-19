package authorization

import (
	"testing"
	"time"
)

func TestSignedToken_roundTrip(t *testing.T) {
	token := newSignedToken("s3cret", "hello", time.Hour)
	value, ok := verifySignedToken("s3cret", token)
	if !ok || value != "hello" {
		t.Errorf("verifySignedToken() = (%q, %v), want (hello, true)", value, ok)
	}
}

func TestSignedToken_expired(t *testing.T) {
	token := newSignedToken("s3cret", "hello", -time.Minute)
	if _, ok := verifySignedToken("s3cret", token); ok {
		t.Error("verifySignedToken(expired) ok = true, want false")
	}
}

func TestSignedToken_wrongSecret(t *testing.T) {
	token := newSignedToken("s3cret", "hello", time.Hour)
	if _, ok := verifySignedToken("different-secret", token); ok {
		t.Error("verifySignedToken(wrong secret) ok = true, want false")
	}
}

func TestSignedToken_malformed(t *testing.T) {
	for _, bad := range []string{"", "garbage", "no-dot-here.", ".", "value-no-expiry.abcdef"} {
		if _, ok := verifySignedToken("s3cret", bad); ok {
			t.Errorf("verifySignedToken(%q) ok = true, want false", bad)
		}
	}
}

func TestEmailVerificationToken_roundTrip(t *testing.T) {
	token := NewEmailVerificationToken("s3cret", "person@example.com")
	email, ok := VerifyEmailVerificationToken("s3cret", token)
	if !ok || email != "person@example.com" {
		t.Errorf("VerifyEmailVerificationToken() = (%q, %v), want (person@example.com, true)", email, ok)
	}
}

func TestPasswordResetToken_roundTrip(t *testing.T) {
	token := NewPasswordResetToken("s3cret", "person@example.com")
	email, ok := VerifyPasswordResetToken("s3cret", token)
	if !ok || email != "person@example.com" {
		t.Errorf("VerifyPasswordResetToken() = (%q, %v), want (person@example.com, true)", email, ok)
	}
}

func TestTokenTags_notInterchangeable(t *testing.T) {
	verifyToken := NewEmailVerificationToken("s3cret", "person@example.com")
	if _, ok := VerifyPasswordResetToken("s3cret", verifyToken); ok {
		t.Error("VerifyPasswordResetToken(a verification token) ok = true, want false -- tokens must not be interchangeable")
	}

	resetToken := NewPasswordResetToken("s3cret", "person@example.com")
	if _, ok := VerifyEmailVerificationToken("s3cret", resetToken); ok {
		t.Error("VerifyEmailVerificationToken(a reset token) ok = true, want false -- tokens must not be interchangeable")
	}

	inviteToken := NewInviteToken("s3cret", "person@example.com", "wsp_1")
	if _, ok := VerifyEmailVerificationToken("s3cret", inviteToken); ok {
		t.Error("VerifyEmailVerificationToken(an invite token) ok = true, want false -- tokens must not be interchangeable")
	}
	if _, _, ok := VerifyInviteToken("s3cret", verifyToken); ok {
		t.Error("VerifyInviteToken(a verification token) ok = true, want false -- tokens must not be interchangeable")
	}
}

func TestInviteToken_roundTrip(t *testing.T) {
	token := NewInviteToken("s3cret", "person@example.com", "wsp_1")
	email, workspaceID, ok := VerifyInviteToken("s3cret", token)
	if !ok || email != "person@example.com" || workspaceID != "wsp_1" {
		t.Errorf("VerifyInviteToken() = (%q, %q, %v), want (person@example.com, wsp_1, true)", email, workspaceID, ok)
	}
}
