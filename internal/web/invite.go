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

// acceptInviteNewCredentialCopy/acceptInviteExistingCredentialCopy are showAcceptInvite/
// submitAcceptInvite's two heading/button pairs -- which one applies depends on whether the
// invited email already holds a credential elsewhere (security audit 2026-09-19, H1 follow-up:
// CAP-O10's own reference shape distinguishes "set a new password" from "confirm the one you
// already have", rather than treating an existing credential as an error).
const (
	acceptInviteNewCredentialHeading      = "Set your password to join the workspace"
	acceptInviteExistingCredentialHeading = "Enter your existing password to join this workspace"
	acceptInviteButtonLabel               = "Join workspace"
)

// showAcceptInvite renders the same token-carrying "set a password" form showResetPassword uses
// (rendering.ResetPasswordPage), just with invite-specific copy and its own action. It also
// decides, before the form is even shown, which of the two headings above applies -- the email a
// valid token names is safe to look up here (it isn't user input at this point, VerifyInviteToken
// already produced it), so an already-registered invitee never sees the "set a new password"
// copy that used to be the only option.
func showAcceptInvite(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := req.URL.Query().Get("token")
		heading := acceptInviteNewCredentialHeading
		if email, _, ok := authorization.VerifyInviteToken(cfg.SessionSecret, token); ok {
			if _, err := store.GetCredential(req.Context(), email); err == nil {
				heading = acceptInviteExistingCredentialHeading
			}
		}
		render(req.Context(), w, rendering.ResetPasswordPage(token, "", heading, "/accept-invite", acceptInviteButtonLabel))
	}
}

// submitAcceptInvite is the one place an invited member's credential is created, or an existing
// one confirmed (security audit 2026-09-19, H1 + its own CAP-O10 follow-up) -- closing the gap
// authenticateMember used to leave open, where the first successful login attempt for an invited
// email created a credential from whatever password was POSTed, regardless of who sent it. Gated
// on a valid, unexpired, still-membership-backed invite token in both branches below.
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

		cred, err := store.GetCredential(req.Context(), email)
		switch {
		case errors.Is(err, data.ErrCredentialNotFound):
			// No credential yet -- the original H1 fix's own path: set a brand new one. The
			// invite's own admin already vouched for this email by typing it in when inviting,
			// same trust rationale authenticateMember's replaced auto-activation used to rely on,
			// now only reachable through a verified token instead of "whoever POSTs first".
			if len(password) < 8 {
				render(req.Context(), w, rendering.ResetPasswordPage(token, "Password must be at least 8 characters.", acceptInviteNewCredentialHeading, "/accept-invite", acceptInviteButtonLabel))
				return
			}
			hash, hashErr := authorization.HashPassword(password)
			if hashErr != nil {
				serverError(w, hashErr)
				return
			}
			if err := store.CreateCredential(req.Context(), email, hash, true); err != nil {
				serverError(w, err)
				return
			}
		case err != nil:
			serverError(w, err)
			return
		default:
			// A credential already exists -- this email registered separately or was already
			// invited elsewhere first. Confirming they own it (not creating a second one) is
			// CAP-O10's own reference shape for this case; submitInviteMember already created this
			// Workspace's membership row regardless of credential state, so nothing else needs
			// writing once the password checks out.
			if !authorization.VerifyPassword(password, cred.PasswordHash) {
				render(req.Context(), w, rendering.ResetPasswordPage(token, "Incorrect password.", acceptInviteExistingCredentialHeading, "/accept-invite", acceptInviteButtonLabel))
				return
			}
		}

		if err := completeLogin(w, req, cfg, store, email); err != nil {
			serverError(w, err)
		}
	}
}
