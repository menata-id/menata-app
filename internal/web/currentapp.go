package web

import (
	"net/http"
	"strings"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

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
// navigation is the fallback for routes naming no Machine (/dashboard, /my-tasks, ...). The
// Machine -> Application map is metadata the Application already declares, so this costs no extra
// declaration; metadata.validateApplicationClaims keeps it unambiguous by refusing a Machine
// claimed twice.
//
// Resolving to no Application is a normal outcome, not a failure: the Workspace-level screens
// (Home, Members) belong to none, and so do routes concerning a shared Machine (mch_user,
// mch_activity). rendering.CurrentApplicationName degrades to the Workspace's own name.
func currentApplication(ws domain.Workspace) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if app, ok := applicationForPath(ws, req.URL.Path); ok {
				req = req.WithContext(rendering.WithCurrentApplication(req.Context(), app))
			}
			next.ServeHTTP(w, req)
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
	return ws.ApplicationForRoute(path)
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
			if actor.ID != "" && actor.ID == cfg.AdminUserID {
				next.ServeHTTP(w, req)
				return
			}
			if len(actor.Roles[app.ID]) == 0 {
				http.Error(w, "you have no role in "+app.Name, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
