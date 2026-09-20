package web

import (
	"net/http"
	"strings"

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
