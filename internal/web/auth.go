package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

func showLogin(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.LoginPage(""))
}

// submitLogin tries the shared admin credential first (unchanged since Phase 2, kept as a
// bootstrap fallback per ROADMAP.md Phase 21's own design pass), then a real per-user credential
// (Phase 21 Step 4).
func submitLogin(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		username := req.FormValue("username")
		password := req.FormValue("password")

		if authorization.CheckCredentials(username, password, cfg.AdminUsername, cfg.AdminPassword) {
			authorization.SetSessionCookie(w, cfg.SessionSecret, cfg.AdminUserID, cfg.SecureCookies)
			redirectTo(w, req, "/home")
			return
		}

		email := normalizeEmail(username)
		switch authenticateMember(req.Context(), store, email, password) {
		case loginNeedsVerification:
			w.WriteHeader(http.StatusUnauthorized)
			render(req.Context(), w, rendering.LoginPage("Please verify your email before signing in -- check your inbox, or resend the link below."))
			return
		case loginRejected:
			w.WriteHeader(http.StatusUnauthorized)
			render(req.Context(), w, rendering.LoginPage("Invalid username or password"))
			return
		}
		if err := completeLogin(w, req, cfg, store, email); err != nil {
			serverError(w, err)
		}
	}
}

type loginOutcome int

const (
	loginRejected loginOutcome = iota
	loginOK
	loginNeedsVerification
)

// authenticateMember verifies email/password against a real per-user credential.
//
// An invited email (ROADMAP.md Phase 21 Step 6) has a membership row but no credential yet -- this
// app has no outbound-email infrastructure to drive a token-based invite flow, so the first
// successful "login" attempt activates the account by setting the submitted password as its real
// credential, rather than verifying one that was never issued. That activated credential starts
// EmailVerified=true immediately (a Workspace Admin already vouched for this specific email by
// typing it in themselves, a different trust model than self-registration) -- named as a
// deliberate simplification, not an oversight.
//
// A self-registered credential (round 2, Step D) starts EmailVerified=false and stays rejected
// (loginNeedsVerification, a distinct outcome from a wrong password) until its own emailed link is
// clicked.
func authenticateMember(ctx context.Context, store *data.Store, email, password string) loginOutcome {
	memberships, err := store.ListMemberships(ctx, email)
	if err != nil || len(memberships) == 0 {
		return loginRejected
	}

	cred, err := store.GetCredential(ctx, email)
	switch {
	case errors.Is(err, data.ErrCredentialNotFound):
		if len(password) < 8 {
			return loginRejected
		}
		newHash, hashErr := authorization.HashPassword(password)
		if hashErr != nil {
			return loginRejected
		}
		if err := store.CreateCredential(ctx, email, newHash, true); err != nil {
			return loginRejected
		}
		return loginOK
	case err != nil:
		return loginRejected
	case !authorization.VerifyPassword(password, cred.PasswordHash):
		return loginRejected
	case !cred.EmailVerified:
		return loginNeedsVerification
	}
	return loginOK
}

// completeLogin signs an already-authenticated email into whichever Workspace(s) it belongs to --
// exactly one signs in directly, more than one defers to Choose Workspace. Shared by submitLogin's
// own per-user path, /verify-email, and Step E's /reset-password: each ends the same way once an
// email is allowed in, so this is the one place that logic lives.
func completeLogin(w http.ResponseWriter, req *http.Request, cfg config.Config, store *data.Store, email string) error {
	memberships, err := store.ListMemberships(req.Context(), email)
	if err != nil {
		return err
	}
	switch len(memberships) {
	case 0:
		http.Error(w, "no workspace membership found for this account", http.StatusForbidden)
	case 1:
		authorization.SetSessionCookie(w, cfg.SessionSecret, memberships[0].UserRecordID, cfg.SecureCookies)
		redirectTo(w, req, "/home")
	default:
		authorization.SetPendingEmailCookie(w, cfg.SessionSecret, email, cfg.SecureCookies)
		redirectTo(w, req, "/choose-workspace")
	}
	return nil
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// showChooseWorkspace lists every Workspace the pending login's email belongs to (ROADMAP.md
// Phase 21 Step 4). Reached only via submitLogin's own redirect -- a request with no valid pending
// cookie has nothing to choose between and goes back to /login.
func showChooseWorkspace(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		email, ok := authorization.PendingEmail(req, cfg.SessionSecret)
		if !ok {
			http.Redirect(w, req, "/login", http.StatusSeeOther)
			return
		}
		choices, err := loadWorkspaceChoices(req.Context(), store, email)
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.ChooseWorkspacePage(choices, ""))
	}
}

// submitChooseWorkspace completes login once a Workspace is picked -- re-checking the choice
// against the pending email's own memberships rather than trusting the posted workspace_id
// directly, so a tampered value can't sign someone into a Workspace they don't belong to.
func submitChooseWorkspace(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		email, ok := authorization.PendingEmail(req, cfg.SessionSecret)
		if !ok {
			http.Redirect(w, req, "/login", http.StatusSeeOther)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		workspaceID := req.FormValue("workspace_id")

		memberships, err := store.ListMemberships(req.Context(), email)
		if err != nil {
			serverError(w, err)
			return
		}
		for _, m := range memberships {
			if m.WorkspaceID == workspaceID {
				authorization.ClearPendingEmailCookie(w, cfg.SecureCookies)
				authorization.SetSessionCookie(w, cfg.SessionSecret, m.UserRecordID, cfg.SecureCookies)
				redirectTo(w, req, "/home")
				return
			}
		}
		http.Error(w, "not a member of that workspace", http.StatusForbidden)
	}
}

func loadWorkspaceChoices(ctx context.Context, store *data.Store, email string) ([]rendering.WorkspaceChoice, error) {
	memberships, err := store.ListMemberships(ctx, email)
	if err != nil {
		return nil, err
	}
	choices := make([]rendering.WorkspaceChoice, 0, len(memberships))
	for _, m := range memberships {
		ws, err := store.GetWorkspace(ctx, m.WorkspaceID)
		if err != nil {
			return nil, err
		}
		choices = append(choices, rendering.WorkspaceChoice{ID: ws.ID, Name: ws.Name, Role: m.WorkspaceRole})
	}
	return choices, nil
}

func logout(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		authorization.ClearSessionCookie(w, cfg.SecureCookies)
		http.Redirect(w, req, "/login", http.StatusSeeOther)
	}
}
