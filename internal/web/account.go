package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
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

// submitProfile writes the signed-in identity's own full name -- and since 2026-09-22 it writes it
// to the identity itself (migration 010), so one save changes it in every Workspace at once rather
// than only in whichever one the viewer happened to be looking at.
//
// That is what makes this screen's placement honest. It sits in the Account menu beside Security,
// which has always written the credential; until the name moved, Profile looked identity-level and
// silently edited one Workspace's record instead.
//
// The email comes from the session's own membership (currentUserEmail), never from a form or URL
// parameter, so there is no way to reach another identity's name through this handler -- the same
// property that makes it safe without a new Permission (see rendering.ProfilePage's doc comment),
// and the enforcement of the owner's rule that a full name may only be changed by the person who
// owns it.
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

		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			http.Error(w, "no workspace membership for this identity", http.StatusForbidden)
			return
		}
		if err := store.SetFullName(ctx, email, name); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/account-profile")
	}
}

// showAccountNotifications renders the signed-in identity's own email-notification preferences
// (Flow 2 gap study Tahap 6, ports ui-sample/account-notifications.html's two real rows) -- an
// identity-level credential, the same "Account" placement Profile/Security already establish.
//
// credentialFor, not a second store.GetCredential: resolveChrome's own identity already read this
// exact row (for the display name), and a second read here was the first violation
// TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed ever caught on this route. A missing credential
// (the shared admin credential's placeholder subject, which "predates Workspace membership
// entirely and has no row" -- requireWorkspaceAdmin's own doc comment) degrades to the column
// defaults (true/true) rather than a 500.
func showAccountNotifications(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		cred, err := credentialFor(ctx, store, chrome.Email)
		if err != nil {
			serverError(w, err)
			return
		}
		notifyAssigned, notifyDecided := true, true
		if cred != nil {
			notifyAssigned, notifyDecided = cred.NotifyAssigned, cred.NotifyDecided
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.AccountNotificationsPage(notifyAssigned, notifyDecided, chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}

// submitAccountNotifications saves the two toggles -- a plain checkbox pair, so an unchecked box
// simply never appears in the posted form (the same reason data.ValuesFromForm treats a boolean
// Field this way elsewhere): form.Has, not form.Get, is what tells "off" apart from "missing".
func submitAccountNotifications(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		email, ok := currentUserEmail(ctx, store, req, cfg)
		if !ok {
			http.Error(w, "no workspace membership for this identity", http.StatusForbidden)
			return
		}
		assigned := req.Form.Has("notify_assigned")
		decided := req.Form.Has("notify_decided")
		if err := store.UpdateNotificationPreferences(ctx, email, assigned, decided); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/account-notifications")
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
