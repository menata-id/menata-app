package web

import (
	"fmt"
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showNotifications is Tahap 6's own worklist (Flow 2 gap study): every mch_notification this
// identity is the recipient of, newest first (composition.MyNotifications) -- a Workspace-level
// runtime route (names no Application, same category as /dashboard and /account-profile),
// reachable by any authenticated member regardless of which Applications they have access to.
func showNotifications(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		rows, err := composition.MyNotifications(ctx, composition.NewLoader(store, machines), userID)
		if err != nil {
			serverError(w, err)
			return
		}
		workspaceName, viewer, switchHref, err := pageChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.NotificationsPage(rows, workspaceName, viewer, switchHref))
	}
}

// showUnreadNotificationCount is the bell badge's own endpoint -- mirrors showPendingCount
// exactly: a bare integer, empty body when zero, meant for hx-get/hx-trigger=load.
func showUnreadNotificationCount(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		count, err := composition.UnreadNotificationCount(req.Context(), composition.NewLoader(store, machines), userID)
		if err != nil {
			serverError(w, err)
			return
		}
		if count == 0 {
			return
		}
		_, _ = fmt.Fprintf(w, "%d", count)
	}
}

// submitMarkAllNotificationsRead marks every one of the viewer's own unread notifications read, in
// one action -- a dedicated bulk write rather than a per-row PUT through the generic edit route:
// the generic route replaces a record's whole data column from whatever the form submits
// (data.ValuesFromForm), so a minimal "just fld_read" form would silently wipe fld_recipient/
// fld_message/fld_link (the exact carry-forward hazard signatureplacement.templ's own composed
// placement view was built to avoid). Reading and rewriting each record's own full Values here,
// in Go, has no such hazard.
func submitMarkAllNotificationsRead(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		records, err := store.ListRecordsBy(ctx, "mch_notification", "fld_recipient", userID)
		if err != nil {
			serverError(w, err)
			return
		}
		for _, r := range records {
			if r.Values["fld_read"] == "read" {
				continue
			}
			r.Values["fld_read"] = "read"
			if _, err := store.UpdateRecord(ctx, "mch_notification", r.ID, r.Values); err != nil {
				serverError(w, err)
				return
			}
		}
		redirectTo(w, req, "/notifications")
	}
}
