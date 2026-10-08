package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// showDeclaredPage renders any navigation item that declares `page:` (007 §12.4). One handler for all of
// them: the item is found by id in the Application the route resolved to, so which Workspace and which
// Application a request is in stays the per-request fact it is everywhere else, and an id that names no
// declared page -- another Application's, or a bespoke screen's -- is a 404 rather than an empty shell.
func showDeclaredPage(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		app, ok := rendering.CurrentApplication(ctx)
		if !ok {
			http.NotFound(w, req)
			return
		}
		navID := chi.URLParam(req, "navID")
		for _, item := range app.AllNavigation {
			if item.ID != navID || item.Page == nil {
				continue
			}
			viewerID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
			body, err := composition.DeclaredPage(ctx, composition.NewLoader(store, machinesFor(ctx)), viewerID, *item.Page)
			if err != nil {
				serverError(w, err)
				return
			}
			workspaceName, viewer, switchHref, err := pageChrome(ctx, req, store, cfg)
			if err != nil {
				serverError(w, err)
				return
			}
			render(ctx, w, rendering.DeclaredScreen(navID, body, workspaceName, viewer, switchHref))
			return
		}
		http.NotFound(w, req)
	}
}
