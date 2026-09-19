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
// (Phase 21 Step 4). A real login naming more than one Workspace membership defers to Choose
// Workspace rather than picking one; naming exactly one signs straight in.
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
			redirectTo(w, req, "/")
			return
		}

		userID, chooseWorkspace, ok := authenticateMember(req.Context(), store, username, password)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			render(req.Context(), w, rendering.LoginPage("Invalid username or password"))
			return
		}
		if chooseWorkspace {
			authorization.SetPendingEmailCookie(w, cfg.SessionSecret, normalizeEmail(username), cfg.SecureCookies)
			redirectTo(w, req, "/choose-workspace")
			return
		}
		authorization.SetSessionCookie(w, cfg.SessionSecret, userID, cfg.SecureCookies)
		redirectTo(w, req, "/")
	}
}

// authenticateMember verifies username/password against a real per-user credential. Exactly one
// Workspace membership returns that membership's mch_user record id to sign in as directly; more
// than one defers the choice to Choose Workspace (chooseWorkspace=true, userID="").
func authenticateMember(ctx context.Context, store *data.Store, username, password string) (userID string, chooseWorkspace bool, ok bool) {
	email := normalizeEmail(username)
	hash, err := store.GetCredential(ctx, email)
	if err != nil || !authorization.VerifyPassword(password, hash) {
		return "", false, false
	}
	memberships, err := store.ListMemberships(ctx, email)
	if err != nil || len(memberships) == 0 {
		return "", false, false
	}
	if len(memberships) > 1 {
		return "", true, true
	}
	return memberships[0].UserRecordID, false, true
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
				redirectTo(w, req, "/")
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
