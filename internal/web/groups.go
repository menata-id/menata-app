package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// showGroups lists the Workspace's Groups (ROADMAP.md Case 03 Fase 4). Gated by
// requireWorkspaceAdmin, like every other membership-administration route.
func showGroups(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		ws := rendering.CurrentWorkspace(ctx)
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
		render(ctx, w, rendering.GroupsPage(groups, chrome.WorkspaceName, chrome.Viewer(), roleApplications(ws), switchHref))
	}
}

func showGroupDetail(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		ws := rendering.CurrentWorkspace(ctx)
		groupID := chi.URLParam(req, "groupID")

		// Through the Loader's memoized Groups list rather than store.GetGroup: something else on this
		// request already lists this Workspace's Groups, and each listing re-reads their role grants
		// -- so asking for one Group by id read the same grants a second time (the per-record route
		// sweep measured `group grants x2` for the same Workspace, 2026-09-28). Same data, one read.
		ld := composition.NewLoader(store, machinesFor(ctx))
		group, err := groupFromListing(ctx, ld, groupID)
		if err != nil {
			recordError(w, err)
			return
		}
		choices, err := groupMemberChoices(ctx, ld, store, groupID)
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
		render(ctx, w, rendering.GroupDetailPage(*group, choices, chrome.WorkspaceName, chrome.Viewer(), roleApplications(ws), switchHref))
	}
}

// groupFromListing picks one Group out of the Workspace's own listing, reporting the same
// ErrRecordNotFound store.GetGroup would for an id that is not there -- so the route still 404s
// rather than rendering an empty screen.
func groupFromListing(ctx context.Context, ld *composition.Loader, groupID string) (*data.Group, error) {
	groups, err := ld.Groups(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == groupID {
			return &groups[i], nil
		}
	}
	return nil, data.ErrRecordNotFound
}

// groupMemberChoices lists every Workspace member with whether they are currently in this Group --
// what the detail screen's checkbox list renders.
func groupMemberChoices(ctx context.Context, ld *composition.Loader, store *data.Store, groupID string) ([]rendering.GroupMemberChoice, error) {
	// Through the Loader, not store.ListMembers: that call reaches ListGroups itself (for each
	// member's own Groups), so a screen that also lists Groups read them -- and their role grants --
	// twice. Loader.Members already feeds ListMembersFrom its memoized Group list; this screen simply
	// was not asking the Loader.
	members, err := ld.Members(ctx)
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
func submitGroupRoles(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		ws := rendering.CurrentWorkspace(ctx)
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
