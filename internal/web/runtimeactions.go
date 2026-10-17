package web

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
	"menata.app/internal/registry"
	"menata.app/internal/rendering"
)

// actionRunner performs one registry.RuntimeActions entry. It reports a refusal itself, to w, and returns ok=false; on
// success it returns where the person goes next ("" means: stay, with the page re-read).
type actionRunner func(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (next string, ok bool)

// actionEnv is what a runner may use of the process: the store, and the mailer and configuration an invitation needs. The
// runner is handed no more than this, so what an action can reach is what is listed here.
type actionEnv struct {
	store  *data.Store
	mailer mail.Mailer
	cfg    config.Config
}

// runtimeActionRunners are the executors, one per registry.RuntimeActions entry (TestRuntimeActionsAndRunnersAgree).
var runtimeActionRunners = map[string]actionRunner{
	"create_group": runCreateGroup,
	"delete_group": runDeleteGroup,
	// A Group's membership is the whole set ticked: unticking everyone is a meaningful submission, which is why
	// data.SetGroupMembers takes a list and not a sequence of adds and removes.
	"set_group_members": runSetGroupMembers,
	"set_group_roles":   runSetGroupRoles,
	"invite_member":     runInviteMember,
	"revoke_invite":     runRevokeInvite,
}

// runtimeAction is `POST /runtime-actions/{action}[?record=<id>]`, the one route a runtime page's Form or Button for a
// named operation posts to. It is registered under the Workspace-admin gate, which is every action's gate today: an action
// that is not admin-only would need a route of its own rather than ride this one, so it fails closed here.
//
// The route holds no logic of any operation: it looks the name up in the closed set, checks that a record is named
// exactly when the action is about one, and hands over. What an action may do is what its runner does, and nothing a page
// said.
func runtimeAction(store *data.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	env := actionEnv{store: store, mailer: mailer, cfg: cfg}
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
		next, ok := run(w, req, env, record)
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

func runCreateGroup(w http.ResponseWriter, req *http.Request, env actionEnv, _ string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	name := strings.TrimSpace(req.FormValue("name"))
	if name == "" {
		http.Error(w, "group name is required", http.StatusUnprocessableEntity)
		return "", false
	}
	if _, err := env.store.CreateGroup(req.Context(), workspaceID, name); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

func runDeleteGroup(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	if err := env.store.DeleteGroup(req.Context(), workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	return domain.RuntimeScreenRoute("nav_workspace_groups"), true
}

func runSetGroupMembers(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	if _, err := env.store.GetGroup(ctx, workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	if err := env.store.SetGroupMembers(ctx, record, req.Form["member"]); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}

// runSetGroupRoles sets the Group's role in each Application, validated against that Application's own declared roles
// (submittedAppRoles, the reader the member screens use), so an undeclared role is refused here as there.
func runSetGroupRoles(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	if _, err := env.store.GetGroup(ctx, workspaceID, record); err != nil {
		recordError(w, err)
		return "", false
	}
	roles, err := submittedAppRoles(req, rendering.CurrentWorkspace(ctx).Applications)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return "", false
	}
	for appID, role := range roles {
		if err := env.store.SetGroupAppRole(ctx, record, appID, role); err != nil {
			serverError(w, err)
			return "", false
		}
	}
	return "", true
}

// runInviteMember records an invitation and emails its workspace-bound token. It creates no membership and no mch_user
// record: those come into existence when the invitation is *accepted* (submitAcceptInvite), which is the owner's rule from
// 2026-09-22 -- someone who has confirmed nothing is not a member, so they must not appear in a member list, in an approver
// picker, or in any authorization lookup. It asks no name for the invitee for the same reason: a name belongs to whoever owns
// the email. An admin supplies the email and the roles, the two things that are the Workspace's decision.
func runInviteMember(w http.ResponseWriter, req *http.Request, env actionEnv, _ string) (string, bool) {
	ctx := req.Context()
	workspaceID, _ := data.WorkspaceScope(ctx)
	email := normalizeEmail(req.FormValue("email"))
	if email == "" {
		http.Error(w, "email is required", http.StatusUnprocessableEntity)
		return "", false
	}
	appRoles, err := submittedAppRoles(req, rendering.CurrentWorkspace(ctx).Applications)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return "", false
	}
	// Already a member of this Workspace: the invitation would have nothing to grant, and accepting it would try to create a
	// second membership for the same identity.
	if _, ok, err := resolveWorkspaceMembership(ctx, env.store, email, workspaceID); err != nil {
		serverError(w, err)
		return "", false
	} else if ok {
		http.Error(w, "that email is already a member of this workspace", http.StatusUnprocessableEntity)
		return "", false
	}
	invite := data.PendingInvite{WorkspaceID: workspaceID, Email: email, WorkspaceRole: "member", AppRoles: appRoles}
	if err := env.store.CreatePendingInvite(ctx, invite); err != nil {
		serverError(w, err)
		return "", false
	}
	sendInviteEmail(ctx, env.mailer, env.cfg, email, workspaceID)
	return "", true
}

// runRevokeInvite withdraws an invitation that has not been accepted. There is no membership to remove and no record to
// delete, which is what an invitation living in its own table buys: one delete, and the emailed link stops working at once
// because accepting re-checks that the invitation still exists.
func runRevokeInvite(w http.ResponseWriter, req *http.Request, env actionEnv, record string) (string, bool) {
	workspaceID, _ := data.WorkspaceScope(req.Context())
	email := normalizeEmail(record)
	if email == "" {
		http.Error(w, "email is required", http.StatusUnprocessableEntity)
		return "", false
	}
	if err := env.store.DeletePendingInvite(req.Context(), workspaceID, email); err != nil {
		serverError(w, err)
		return "", false
	}
	return "", true
}
