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

// sendInviteEmail builds and sends (or, via mail.LogMailer, logs) email's invite-acceptance link,
// binding workspaceID into the token itself (security audit 2026-09-19, H1) -- mirrors
// sendVerificationEmail's shape (verification.go), a 7-day validity since an invite commonly sits
// unread over a weekend, longer than the other two tokens' shorter-lived, more time-sensitive
// flows.
func sendInviteEmail(ctx context.Context, mailer mail.Mailer, cfg config.Config, email, workspaceID string) {
	token := authorization.NewInviteToken(cfg.SessionSecret, email, workspaceID)
	link := cfg.AppBaseURL + "/accept-invite?token=" + url.QueryEscape(token)
	body := fmt.Sprintf("You've been invited to join a Menata App workspace. Set your password to accept (link valid 7 days):\n\n%s\n\nIf you weren't expecting this, you can ignore this email.", link)
	if err := mailer.Send(ctx, email, "You've been invited - Menata App", body); err != nil {
		log.Printf("failed to send invite email to %s: %v", email, err)
	}
}

// showAcceptInvite renders the same token-carrying "set a password" form showResetPassword uses
// (rendering.ResetPasswordPage), just with invite-specific copy and its own action -- an invited
// member never had a password to reset, only one to set for the first time.
func showAcceptInvite(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.ResetPasswordPage(req.URL.Query().Get("token"), "", "Set your password to join the workspace", "/accept-invite", "Join workspace"))
}

// submitAcceptInvite is the one place an invited member's credential is actually created (security
// audit 2026-09-19, H1) -- closing the gap authenticateMember used to leave open, where the first
// successful login attempt for an invited email created a credential from whatever password was
// POSTed, regardless of who sent it. A credential can only be created here, gated by a valid,
// unexpired, not-yet-used invite token.
func submitAcceptInvite(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		token := req.FormValue("token")
		password := req.FormValue("password")

		email, workspaceID, ok := authorization.VerifyInviteToken(cfg.SessionSecret, token)
		if !ok {
			http.Error(w, "invalid or expired invite link", http.StatusBadRequest)
			return
		}
		// The bound workspaceID must still name a real, current membership of email -- not just
		// "some token was once signed for this pair". This is what actually uses the binding
		// NewInviteToken puts in the token (rather than just carrying email): it re-confirms the
		// invite is still live (e.g. not since removed by an admin) at the moment it's accepted,
		// not only at the moment it was sent.
		if _, ok, err := resolveWorkspaceMembership(req.Context(), store, email, workspaceID); err != nil {
			serverError(w, err)
			return
		} else if !ok {
			http.Error(w, "invalid or expired invite link", http.StatusBadRequest)
			return
		}
		if len(password) < 8 {
			render(req.Context(), w, rendering.ResetPasswordPage(token, "Password must be at least 8 characters.", "Set your password to join the workspace", "/accept-invite", "Join workspace"))
			return
		}

		// A credential already existing means this invite was already accepted (or the email
		// separately registered/invited elsewhere first) -- the same generic rejection either way,
		// so a token can't be used to probe which case it is.
		if _, err := store.GetCredential(req.Context(), email); err == nil {
			http.Error(w, "invalid or expired invite link", http.StatusBadRequest)
			return
		} else if !errors.Is(err, data.ErrCredentialNotFound) {
			serverError(w, err)
			return
		}

		hash, err := authorization.HashPassword(password)
		if err != nil {
			serverError(w, err)
			return
		}
		// The invite's own admin already vouched for this email by typing it in when inviting --
		// same trust rationale authenticateMember's replaced auto-activation used to rely on, now
		// only reachable through a verified token instead of "whoever POSTs first".
		if err := store.CreateCredential(req.Context(), email, hash, true); err != nil {
			serverError(w, err)
			return
		}

		if err := completeLogin(w, req, cfg, store, email); err != nil {
			serverError(w, err)
		}
	}
}
