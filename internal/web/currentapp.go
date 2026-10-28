package web

import (
	"log"
	"net/http"
	"strings"

	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
	"menata.app/internal/rendering"
)

// currentWorkspace resolves which Workspace a request is in and puts its installation on ctx
// (2026-09-22). Registered before currentApplication, which reads what this resolves.
//
// This is the fix for what the owner found on a brand-new Workspace: Dokter Kecil had just been
// created, its Home should have been empty, and it showed two Applications. The Applications came
// from a single manifest the process loaded once at startup and handed to every request, so a
// `workspaces` row scoped records and membership but not what the Workspace *was*.
//
// The hop is slug-shaped because that is what a manifest names: session -> Workspace row -> slug
// -> installed manifest. A Workspace with no manifest resolves to the zero Workspace, which is a
// real answer and not an error -- no Applications, an empty Home, exactly the state a Workspace is
// in before anything is installed into it.
//
// Best-effort on a failed lookup, like resolveChrome's own reads: a page renders with empty chrome
// rather than failing outright, and the request is still scoped to the right records either way.
func currentWorkspace(store *data.Store, workspaces map[string]domain.Workspace) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			// The Workspace row comes from resolveIdentity, which read it for this request
			// already; this used to fetch it a second time (the duplicate resolveChrome's own
			// doc comment now records). The fallback keeps a handler mounted without that
			// middleware working, same as every other consumer of the identity.
			if row, ok := currentWorkspaceRow(ctx, store); ok {
				ctx = rendering.WithCurrentWorkspace(ctx, workspaces[row.Slug], row.Name, row.Archived)
				req = req.WithContext(ctx)
				data.NoteWorkspace(ctx, row.Slug)
			}
			next.ServeHTTP(w, req)
		})
	}
}

