package web

import (
	"net/http"
	"strings"

	"menata.app/internal/data"
)

// blockUnavailableWorkspace answers 503 for a request in a Workspace whose manifest failed to load (K09).
// Without it that Workspace would resolve to the zero domain.Workspace -- no Machines, no Applications -- and
// every screen would fail in its own way, or worse render empty and look like lost data.
//
// A member is never trapped: the routes that leave or manage the session (switching Workspace, the account
// pages, sign-out) stay reachable, since none of them reads this Workspace's metadata.
func blockUnavailableWorkspace(store *data.Store, unavailable map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(unavailable) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			row, ok := currentWorkspaceRow(req.Context(), store)
			if !ok || !unavailable[row.Slug] || leavesTheWorkspace(req.URL.Path) {
				next.ServeHTTP(w, req)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">` +
				`<title>Workspace unavailable</title><main style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem">` +
				`<h1>This workspace is unavailable</h1>` +
				`<p>Its configuration did not load, so it cannot be shown right now. Your data is untouched. ` +
				`A workspace administrator can see the reason in the server log.</p>` +
				`<p><a href="/switch-workspace">Switch to another workspace</a></p></main>`))
		})
	}
}

// leavesTheWorkspace is the closed set of paths that must keep answering in an unavailable Workspace.
func leavesTheWorkspace(path string) bool {
	switch {
	case path == "/logout", path == "/api/account-menu/workspaces":
		return true
	case strings.HasPrefix(path, "/switch-workspace"), strings.HasPrefix(path, "/create-workspace"), strings.HasPrefix(path, "/account-"):
		return true
	}
	return false
}
