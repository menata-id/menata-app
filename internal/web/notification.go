package web

import (
	"fmt"
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
)

// showUnreadNotificationCount is the bell badge's own endpoint -- mirrors showPendingCount
// exactly: a bare integer, empty body when zero, meant for hx-get/hx-trigger=load.
func showUnreadNotificationCount(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
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
