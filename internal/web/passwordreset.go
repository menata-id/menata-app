package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/mail"
	"menata.app/internal/rendering"
)

func showForgotPassword(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.ForgotPasswordPage(""))
}

// submitForgotPassword always renders the same generic confirmation regardless of whether email
// has an account -- a different response would let this be used to enumerate registered emails,
// the same reasoning Step D's resend-verification already applies. Sent for any existing
// credential regardless of EmailVerified: completing a reset is itself proof of inbox ownership
// (see submitResetPassword), so there's no reason to withhold it from an unverified account.
func submitForgotPassword(store *data.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		email := normalizeEmail(req.FormValue("email"))
		if _, err := store.GetCredential(req.Context(), email); err == nil {
			sendPasswordResetEmail(req.Context(), mailer, cfg, email)
		}
		render(req.Context(), w, rendering.ForgotPasswordPage("If that email has an account, we've sent a link to reset your password."))
	}
}

// sendPasswordResetEmail builds and sends (or, via mail.LogMailer, logs) email's reset link.
// Best-effort, same posture as sendVerificationEmail: a failed send is logged, not returned as an
// error.
func sendPasswordResetEmail(ctx context.Context, mailer mail.Mailer, cfg config.Config, email string) {
	token := authorization.NewPasswordResetToken(cfg.SessionSecret, email)
	link := cfg.AppBaseURL + "/reset-password?token=" + url.QueryEscape(token)
	body := fmt.Sprintf("Reset your Menata App password by clicking this link (valid 30 minutes):\n\n%s\n\nIf you didn't request this, you can ignore this email -- your password won't change.", link)
	if err := mailer.Send(ctx, email, "Reset your password - Menata App", body); err != nil {
		log.Printf("failed to send password reset email to %s: %v", email, err)
	}
}

func showResetPassword(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.ResetPasswordPage(req.URL.Query().Get("token"), "", "Choose a new password", "/reset-password", "Set new password"))
}

// submitResetPassword re-verifies the token rather than trusting the one already rendered into
// the form -- a stale or tampered token must not be treated as still valid just because a page
// showing it once rendered successfully.
func submitResetPassword(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		token := req.FormValue("token")
		password := req.FormValue("password")

		email, ok := authorization.VerifyPasswordResetToken(cfg.SessionSecret, token)
		if !ok {
			http.Error(w, "invalid or expired reset link", http.StatusBadRequest)
			return
		}
		if len(password) < 8 {
			render(req.Context(), w, rendering.ResetPasswordPage(token, "Password must be at least 8 characters.", "Choose a new password", "/reset-password", "Set new password"))
			return
		}

		hash, err := authorization.HashPassword(password)
		if err != nil {
			serverError(w, err)
			return
		}
		if err := store.SetCredential(req.Context(), email, hash); err != nil {
			if errors.Is(err, data.ErrCredentialNotFound) {
				http.Error(w, "invalid or expired reset link", http.StatusBadRequest)
				return
			}
			serverError(w, err)
			return
		}
		// Completing a reset is itself proof of inbox ownership -- equivalent to clicking a
		// verification link, so it closes the same gap for anyone who registered but never
		// verified before losing their password.
		if err := store.MarkEmailVerified(req.Context(), email); err != nil {
			serverError(w, err)
			return
		}
		if err := invalidateSessionsFor(req.Context(), store, email); err != nil {
			serverError(w, err)
			return
		}

		if err := completeLogin(w, req, cfg, store, email); err != nil {
			serverError(w, err)
		}
	}
}
