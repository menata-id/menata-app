package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/aiassist"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/pdf"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// showDeclaredPage renders any navigation item that declares `page:` (007 §12.4). One handler for all of
// them: the item is found by id in the Application the route resolved to, so which Workspace and which
// Application a request is in stays the per-request fact it is everywhere else, and an id that names no
// declared page -- another Application's, or a bespoke screen's -- is a 404 rather than an empty shell.
func showDeclaredPage(store *data.Store, cfg config.Config, files *storage.Store, printer *pdf.Printer, reader aiassist.Extractor) http.HandlerFunc {
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
				record, err := store.GetRecord(ctx, item.RecordOf, recordID)
				if err != nil {
					if errors.Is(err, data.ErrRecordNotFound) {
						http.NotFound(w, req)
						return
					}
					serverError(w, err)
					return
				}
				params["id"] = recordID
				if m := machinesFor(ctx)[item.RecordOf]; m != nil {
					for name, v := range recordParams(m, record.Values) {
						params[name] = v
					}
				}
			}
			actor := currentActor(req, store, cfg)
			targets := linkTargetsFor(rendering.CurrentWorkspace(ctx), app, actor, cfg)
			loader := composition.NewLoader(store, machinesFor(ctx)).WithFiles(files).WithPDF(printer.Available()).WithReader(aiassist.Available(reader))
			header, carry, params, err := viewGroupOf(ctx, loader, app, item, actor, params)
			if err != nil {
				serverError(w, err)
				return
			}
			body, err := composition.DeclaredPageCarrying(ctx, loader, actor, params, time.Now(), targets, navID, *item.Page, carry)
			if err != nil {
				serverError(w, err)
				return
			}
			workspaceName, viewer, switchHref, err := pageChrome(ctx, req, store, cfg)
			if err != nil {
				serverError(w, err)
				return
			}
			render(ctx, w, rendering.DeclaredScreen(navID, body, header, workspaceName, viewer, switchHref))
			return
		}
		http.NotFound(w, req)
	}
}

// viewGroupOf is what a screen of a view group adds to its page: the group's header, the carrying of its scope choice onto the links
// between its screens, and the parameters the page is read with (a scope value naming no record is dropped). A screen of no group
// has none of the three and keeps the parameters it came with.
func viewGroupOf(ctx context.Context, loader *composition.Loader, app domain.Application, item domain.NavigationItem, viewer domain.Actor, params map[string]string) (*rendering.ViewHeader, func(navID, href string) string, map[string]string, error) {
	group, ok := app.ViewGroupByID(item.ViewGroup)
	if !ok {
		return nil, nil, params, nil
	}
	header, params, err := composition.ViewHeaderFor(ctx, loader, app, item, group, viewer, params, time.Now())
	return header, composition.ViewCarry(app, group, params), params, err
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
			render(ctx, w, rendering.DeclaredScreen(navID, body, nil, workspaceName, viewer, switchHref))
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
	// The runtime owns every `record.` name (domain.RecordParamPrefix): a record page sets them from its own record, and a query
	// string never supplies one -- `?record.fld_contract=` would otherwise aim a PO's instalments at another contract.
	for name := range out {
		if strings.HasPrefix(name, domain.RecordParamPrefix) {
			delete(out, name)
		}
	}
	out["application"] = applicationID
	return out
}

// recordParams are the values of a record page's record as request parameters, `record.<field id>`: its scalar Fields as
// stored, so a Dataset on the page may select "the records whose contract is this record's" (`$parameters.record.fld_contract`).
func recordParams(m *domain.Machine, values map[string]any) map[string]string {
	out := map[string]string{}
	for _, f := range m.Fields {
		if f.Type == domain.FieldTypeFile || f.Type == domain.FieldTypeGroup {
			continue
		}
		if v := composition.DisplayString(values[f.ID]); v != "" {
			out[domain.RecordParamPrefix+f.ID] = v
		}
	}
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
		"workspace_files":        workspaceFilesProvider(store, cfg),
		"file_reviews":           fileReviewsProvider(store, cfg, previews),
		"file_diffs":             fileDiffsProvider(store, cfg, previews),
		"snapshots":              snapshotsProvider(store, cfg),
		"templates":              templatesProvider(cfg),
		"applications":           applicationsProvider(store, cfg),
		"workspace_applications": workspaceApplicationsProvider(),
		"current_workspace":      currentWorkspaceProvider(store),
		"builder_nodes":          builderNodesProvider(store, cfg),
		"builder_roles":          builderRolesProvider(),
		"builder_drafts":         builderDraftsProvider(store, cfg),
	}
}

// linkTargetsFor is what a link on one of app's pages may name: its own targets (Application.LinkTargets) and -- for a link
// that says `in:` -- the screens of the Workspace's *other* Applications this viewer may open, each named
// "<application id>/<item id>" (domain.ForeignLinkID). An Application that is not installed here, or that this viewer
// cannot open, contributes none, so such a link is not drawn rather than drawn to a 403 or a 404. A screen that is no menu
// entry (the form a list opens) may be named -- that is what "Create payment plan" is -- but a record page may not: it needs
// the record's id, which only a link from inside the Application passes.
func linkTargetsFor(ws domain.Workspace, app domain.Application, actor domain.Actor, cfg config.Config) []domain.NavigationItem {
	targets := app.LinkTargets(ws.Navigation)
	for _, other := range openableApplications(ws, actor, cfg) {
		if other.ID == app.ID {
			continue
		}
		for _, item := range other.AllNavigation {
			if item.RecordOf != "" || item.Route == "" {
				continue
			}
			item.ID = domain.ForeignLinkID(other.ID, item.ID)
			targets = append(targets, item)
		}
	}
	return targets
}
