package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showProfile renders the signed-in identity's own Profile page. Name/Email come from
// resolveChrome, which already loads this same mch_user record for the Account menu -- no extra
// query.
func showProfile(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.ProfilePage(chrome.Name, chrome.Email, chrome.WorkspaceName, chrome.Viewer(), switchHref, ""))
	}
}

// submitProfile writes the signed-in identity's own display name. userID comes only from the
// session (authorization.CurrentUserID), never from a URL or form parameter -- there is no way to
// reach another user's record through this handler, which is what makes it safe without a new
// Permission (see rendering.ProfilePage's own doc comment).
//
// UpdateRecord replaces a record's whole values, not a partial merge (internal/data/store.go's
// UPDATE ... SET data = $3::jsonb), so this reads the current record first and writes it back
// with only fld_name changed -- a bare map[string]any{"fld_name": name} would silently drop
// fld_email and fld_weekly_capacity.
func submitProfile(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.FormValue("name"))
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		if name == "" {
			chrome, err := resolveChrome(ctx, req, store, cfg)
			if err != nil {
				serverError(w, err)
				return
			}
			_, switchHref := viewerWorkspaceContext(ctx, store, userID)
			render(ctx, w, rendering.ProfilePage(name, chrome.Email, chrome.WorkspaceName, chrome.Viewer(), switchHref, "Full name is required."))
			return
		}

		record, err := store.GetRecord(ctx, domain.UserMachineID, userID)
		if err != nil {
			recordError(w, err)
			return
		}
		record.Values["fld_name"] = name
		if _, err := store.UpdateRecord(ctx, domain.UserMachineID, userID, record.Values); err != nil {
			recordError(w, err)
			return
		}
		redirectTo(w, req, "/account-profile")
	}
}

// showSecurity renders the signed-in identity's own Security page. No extra data beyond chrome --
// see rendering.SecurityPage's own doc comment for why there is no session list to fetch.
func showSecurity(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.SecurityPage(chrome.WorkspaceName, chrome.Viewer(), switchHref, "", "", ""))
	}
}

// securityPageAfter re-renders Security with one of the three result messages set, after a
// change-password or sign-out-other-devices submission -- shared so neither handler repeats the
// chrome/switchHref resolution the plain GET above already does.
func securityPageAfter(store *data.Store, cfg config.Config, req *http.Request, passwordErr, passwordOK, signOutOK string) (templ.Component, error) {
	ctx := req.Context()
	chrome, err := resolveChrome(ctx, req, store, cfg)
	if err != nil {
		return nil, err
	}
	userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
	_, switchHref := viewerWorkspaceContext(ctx, store, userID)
	return rendering.SecurityPage(chrome.WorkspaceName, chrome.Viewer(), switchHref, passwordErr, passwordOK, signOutOK), nil
}

// applyPasswordChange verifies current against email's stored credential, validates newPassword,
// and on success sets it and bumps every session that credential could have signed
// (invalidateSessionsFor, internal/web/auth.go -- the same helper password reset uses). errMsg is
// a validation failure meant for the page (wrong current password, too short, mismatched
// confirmation); a non-nil error is a real failure meant for serverError. Split out of
// submitChangePassword to keep that handler under internal/conformance's TestHandlersStaySmall
// budget -- the same "derivation moves out of the handler" rule its own doc comment states.
func applyPasswordChange(ctx context.Context, store *data.Store, email, current, newPassword, confirm string) (errMsg string, err error) {
	cred, err := store.GetCredential(ctx, email)
	if err != nil {
		return "", err
	}
	switch {
	case !authorization.VerifyPassword(current, cred.PasswordHash):
		return "Current password is incorrect.", nil
	case len(newPassword) < 8:
		return "New password must be at least 8 characters.", nil
	case newPassword != confirm:
		return "New password and confirmation do not match.", nil
	}
	hash, err := authorization.HashPassword(newPassword)
	if err != nil {
		return "", err
	}
	if err := store.SetCredential(ctx, email, hash); err != nil {
		return "", err
	}
	return "", invalidateSessionsFor(ctx, store, email)
}

// submitChangePassword re-issues *this* request's own cookie (setSessionCookieFor) after a
// successful applyPasswordChange, so the acting session survives its own change -- unlike a
// pre-auth reset, which has no session to preserve.
func submitChangePassword(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			http.Error(w, "no workspace membership for this identity", http.StatusForbidden)
			return
		}

		errMsg, err := applyPasswordChange(ctx, store, email,
			req.FormValue("current_password"), req.FormValue("new_password"), req.FormValue("confirm_password"))
		if err != nil {
			serverError(w, err)
			return
		}
		okMsg := "Password updated."
		if errMsg != "" {
			okMsg = ""
		} else if err := setSessionCookieFor(ctx, store, w, cfg, userID); err != nil {
			serverError(w, err)
			return
		}

		page, err := securityPageAfter(store, cfg, req, errMsg, okMsg, "")
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, page)
	}
}

// submitSignOutOtherDevices revokes every session but this one: the same invalidate-then-recookie
// pair submitChangePassword uses, with no password check beyond already being authenticated.
func submitSignOutOtherDevices(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			http.Error(w, "no workspace membership for this identity", http.StatusForbidden)
			return
		}

		if err := invalidateSessionsFor(ctx, store, email); err != nil {
			serverError(w, err)
			return
		}
		if err := setSessionCookieFor(ctx, store, w, cfg, userID); err != nil {
			serverError(w, err)
			return
		}

		page, err := securityPageAfter(store, cfg, req, "", "", "Every other session has been signed out.")
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, page)
	}
}
