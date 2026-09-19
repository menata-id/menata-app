package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

func showRegistration(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.RegistrationPage(""))
}

// submitRegistration is login.html's "Create a workspace" flow (ROADMAP.md Phase 21 Step 3):
// registration *is* Workspace creation, not a separate signup into an existing one -- there is no
// path here to a bare user account with nowhere to go.
func submitRegistration(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		workspaceName := strings.TrimSpace(req.FormValue("workspace_name"))
		email := normalizeEmail(req.FormValue("fld_email"))
		password := req.FormValue("password")

		if msg, ok := validateRegistration(workspaceName, email, password); !ok {
			rejectRegistration(w, req, msg)
			return
		}
		if _, err := store.GetCredential(req.Context(), email); err == nil {
			rejectRegistration(w, req, "An account with that email already exists.")
			return
		} else if !errors.Is(err, data.ErrCredentialNotFound) {
			serverError(w, err)
			return
		}

		userMachine := machines[domain.UserMachineID]
		values := data.ValuesFromForm(userMachine, req.Form)
		values["fld_email"] = email
		data.ApplyDefaults(userMachine, values)
		if err := data.ValidateRecord(userMachine, values); err != nil {
			rejectRegistration(w, req, err.Error())
			return
		}

		userID, err := registerWorkspace(req.Context(), store, workspaceName, email, password, values)
		if err != nil {
			serverError(w, err)
			return
		}

		authorization.SetSessionCookie(w, cfg.SessionSecret, userID, cfg.SecureCookies)
		redirectTo(w, req, "/")
	}
}

func rejectRegistration(w http.ResponseWriter, req *http.Request, msg string) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	render(req.Context(), w, rendering.RegistrationPage(msg))
}

func validateRegistration(workspaceName, email, password string) (string, bool) {
	switch {
	case workspaceName == "":
		return "Workspace name is required.", false
	case email == "":
		return "Email is required.", false
	case len(password) < 8:
		return "Password must be at least 8 characters.", false
	}
	return "", true
}

// registerWorkspace creates the new Workspace, its first mch_user record, the login credential,
// and the admin membership joining them -- one registration, four inserts (ROADMAP.md Phase 21
// Step 3). Not wrapped in a transaction: a failure partway through is an operational anomaly to
// clean up by hand (this app has no real users yet to affect), not a case an actual retry-safe
// flow is forced by yet -- the same posture logActivity's own best-effort writes already take.
func registerWorkspace(ctx context.Context, store *data.Store, workspaceName, email, password string, userValues map[string]any) (string, error) {
	hash, err := authorization.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	ws, err := store.CreateWorkspace(ctx, workspaceName, slugify(workspaceName))
	if err != nil {
		return "", err
	}
	user, err := store.CreateRecord(data.WithWorkspaceScope(ctx, ws.ID), domain.UserMachineID, userValues)
	if err != nil {
		return "", err
	}
	if err := store.CreateCredential(ctx, email, hash); err != nil {
		return "", err
	}
	if err := store.AddMember(ctx, ws.ID, user.ID, email, "admin", ""); err != nil {
		return "", err
	}
	return user.ID, nil
}

// slugify turns a Workspace name into a URL/display-friendly slug: lowercase alphanumerics joined
// by single hyphens, never leading/trailing/doubled. CreateWorkspace handles collisions, so this
// only needs to produce a reasonable starting point, not a guaranteed-unique one.
func slugify(name string) string {
	var b strings.Builder
	lastWasHyphen := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasHyphen = false
		case !lastWasHyphen:
			b.WriteByte('-')
			lastWasHyphen = true
		}
	}
	slug := strings.TrimSuffix(b.String(), "-")
	if slug == "" {
		return "workspace"
	}
	return slug
}
