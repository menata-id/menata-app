package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
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
			// A Workspace page has no record page: an address with a record is not one of its pages.
			if chi.URLParam(req, "recordID") != "" {
				http.NotFound(w, req)
				return
			}
			showWorkspacePage(store, cfg, navID).ServeHTTP(w, req)
			return
		}
		for _, item := range app.AllNavigation {
			if item.ID != navID || item.Page == nil {
				continue
			}
			params := pageParams(req, app.ID)
			// `id` is the runtime's: a record page's route sets it, and a query string never does.
			delete(params, "id")
			// A record page is opened for one record, which the route names and which must be a record of its Machine in
			// this Workspace; a list page has no record. Either way round is a 404, not an empty page.
			recordID := chi.URLParam(req, "recordID")
			if (item.RecordOf != "") != (recordID != "") {
				http.NotFound(w, req)
				return
			}
			if recordID != "" {
				if _, err := store.GetRecord(ctx, item.RecordOf, recordID); err != nil {
					if errors.Is(err, data.ErrRecordNotFound) {
						http.NotFound(w, req)
						return
					}
					serverError(w, err)
					return
				}
				params["id"] = recordID
			}
			body, err := composition.DeclaredPage(ctx, composition.NewLoader(store, machinesFor(ctx)), currentActor(req, store, cfg), params, time.Now(), app.LinkTargets(rendering.CurrentWorkspace(ctx).Navigation), *item.Page)
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
// **Who may open it is the item's `open_to:`**: Workspace admins unless it says `member`, which admits any member of the
// Workspace. An Application's pages are gated by its roles (`requireApplicationAccess`); this page belongs to none, so the
// choice is the page's own, and the default stays the narrow one -- a page that says nothing is opened by the people who
// administer the Workspace, as it was before the key existed. An id naming no page is judged by the admin gate before the
// 404, so a non-admin cannot tell a missing page from a page that is not theirs.
func showWorkspacePage(store *data.Store, cfg config.Config, navID string) http.Handler {
	return gatedPage(store, cfg, navID, false, "", func(ws domain.Workspace) []domain.NavigationItem { return ws.Navigation })
}

// showRuntimePage draws the page the runtime declares for one of the screens it owns (domain.Workspace.RuntimePages),
// at that screen's own route: the same drawing and the same `open_to:` gate as a Workspace's page, with the page
// found among the runtime's. A Workspace that lacks what the page reads has no such page, and answers 404.
func showRuntimePage(store *data.Store, cfg config.Config, navID string) http.Handler {
	return gatedPage(store, cfg, navID, true, "", func(ws domain.Workspace) []domain.NavigationItem { return ws.RuntimePages })
}

// showRuntimeRecordPage is showRuntimePage for a screen that is one record's page: the route's `recordParam` names the
// record, the page reads it as `$parameters.id`, and an id the screen's RecordSource does not provide is a 404.
func showRuntimeRecordPage(store *data.Store, cfg config.Config, navID, recordParam string) http.Handler {
	return gatedPage(store, cfg, navID, true, recordParam, func(ws domain.Workspace) []domain.NavigationItem { return ws.RuntimePages })
}

// gatedPage is what showWorkspacePage and showRuntimePage share: find the item among those `items` names, gate by its
// `open_to:`, lower its page for this viewer and draw it.
func gatedPage(store *data.Store, cfg config.Config, navID string, runtime bool, recordParam string, items func(domain.Workspace) []domain.NavigationItem) http.Handler {
	draw := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		ws := rendering.CurrentWorkspace(ctx)
		for _, item := range items(ws) {
			if item.ID != navID || item.Page == nil {
				continue
			}
			actor := pageViewer(currentActor(req, store, cfg), cfg)
			loader := composition.NewLoader(store, machinesFor(ctx))
			if runtime {
				loader = loader.WithRuntime(ws.RuntimeMachines, actor).WithParams(pageParams(req, ""))
				for source, provider := range webRuntimeProviders(store, cfg) {
					loader = loader.WithRuntimeProvider(source, provider)
				}
			}
			params := pageParams(req, "")
			if recordParam != "" {
				// One record's page: the route names the record, and one the source does not hold is a 404.
				id := chi.URLParam(req, recordParam)
				found, err := composition.RuntimeRecordExists(ctx, loader, actor, item.RecordSource, id)
				if err != nil {
					serverError(w, err)
					return
				}
				if !found {
					http.NotFound(w, req)
					return
				}
				name := item.RecordParam
				if name == "" {
					name = "id"
				}
				params[name] = id
			}
			current := navID
			if item.SettingsCurrent != "" {
				current = item.SettingsCurrent
			}
			body, err := composition.DeclaredPageFor(ctx, loader, actor, params, time.Now(), ws.LinkTargets(), current, *item.Page)
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
	})
	admin := requireWorkspaceAdmin(store, cfg)(draw)
	member := requireWorkspaceMember(store, cfg)(draw)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, item := range items(rendering.CurrentWorkspace(req.Context())) {
			if item.ID == navID && item.Page != nil && item.OpenTo == domain.OpenToMember {
				member.ServeHTTP(w, req)
				return
			}
		}
		admin.ServeHTTP(w, req)
	})
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

// pageViewer is the viewer a page is lowered for. The shared admin credential has no membership row and no Workspace role, and
// the admin gate lets it through by id: what a page asks of the viewer (`when: {viewer: workspace_role, is: admin}`, whether an
// admin action is offered) must say what the gate will do, so that identity is an admin here. It changes nothing a route
// decides: every route reads the actor itself.
func pageViewer(actor domain.Actor, cfg config.Config) domain.Actor {
	if actor.ID != "" && actor.ID == cfg.AdminUserID && actor.WorkspaceRole == "" {
		actor.WorkspaceRole = domain.WorkspaceRoleAdmin
	}
	return actor
}

// webRuntimeProviders are the readers of every source whose registry entry says `Provider: "web"`: they need what only a handler
// holds (the store's per-request identity, the process's configuration). A source left to the web package with no reader here is
// caught by TestWebProvidedRuntimeSourcesHaveAReader.
func webRuntimeProviders(store *data.Store, cfg config.Config) map[string]composition.RuntimeProvider {
	previews := &filePreviews{}
	return map[string]composition.RuntimeProvider{
		"workspace_files":    workspaceFilesProvider(store, cfg),
		"file_reviews":       fileReviewsProvider(store, cfg, previews),
		"file_diffs":         fileDiffsProvider(store, cfg, previews),
		"snapshots":          snapshotsProvider(store, cfg),
		"templates":          templatesProvider(cfg),
		"applications":       applicationsProvider(store, cfg),
		"draft_applications": draftApplicationsProvider(store),
		"current_workspace":  currentWorkspaceProvider(store),
	}
}
