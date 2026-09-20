package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// showGroups lists the Workspace's Groups (ROADMAP.md Case 03 Fase 4). Gated by
// requireWorkspaceAdmin, like every other membership-administration route.
func showGroups(store *data.Store, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		groups, err := store.ListGroups(ctx, workspaceID)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.GroupsPage(groups, chrome.WorkspaceName, chrome.UserInitials, roleApplications(ws), switchHref))
	}
}

func showGroupDetail(store *data.Store, cfg config.Config, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		groupID := chi.URLParam(req, "groupID")

		group, err := store.GetGroup(ctx, workspaceID, groupID)
		if err != nil {
			recordError(w, err)
			return
		}
		choices, err := groupMemberChoices(ctx, store, workspaceID, groupID)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		_, switchHref := viewerWorkspaceContext(ctx, store, userID)
		render(ctx, w, rendering.GroupDetailPage(*group, choices, chrome.WorkspaceName, chrome.UserInitials, roleApplications(ws), switchHref))
	}
}

// groupMemberChoices lists every Workspace member with whether they are currently in this Group --
// what the detail screen's checkbox list renders.
func groupMemberChoices(ctx context.Context, store *data.Store, workspaceID, groupID string) ([]rendering.GroupMemberChoice, error) {
	members, err := store.ListMembers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	inGroup, err := store.GroupMemberIDs(ctx, groupID)
	if err != nil {
		return nil, err
	}
	current := make(map[string]bool, len(inGroup))
	for _, id := range inGroup {
		current[id] = true
	}

	choices := make([]rendering.GroupMemberChoice, 0, len(members))
	for _, m := range members {
		choices = append(choices, rendering.GroupMemberChoice{
			UserRecordID: m.UserRecordID,
			Email:        m.Email,
			InGroup:      current[m.UserRecordID],
		})
	}
	return choices, nil
}

func submitCreateGroup(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.FormValue("name"))
		if name == "" {
			http.Error(w, "group name is required", http.StatusUnprocessableEntity)
			return
		}
		if _, err := store.CreateGroup(ctx, workspaceID, name); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/workspace-groups")
	}
}

// submitGroupMembers replaces the Group's membership with exactly what was ticked. The form
// describes the intended final set rather than a sequence of adds and removes, which is why
// data.SetGroupMembers takes a whole list: unticking everyone is a meaningful submission, and an
// add/remove protocol would make it indistinguishable from submitting nothing.
func submitGroupMembers(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		groupID := chi.URLParam(req, "groupID")
		if _, err := store.GetGroup(ctx, workspaceID, groupID); err != nil {
			recordError(w, err)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		if err := store.SetGroupMembers(ctx, groupID, req.Form["member"]); err != nil {
			serverError(w, err)
			return
		}
		redirectTo(w, req, "/workspace-groups/"+groupID)
	}
}

// submitGroupRoles sets the Group's role in each Application, validated against that
// Application's own declared roles: vocabulary by the same submittedAppRoles the member path
// uses -- so an undeclared role is rejected here too, not only there.
func submitGroupRoles(store *data.Store, ws domain.Workspace) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		groupID := chi.URLParam(req, "groupID")
		if _, err := store.GetGroup(ctx, workspaceID, groupID); err != nil {
			recordError(w, err)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		roles, err := submittedAppRoles(req, ws.Applications)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		for appID, role := range roles {
			if err := store.SetGroupAppRole(ctx, groupID, appID, role); err != nil {
				serverError(w, err)
				return
			}
		}
		redirectTo(w, req, "/workspace-groups/"+groupID)
	}
}

func submitDeleteGroup(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		workspaceID, _ := data.WorkspaceScope(ctx)
		if err := store.DeleteGroup(ctx, workspaceID, chi.URLParam(req, "groupID")); err != nil {
			recordError(w, err)
			return
		}
		redirectTo(w, req, "/workspace-groups")
	}
}
