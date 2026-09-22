package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
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

// acceptInviteNewCredentialHeading/acceptInviteExistingCredentialHeading are showAcceptInvite/
// submitAcceptInvite's two headings -- which one applies depends on whether the invited email
// already holds a credential elsewhere (security audit 2026-09-19, H1 follow-up: CAP-O10's own
// reference shape distinguishes "create your account" from "confirm the one you already have",
// rather than treating an existing credential as an error).
const (
	acceptInviteNewCredentialHeading      = "Create your Menata account to join"
	acceptInviteExistingCredentialHeading = "Confirm your password to join this workspace"
)

// showAcceptInvite renders the join form, deciding first which of the two shapes applies: a brand
// new identity states its own full name and sets a password, while one that already exists only
// confirms the password it already has. The email a valid token names is safe to look up here (it
// isn't user input at this point -- VerifyInviteToken produced it).
func showAcceptInvite(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := req.URL.Query().Get("token")
		heading, needsName := acceptInviteNewCredentialHeading, true
		if email, _, ok := authorization.VerifyInviteToken(cfg.SessionSecret, token); ok {
			if _, err := store.GetCredential(req.Context(), email); err == nil {
				heading, needsName = acceptInviteExistingCredentialHeading, false
			}
		}
		render(req.Context(), w, rendering.AcceptInvitePage(token, "", heading, needsName))
	}
}

// submitAcceptInvite is where a membership begins (owner decision, 2026-09-22). Until this runs,
// the invitation is a row in pending_invites and nothing else: no mch_user record, no membership,
// no role, nothing any list or authorization check can see.
//
// It is also the one place an invited person's credential is created, or an existing one confirmed
// (security audit 2026-09-19, H1 + its own CAP-O10 follow-up) -- closing the gap authenticateMember
// used to leave open, where the first successful login attempt for an invited email created a
// credential from whatever password was POSTed, regardless of who sent it.
//
// A brand-new identity states its own full name here, and it goes onto the credential rather than
// into any Workspace's record (migration 010): this is the person's profile across every Menata
// Workspace, so it is stated once, by its owner, and never again by an admin on their behalf.
func submitAcceptInvite(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		token := req.FormValue("token")

		email, workspaceID, ok := authorization.VerifyInviteToken(cfg.SessionSecret, token)
		if !ok {
			http.Error(w, "invalid or expired invite link", http.StatusBadRequest)
			return
		}
		// The bound workspaceID must still name a live invitation -- not just "some token was once
		// signed for this pair". This is what actually uses the binding NewInviteToken puts in the
		// token (rather than just carrying email): it re-confirms the invitation has not since been
		// revoked or already accepted, at the moment it is used rather than when it was sent.
		invite, err := store.GetPendingInvite(ctx, workspaceID, email)
		if err != nil {
			if errors.Is(err, data.ErrPendingInviteNotFound) {
				http.Error(w, "invalid or expired invite link", http.StatusBadRequest)
				return
			}
			serverError(w, err)
			return
		}

		heading, needsName, errMsg, err := confirmInviteIdentity(ctx, store, email,
			strings.TrimSpace(req.FormValue("full_name")), req.FormValue("password"))
		if err != nil {
			serverError(w, err)
			return
		}
		if errMsg != "" {
			render(ctx, w, rendering.AcceptInvitePage(token, errMsg, heading, needsName))
			return
		}

		if err := admitInvitedMember(ctx, machines, store, *invite); err != nil {
			serverError(w, err)
			return
		}
		if err := completeLogin(w, req, cfg, store, email); err != nil {
			serverError(w, err)
		}
	}
}

// confirmInviteIdentity settles who is accepting, before any membership is written: it either
// creates the identity from the name and password typed on the form, or confirms the password of
// one that already exists. errMsg is a message for the form (missing name, short password, wrong
// password) and comes back with the heading/shape the re-render needs; a non-nil error is a real
// failure for serverError.
//
// Split out of submitAcceptInvite to keep that handler inside internal/conformance's
// TestHandlersStaySmall budget, the same reason applyPasswordChange (account.go) exists.
func confirmInviteIdentity(ctx context.Context, store *data.Store, email, fullName, password string) (heading string, needsName bool, errMsg string, err error) {
	cred, err := store.GetCredential(ctx, email)
	switch {
	case errors.Is(err, data.ErrCredentialNotFound):
		// No credential yet -- the original H1 fix's own path: create the identity. The invite's
		// own admin already vouched for this email by typing it in when inviting, the same trust
		// rationale authenticateMember's replaced auto-activation relied on, now only reachable
		// through a verified token instead of "whoever POSTs first".
		//
		// This is where a brand-new person states their own full name, and it is written onto the
		// credential rather than into any Workspace's record (migration 010): one profile, used by
		// every Workspace they ever join, changeable only by them.
		switch {
		case fullName == "":
			return acceptInviteNewCredentialHeading, true, "Your full name is required.", nil
		case len(password) < 8:
			return acceptInviteNewCredentialHeading, true, "Password must be at least 8 characters.", nil
		}
		hash, hashErr := authorization.HashPassword(password)
		if hashErr != nil {
			return "", false, "", hashErr
		}
		if err := store.CreateCredential(ctx, email, fullName, hash, true); err != nil {
			return "", false, "", err
		}
		return acceptInviteNewCredentialHeading, true, "", nil
	case err != nil:
		return "", false, "", err
	default:
		// A credential already exists -- this email registered separately or was invited elsewhere
		// first. Confirming they own it (not creating a second one) is CAP-O10's own reference
		// shape, and their name is not asked for again: it is already on the identity this
		// password is about to prove ownership of.
		if !authorization.VerifyPassword(password, cred.PasswordHash) {
			return acceptInviteExistingCredentialHeading, false, "Incorrect password.", nil
		}
		return acceptInviteExistingCredentialHeading, false, "", nil
	}
}

// admitInvitedMember turns an accepted invitation into real membership: the Workspace's own
// mch_user record for this person, the membership row naming it, that Workspace's per-Application
// roles, and finally the invitation's own removal -- deleted last, so a failure part-way leaves
// the invitation live and retryable rather than consumed.
//
// The record it creates carries only Workspace-scoped values (metadata/user.yaml): the person's
// name and email are on their identity, which by this point is guaranteed to exist -- either it
// already did, or submitAcceptInvite just created it from the name they typed.
//
// Split out of the handler rather than inlined to keep it inside internal/conformance's
// TestHandlersStaySmall budget, the same reason applyPasswordChange (account.go) exists.
func admitInvitedMember(ctx context.Context, machines map[string]*domain.Machine, store *data.Store, invite data.PendingInvite) error {
	scoped := data.WithWorkspaceScope(ctx, invite.WorkspaceID)

	userMachine := machines[domain.UserMachineID]
	values := map[string]any{}
	data.ApplyDefaults(userMachine, values)
	if err := data.ValidateRecord(userMachine, values); err != nil {
		return err
	}
	user, err := store.CreateRecord(scoped, domain.UserMachineID, values)
	if err != nil {
		return err
	}
	if err := store.AddMember(ctx, invite.WorkspaceID, user.ID, invite.Email, invite.WorkspaceRole, invite.AppRoles[legacyAppRoleApplicationID]); err != nil {
		return err
	}
	for appID, role := range invite.AppRoles {
		if err := store.SetMemberAppRole(ctx, invite.WorkspaceID, user.ID, appID, role); err != nil {
			return err
		}
	}
	return store.DeletePendingInvite(ctx, invite.WorkspaceID, invite.Email)
}
