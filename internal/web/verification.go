package web

import (
	"context"
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

// sendVerificationEmail builds and sends (or, via mail.LogMailer, logs) email's verification
// link. Best-effort: a failed send is logged, not returned as an error -- the account already
// exists and registration/resend must not fail just because the mailer had a bad moment, the same
// posture logActivity's own writes already take elsewhere.
func sendVerificationEmail(ctx context.Context, mailer mail.Mailer, cfg config.Config, email string) {
	token := authorization.NewEmailVerificationToken(cfg.SessionSecret, email)
	link := cfg.AppBaseURL + "/verify-email?token=" + url.QueryEscape(token)
	body := fmt.Sprintf("Verify your email to finish setting up your Menata App workspace (link valid 24 hours):\n\n%s\n\nIf you didn't request this, you can ignore this email.", link)
	if err := mailer.Send(ctx, email, "Verify your email - Menata App", body); err != nil {
		log.Printf("failed to send verification email to %s: %v", email, err)
	}
}

// showVerifyEmail completes registration's own blocking verification (ROADMAP.md Phase 21 round 2,
// Step D): a valid, unexpired token marks the credential verified and signs the person straight
// in, the same way a normal login would.
func showVerifyEmail(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		email, ok := authorization.VerifyEmailVerificationToken(cfg.SessionSecret, req.URL.Query().Get("token"))
		if !ok {
			http.Error(w, "invalid or expired verification link", http.StatusBadRequest)
			return
		}
		if err := store.MarkEmailVerified(req.Context(), email); err != nil {
			serverError(w, err)
			return
		}
		if err := completeLogin(w, req, cfg, store, email); err != nil {
			serverError(w, err)
		}
	}
}

func showResendVerification(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.ResendVerificationPage(""))
}

// submitResendVerification always renders the same generic confirmation regardless of whether
// email exists or is already verified -- a different response would let a caller enumerate
// registered emails, cheap to avoid at design time (the same reasoning Step E's forgot-password
// flow uses).
func submitResendVerification(store *data.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		email := normalizeEmail(req.FormValue("email"))
		if cred, err := store.GetCredential(req.Context(), email); err == nil && !cred.EmailVerified {
			sendVerificationEmail(req.Context(), mailer, cfg, email)
		}
		render(req.Context(), w, rendering.ResendVerificationPage("If that email needs verifying, we've sent a new link."))
	}
}
