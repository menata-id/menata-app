package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

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
		navID := chi.URLParam(req, "navID")
		app, ok := rendering.CurrentApplication(ctx)
		if !ok {
			showWorkspacePage(store, cfg, navID).ServeHTTP(w, req)
			return
		}
		for _, item := range app.AllNavigation {
			if item.ID != navID || item.Page == nil {
				continue
			}
			body, err := composition.DeclaredPage(ctx, composition.NewLoader(store, machinesFor(ctx)), currentActor(req, store, cfg), pageParams(req, app.ID), time.Now(), app.AllNavigation, *item.Page)
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

// showWorkspacePage draws a page the Workspace itself declares (`navigation:` on its manifest, `page:` on an item), the
// one that no Application owns: an overview across Applications, say. It is found by id in the Workspace and answers
// 404 for anything else, which includes an Application's page named with no Application in the request.
//
// **Workspace admins only, for now.** An Application's pages are gated by its roles (`requireApplicationAccess`); this
// page belongs to none, and what it reads is the Datasets of Machines that do. Until a page can say who sees it
// (`when:` per viewer) the only honest gate is the one the Workspace already has for its own screens, so a member with no
// admin role gets the 403 a settings screen gives, rather than figures from an Application they hold no role in.
func showWorkspacePage(store *data.Store, cfg config.Config, navID string) http.Handler {
	return requireWorkspaceAdmin(store, cfg)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		ws := rendering.CurrentWorkspace(ctx)
		for _, item := range ws.Navigation {
			if item.ID != navID || item.Page == nil {
				continue
			}
			body, err := composition.DeclaredPage(ctx, composition.NewLoader(store, machinesFor(ctx)), currentActor(req, store, cfg), pageParams(req, ""), time.Now(), ws.LinkTargets(), *item.Page)
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
	}))
}

// queryParams is the request's query string as one value per name (the first, as net/http's Get does).
func queryParams(req *http.Request) map[string]string {
	q := req.URL.Query()
	out := make(map[string]string, len(q))
	for k := range q {
		out[k] = q.Get(k)
	}
	return out
}

// pageParams is queryParams plus the one name the runtime owns: `application`, the Application the page is drawn
// in. It is set last so a query string cannot supply it -- `?application=app_other` would otherwise read another
// Application's events through a Dataset that filters on it (ds_recent_activity).
func pageParams(req *http.Request, applicationID string) map[string]string {
	out := queryParams(req)
	out["application"] = applicationID
	return out
}
