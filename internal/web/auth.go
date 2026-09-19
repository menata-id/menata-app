package web

import (
	"context"
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
// An invited email (ROADMAP.md Phase 21 Step 6) has a membership row but no credential yet --
// until security audit 2026-09-19's H1 fix, the first successful "login" attempt activated the
// account by setting whatever password was POSTed as its real credential, which let anyone who
// knew or guessed the invited email claim it before the real invitee ever logged in. A credential
// can now only be created via a verified, workspace-bound invite token (submitAcceptInvite,
// invite.go) or self-registration (submitRegistration) -- never here, so an email with a
// membership but no credential yet is simply rejected, the same as a wrong password.
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
		render(req.Context(), w, rendering.ChooseWorkspacePage(choices, "", "/choose-workspace", "/login", "Sign out"))
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
		userRecordID, ok, err := resolveWorkspaceMembership(req.Context(), store, email, req.FormValue("workspace_id"))
		if err != nil {
			serverError(w, err)
			return
		}
		if !ok {
			http.Error(w, "not a member of that workspace", http.StatusForbidden)
			return
		}
		authorization.ClearPendingEmailCookie(w, cfg.SecureCookies)
		authorization.SetSessionCookie(w, cfg.SessionSecret, userRecordID, cfg.SecureCookies)
		redirectTo(w, req, "/home")
	}
}

// showSwitchWorkspace is the mid-session counterpart to showChooseWorkspace -- reached from the
// Workspace Home eyebrow's switch icon (WorkspaceHomePage's switchHref) once a session already
// exists, rather than from the pending-email cookie a fresh login leaves. Redirects home rather
// than erroring when there's nothing to switch to (no membership row, or only this one Workspace)
// since that's this handler being reached by a stale link, not a real failure.
func showSwitchWorkspace(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			redirectTo(w, req, "/home")
			return
		}
		choices, err := loadWorkspaceChoices(ctx, store, email)
		if err != nil {
			serverError(w, err)
			return
		}
		if len(choices) < 2 {
			redirectTo(w, req, "/home")
			return
		}
		render(ctx, w, rendering.ChooseWorkspacePage(choices, "", "/switch-workspace", "/home", "Back to Menata"))
	}
}

// submitSwitchWorkspace re-checks the posted choice against the signed-in identity's own
// memberships the same way submitChooseWorkspace does, and simply re-points the session cookie
// at the chosen Workspace's mch_user record -- no pending-email cookie is involved mid-session.
func submitSwitchWorkspace(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			redirectTo(w, req, "/home")
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		userRecordID, ok, err := resolveWorkspaceMembership(ctx, store, email, req.FormValue("workspace_id"))
		if err != nil {
			serverError(w, err)
			return
		}
		if !ok {
			http.Error(w, "not a member of that workspace", http.StatusForbidden)
			return
		}
		authorization.SetSessionCookie(w, cfg.SessionSecret, userRecordID, cfg.SecureCookies)
		redirectTo(w, req, "/home")
	}
}

// currentUserEmail resolves the signed-in session to the email loadWorkspaceChoices needs to
// switch by -- read off the current Workspace's own membership row, since that's the one place
// session identity and email already meet (showWorkspaceHome does the same lookup). Missing for
// the shared admin credential's placeholder identity (predates real Workspace membership), which
// this treats as "nothing to switch between" rather than an error.
func currentUserEmail(ctx context.Context, store *data.Store, req *http.Request, cfg config.Config) (string, bool) {
	userID, ok := authorization.CurrentUserID(req, cfg.SessionSecret)
	if !ok {
		return "", false
	}
	workspaceID, ok := data.WorkspaceScope(ctx)
	if !ok {
		return "", false
	}
	membership, err := store.GetMembership(ctx, workspaceID, userID)
	if err != nil || membership == nil || membership.Email == "" {
		return "", false
	}
	return membership.Email, true
}

// resolveWorkspaceMembership finds which of email's memberships names workspaceID, returning the
// mch_user record id to sign in as -- shared by submitChooseWorkspace (pre-session) and
// submitSwitchWorkspace (mid-session), which otherwise differ only in where email and the session
// cookie come from.
func resolveWorkspaceMembership(ctx context.Context, store *data.Store, email, workspaceID string) (userRecordID string, ok bool, err error) {
	memberships, err := store.ListMemberships(ctx, email)
	if err != nil {
		return "", false, err
	}
	for _, m := range memberships {
		if m.WorkspaceID == workspaceID {
			return m.UserRecordID, true, nil
		}
	}
	return "", false, nil
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
