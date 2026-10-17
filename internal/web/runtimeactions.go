package web

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/registry"
	"menata.app/internal/rendering"
)

// actionRunner performs one registry.RuntimeActions entry. It reports a refusal itself, to w, and returns ok=false; on
// success it returns where the person goes next ("" means: stay, with the page re-read).
type actionRunner func(w http.ResponseWriter, req *http.Request, store *data.Store, record string) (next string, ok bool)

// runtimeActionRunners are the executors, one per registry.RuntimeActions entry (TestRuntimeActionsAndRunnersAgree).
var runtimeActionRunners = map[string]actionRunner{
	"create_group": runCreateGroup,
	"delete_group": runDeleteGroup,
	// A Group's membership is the whole set ticked: unticking everyone is a meaningful submission, which is why
	// data.SetGroupMembers takes a list and not a sequence of adds and removes.
	"set_group_members": runSetGroupMembers,
	"set_group_roles":   runSetGroupRoles,
}

// runtimeAction is `POST /runtime-actions/{action}[?record=<id>]`, the one route a runtime page's Form or Button for a
// named operation posts to. It is registered under the Workspace-admin gate, which is every action's gate today: an action
// that is not admin-only would need a route of its own rather than ride this one, so it fails closed here.
//
// The route holds no logic of any operation: it looks the name up in the closed set, checks that a record is named
// exactly when the action is about one, and hands over. What an action may do is what its runner does, and nothing a page
// said.
func runtimeAction(store *data.Store, _ config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id := chi.URLParam(req, "action")
		a, known := registry.RuntimeActions[id]
		run, hasRunner := runtimeActionRunners[id]
		if !known || !hasRunner {
			http.Error(w, "unknown action", http.StatusNotFound)
			return
		}
		if !a.Admin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		record := req.URL.Query().Get("record")
		if (a.Source != "") != (record != "") {
			http.Error(w, "this action is about a record exactly when it names one", http.StatusBadRequest)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		next, ok := run(w, req, store, record)
		if !ok {
			return
		}
		switch {
		case next != "":
			redirectTo(w, req, next)
		case req.Header.Get("HX-Request") == "true":
			w.Header().Set("HX-Refresh", "true")
		default:
			http.Redirect(w, req, "/home", http.StatusSeeOther)
		}
	}
}

func runCreateGroup(w http.ResponseWriter, req *http.Request, store *data.Store, _ string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	name := strings.TrimSpace(req.FormValue("name"))
	if name == "" {
		http.Error(w, "group name is required", http.StatusUnprocessableEntity)
		return "", false
	}
	if _, err := store.CreateGroup(req.Context(), workspaceID, name); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

func runDeleteGroup(w http.ResponseWriter, req *http.Request, store *data.Store, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	if err := store.DeleteGroup(req.Context(), workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	return domain.RuntimeScreenRoute("nav_workspace_groups"), true
}

func runSetGroupMembers(w http.ResponseWriter, req *http.Request, store *data.Store, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	if _, err := store.GetGroup(ctx, workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	if err := store.SetGroupMembers(ctx, record, req.Form["member"]); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

// runSetGroupRoles sets the Group's role in each Application, validated against that Application's own declared roles
// (submittedAppRoles, the reader the member screens use), so an undeclared role is refused here as there.
func runSetGroupRoles(w http.ResponseWriter, req *http.Request, store *data.Store, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	if _, err := store.GetGroup(ctx, workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	roles, err := submittedAppRoles(req, rendering.CurrentWorkspace(ctx).Applications)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return "", false
	}
	for appID, role := range roles {
		if err := store.SetGroupAppRole(ctx, record, appID, role); err != nil {
			serverError(w, err)
			return "", false
		}
	}
	return "", true
}