// currentApplication resolves which Application a request is in and puts it on ctx for the
// rendering layer (Fase 3, 2026-09-20). Registered inside the authenticated group, after
// requireAuth, since no pre-auth screen has an Application.
//
// THE DERIVATION, and why it is Machine-first rather than navigation-first:
//
// The obvious rule -- find the navigation item whose route matches this path -- looks sufficient
// and is not. Diffing the registered route table against the declared navigation shows the whole
// /machines/{machineID}/... surface (record detail, /decide, /edit, /pdf-preview,
// /signature-placement) plus /documents and the member-edit routes are named by *no* navigation
// item at all -- and those are most of Document Approval's real screens. Navigation-only
// derivation would return nothing for exactly the pages the proof of concept cares about.
//
// So the Machine the route concerns is asked first (it is right there in the path), and
// navigation is the fallback for routes naming no Machine (/calendar, ...). The
// Machine -> Application map is metadata the Application already declares, so this costs no extra
// declaration; metadata.validateApplicationClaims keeps it unambiguous by refusing a Machine
// claimed twice.
//
// Resolving to no Application is a normal outcome, not a failure: the Workspace-level screens
// (Home, Members) belong to none, and so do routes concerning a shared Machine (mch_user,
// mch_activity). rendering.CurrentApplication returns false and callers degrade to the Workspace's own name.
func currentApplication() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := rendering.WithCurrentPath(req.Context(), req.URL.Path, req.URL.RawQuery)
			if app, ok := applicationForPath(rendering.CurrentWorkspace(req.Context()), req.URL.Path); ok {
				if app.Inactive {
					// Switched off by a Workspace admin: nothing the Application owns is served, not even to an admin (they switch it
					// back on from the Workspace's own Applications screen). Its data is untouched.
					http.NotFound(w, req)
					return
				}
				ctx = rendering.WithCurrentApplication(ctx, app)
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

// applicationForPath is currentApplication's pure half, so the derivation is testable without a
// server.
func applicationForPath(ws domain.Workspace, path string) (domain.Application, bool) {
	if machineID, ok := machineIDFromPath(path); ok {
		if app, found := ws.ApplicationForMachine(machineID); found {
			return app, true
		}
		// A route naming a Machine that no Application claims (a shared, Workspace-level one)
		// belongs to no Application. Falling through to the route match would be wrong, not
		// helpful: /machines/mch_activity/... is Workspace-level however it was reached.
		return domain.Application{}, false
	}
	if hubID, ok := settingsHubIDFromPath(path); ok {
		// /settings/{navID}/members... belongs to the Application whose hub item is navID. The hub's own
		// route is a navigation route and resolves below; these sub-addresses are the runtime's, so they
		// are found through the hub they hang under.
		for _, app := range ws.Applications {
			if _, found := composition.SettingsHubOf(app, hubID); found {
				return app, true
			}
		}
		return domain.Application{}, false
	}
	// One record's page (`/pages/{navID}/{recordID}`, `record_of:`) belongs to the Application whose navigation declares
	// `/pages/{navID}`, so its roles gate it as they gate the list.
	if rest, ok := strings.CutPrefix(path, "/pages/"); ok {
		if navID, tail, found := strings.Cut(rest, "/"); found && navID != "" && tail != "" {
			return ws.ApplicationForRoute("/pages/" + navID)
		}
	}
	return ws.ApplicationForRoute(path)
}

// settingsHubIDFromPath extracts {navID} from /settings/{navID}/<anything>. The hub's own address,
// /settings/{navID}, has no further segment and is resolved by navigation like any other route.
func settingsHubIDFromPath(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/settings/")
	if !ok {
		return "", false
	}
	id, tail, found := strings.Cut(rest, "/")
	if !found || id == "" || tail == "" {
		return "", false
	}
	return id, true
}

// machineIDFromPath extracts the Machine id from any /machines/{machineID}/... route, including
// the API twins under /api. Returns false for a path that names no Machine.
//
// Chi's own URL params are not available here: this middleware runs before the route is matched,
// which is deliberate -- the Application has to be on ctx before any handler renders.
func machineIDFromPath(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/api")
	if !ok {
		rest = path
	}
	rest, ok = strings.CutPrefix(rest, "/machines/")
	if !ok {
		return "", false
	}
	id, _, _ := strings.Cut(rest, "/")
	if id == "" {
		return "", false
	}
	return id, true
}

// requireApplicationAccess refuses a request that is inside an Application the viewer holds no
// role in (owner decision, 2026-09-21: *someone who is not a member of an application cannot do
// anything in it* -- not even look).
//
// It is registered immediately after currentApplication and reads what that middleware just
// resolved, which is what makes it complete rather than approximate: every route inside an
// Application passes through here, the generic /machines/... surface and the bespoke screens
// alike, so there is no page to remember to gate and none to forget. Gating the handlers one by
// one was the alternative, and it would have left /dashboard and /approval-inbox -- the two that
// read across everyone's documents -- depending on nobody overlooking them.
//
// **This is Application-level access, deliberately coarser than a per-Machine `read` Permission.**
// The runtime has no `read` Action (upstream's CAP-P05 `can_read` is the finer shape, unbuilt
// here; see ROADMAP.md), so what can be said today is "in or out of this Application", not "may
// see Documents but not Approval Steps". That is exactly the rule the owner stated, so it is
// enough -- but it is worth naming, because the Authorization Matrix renders this as one row and
// a reader could otherwise take it for per-record read control.
//
// Three cases pass through untouched, each for its own reason:
//
//   - A request in no Application at all (Workspace Home, Members, Groups, the matrix itself).
//     There is no vocabulary to check against.
//   - An Application that declares no `roles:` (Project Management). Nobody can hold a role there,
//     so requiring one would lock out every single person -- the "rule that reads as a grant and
//     denies everyone" shape this repo refuses elsewhere.
//   - The shared admin credential's placeholder identity, by id, the same narrow exemption
//     requireWorkspaceAdmin takes. It has no membership row and therefore no role anywhere; it
//     predates all of this.
//
// The membership read is skipped entirely for the first two, so an Application declaring no roles
// costs nothing per request and the Workspace screens cost nothing at all.
func requireApplicationAccess(store *data.Store, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			app, ok := rendering.CurrentApplication(req.Context())
			if !ok || len(app.Roles) == 0 {
				next.ServeHTTP(w, req)
				return
			}
			actor := currentActor(req, store, cfg)
			if !canOpenApplication(app, actor, cfg) {
				http.Error(w, noRoleMessage(app, actor), http.StatusForbidden)
				return
			}
			// An item that names who may see it (`visible_to:`) is not served to anyone else either, at its address or on its record pages.
			if navAddressRefused(app, req.Method, req.URL.Path, req.URL.RawQuery, actor, cfg) {
				http.Error(w, "this screen is not open to your role in this application", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// navItemVisible is the rule `visible_to:` states, for one item and one viewer: an item that names no roles is everyone's; otherwise the viewer holds one of
// them in the Application, or is a Workspace admin or the configured admin (who could rewrite the declaration). The menu and the route both ask it.
func navItemVisible(app domain.Application, item domain.NavigationItem, actor domain.Actor, cfg config.Config) bool {
	// An item under a parent is offered only where its parent is: a child of a hidden parent would be reachable by its address and listed nowhere.
	if item.Parent != "" {
		for _, other := range app.AllNavigation {
			if other.ID == item.Parent && other.Parent == "" && !navItemVisible(app, other, actor, cfg) {
				return false
			}
		}
	}
	if len(item.VisibleTo) == 0 || actor.WorkspaceRole == domain.WorkspaceRoleAdmin || (actor.ID != "" && actor.ID == cfg.AdminUserID) {
		return true
	}
	for _, role := range item.VisibleTo {
		if actor.HasRole(app.ID, role) {
			return true
		}
	}
	return false
}

// hiddenNavItems is the ids of app's items this viewer is not offered (presentation only: the route gate answers for the address).
func hiddenNavItems(app domain.Application, actor domain.Actor, cfg config.Config) map[string]bool {
	var hidden map[string]bool
	for _, item := range app.AllNavigation {
		if !navItemVisible(app, item, actor, cfg) {
			if hidden == nil {
				hidden = map[string]bool{}
			}
			hidden[item.ID] = true
		}
	}
	return hidden
}

// navItemsOfRequest are the navigation items whose address the request is, which is how `visible_to:` finds what to judge:
//   - a page (`/pages/<id>`, also its record pages and `/pdf`) is the item that declares that route;
//   - an address is the item's route path **and its `?tab=`**: `/approval-inbox?tab=mine` and `/approval-inbox` are two items;
//   - a Machine's own **screens** (a GET of `/machines/<id>/...`: its list, a record, an edit form) are every item whose route opens that Machine, so hiding the
//     menu entry hides the record pages behind it too. Writes (`POST/PUT/PATCH/DELETE`) and the JSON twins (`/api/machines/...`) are not screens: what a person
//     may write is the Machine's Permissions, and a form on a visible page posts to the Machine's route whatever the Machine's own page offers. `visible_to:` is
//     which screens are offered, not who may read or write data.
//
// Several items may share an address; the caller offers it if any of them is offered.
func navItemsOfRequest(app domain.Application, method, path, rawQuery string) []domain.NavigationItem {
	if rest, ok := strings.CutPrefix(path, "/pages/"); ok {
		if navID, tail, found := strings.Cut(rest, "/"); found && navID != "" && tail != "" {
			path = "/pages/" + navID
		}
	}
	machineID, onMachine := machineIDFromPath(path)
	if onMachine && (method != http.MethodGet || strings.HasPrefix(path, "/api/")) {
		return nil
	}
	var out []domain.NavigationItem
	for _, item := range app.AllNavigation {
		route, query, _ := strings.Cut(item.Route, "?")
		switch {
		case onMachine:
			if m, ok := machineIDFromPath(route); ok && m == machineID {
				out = append(out, item)
			}
		case route == path && requestTab(query) == requestTab(rawQuery):
			out = append(out, item)
		}
	}
	return out
}

// requestTab is the value of a raw query's `tab`, "" when it has none.
func requestTab(rawQuery string) string {
	for _, pair := range strings.Split(rawQuery, "&") {
		if key, value, _ := strings.Cut(pair, "="); key == "tab" {
			return value
		}
	}
	return ""
}

// navAddressRefused says whether the viewer is offered none of the items at the request's address (and so is refused it). An address no item names is not
// this rule's to judge.
func navAddressRefused(app domain.Application, method, path, rawQuery string, actor domain.Actor, cfg config.Config) bool {
	items := navItemsOfRequest(app, method, path, rawQuery)
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if navItemVisible(app, item, actor, cfg) {
			return false
		}
	}
	return true
}

// canOpenApplication is requireApplicationAccess's rule as a predicate, so the lists that offer an
// Application -- Workspace Home's cards, the launcher, the mobile "More" sheet -- ask the same question
// the gate answers instead of restating it (001 #8). Before it existed they offered every installed
// Application, so a member with no role in one saw its card ("Your role: —") and met a 403 on opening it.
func canOpenApplication(app domain.Application, actor domain.Actor, cfg config.Config) bool {
	if len(app.Roles) == 0 {
		return true
	}
	if actor.ID != "" && actor.ID == cfg.AdminUserID {
		return true
	}
	return len(actor.Roles[app.ID]) > 0
}

// openableApplications is ws's Applications filtered by canOpenApplication, in declaration order.
func openableApplications(ws domain.Workspace, actor domain.Actor, cfg config.Config) []domain.Application {
	apps := make([]domain.Application, 0, len(ws.Applications))
	for _, app := range ws.Applications {
		if !app.Inactive && canOpenApplication(app, actor, cfg) {
			apps = append(apps, app)
		}
	}
	return apps
}

// applicationOwnedRoutes is the set of paths that exist only because some Application declares them
// in its `navigation:` -- the union over every installed Application in every Workspace, plus the
// template library's, with the query string dropped (`/approval-inbox?tab=mine` is one path).
//
// Derived, never listed (001 #3): a hand-written list of "Task Tracker's routes" would be the second
// copy of what `navigation:` already says. The library is part of the union so an Application no
// Workspace happens to have installed still has its routes recognised as owned.
//
// A runtime screen is excluded even if some manifest were to declare its route, so this set can never
// take a Workspace-level destination away.
func applicationOwnedRoutes(workspaces map[string]domain.Workspace, library []domain.Application) map[string]bool {
	owned := map[string]bool{}
	add := func(apps []domain.Application) {
		for _, app := range apps {
			for _, item := range app.AllNavigation {
				path, _, _ := strings.Cut(item.Route, "?")
				if path != "" {
					owned[path] = true
				}
			}
		}
	}
	for _, ws := range workspaces {
		add(ws.Applications)
	}
	add(library)
	for _, s := range domain.RuntimeScreens {
		delete(owned, s.Route)
	}
	return owned
}

// ownedApplicationRoutes reads the template library for applicationOwnedRoutes. A library that cannot
// be read is logged and left out rather than failing the route table: the installed Applications are
// still a correct (if smaller) union, and `/install-application` reports the same library error loudly
// to whoever opens it.
func ownedApplicationRoutes(d Deps) map[string]bool {
	var library []domain.Application
	templates, err := installer.Templates(d.Cfg.TemplatePath)
	if err != nil {
		log.Printf("owned routes: template library not read, using installed Applications only: %v", err)
	}
	for _, t := range templates {
		library = append(library, *t.Application)
	}
	return applicationOwnedRoutes(d.Workspaces, library)
}

// requireInstalledApplication answers 404 for a route some Application owns when the request's
// Workspace has not installed that Application.
//
// **It exists because those routes are registered globally and nothing else stopped them.**
// `requireApplicationAccess` lets a request in no Application pass untouched (correctly: Workspace
// Home has none), and so a request for `/calendar` in a Workspace that never installed Task Tracker
// reached `showCalendar`, which asks `routeByID` for an id this Workspace does not have and panics --
// or, for `/dashboard`, `/my-tasks`, `/sprint` and `/team-capacity` (all since deleted or declared), answered 500. Measured
// 2026-10-05: eight routes in each of four Workspaces. The shape is `ds_documents_with_steps` again,
// a requirement that lived only in Go and was reachable only through the failure it caused.
//
// Placed after currentApplication, which already did the work: a route that resolved to an Application
// is by construction installed here (it was found in this Workspace's own navigation), so this only
// has to ask about the *unresolved* ones. `routeByID` stays fail-loud -- an absent id inside an
// installed Application is a programmer error, and softening it would hide the next one.
func requireInstalledApplication(owned map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if _, in := rendering.CurrentApplication(req.Context()); !in && owned[req.URL.Path] {
				http.NotFound(w, req)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// noRoleMessage says what to do about a missing Application role, which the bare
// "you have no role in X" it replaced did not.
//
// **It exists because the same install gap has now produced this 403 three times**, and each time the
// person hitting it could not tell a broken install from a deliberate exclusion. An Application's
// metadata is installed by internal/installer, which is forbidden from reaching the database at all
// (boundary_test.go) -- so the role grant is necessarily a separate step, grantInstallerRole, and any
// install that does not go through POST /install-application skips it. That happened for the
// dokter-kecil Workspace and then, a day later, for hanomerch: metadata complete, zero rows in
// workspace_member_app_roles, and every screen answering 403 to the Workspace's only member.
//
// The plane boundary means this cannot be *prevented* here, so the goal is a message that ends the
// guessing: a workspace admin is told they can fix it themselves and where, and a member is told who
// to ask. Distinguishing them costs nothing -- domain.Actor already carries WorkspaceRole, resolved
// once per request.
func noRoleMessage(app domain.Application, actor domain.Actor) string {
	if actor.WorkspaceRole == "admin" {
		return "No one in this workspace has been given a role in " + app.Name + " yet, so nobody can open it." +
			" Assign one at " + domain.RuntimeScreenRoute("nav_workspace_members") + " -- including to yourself." +
			" (An application installed without a role grant looks exactly like this.)"
	}
	return "You have no role in " + app.Name + "." +
		" Ask a workspace admin to give you one at " + domain.RuntimeScreenRoute("nav_workspace_members") + "."
}
