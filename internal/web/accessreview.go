package web

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"menata.app/internal/aiassist"
	"menata.app/internal/composition"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// reviewAccess is what the review of a new Application asks about who else can open it: the Workspace's active members, the roles the
// Application declares and the role the others start with (the first that is not the publisher's own). It asks nothing for an Application
// that declares no roles -- nothing gates who can open it -- and nothing for an update, which changes no one's access.
func reviewAccess(ctx context.Context, store *data.Store, change aiassist.GeneratedChange) rendering.AccessView {
	app := change.Application
	if change.Kind != aiassist.KindNewApplication || app == nil || len(app.Roles) == 0 {
		return rendering.AccessView{}
	}
	view := rendering.AccessView{Roles: app.Roles, DefaultRole: app.Roles[0]}
	for _, r := range app.Roles {
		if r != app.PublisherRole {
			view.DefaultRole = r
			break
		}
	}
	loader := composition.NewLoader(store, machinesFor(ctx))
	members, err := loader.Members(ctx)
	names, nerr := loader.PersonNames(ctx)
	if err != nil || nerr != nil {
		return view // the question is still asked of "everyone"; the people list is the part that failed
	}
	for _, m := range members {
		if m.Deactivated {
			continue
		}
		label := strings.TrimSpace(names[m.UserRecordID])
		if label == "" || label == m.Email {
			label = m.Email
		}
		view.People = append(view.People, rendering.AccessPerson{ID: m.UserRecordID, Label: label})
	}
	return view
}

// accessGrant is the access a publication gives besides the publisher's own role: a role for each of these members.
type accessGrant struct {
	role   string
	people []string
}

// readAccess reads the review's choice from the publish form and checks it before anything is written: the role must be one the Application
// declares, and every person ticked an active member of this Workspace (a form is never trusted for either). It answers 422 itself and
// returns ok=false for a choice it cannot honour. "Only me", and a form that carries no choice, grant nothing.
func readAccess(w http.ResponseWriter, req *http.Request, store *data.Store, workspaceID string, app *aiassist.GeneratedApplication) (accessGrant, bool) {
	mode := req.Form.Get("access")
	if app == nil || len(app.Roles) == 0 || mode == "" || mode == rendering.AccessOnlyMe {
		return accessGrant{}, true
	}
	refuse := func(format string, args ...any) (accessGrant, bool) {
		http.Error(w, fmt.Sprintf(format, args...), http.StatusUnprocessableEntity)
		return accessGrant{}, false
	}
	role := req.Form.Get("people_role")
	if !slices.Contains(app.Roles, role) {
		return refuse("pick the role they get: one of %s", strings.Join(app.Roles, ", "))
	}
	members, err := store.ListMembers(req.Context(), workspaceID)
	if err != nil {
		serverError(w, err)
		return accessGrant{}, false
	}
	active := map[string]bool{}
	for _, m := range members {
		if !m.Deactivated {
			active[m.UserRecordID] = true
		}
	}
	grant := accessGrant{role: role}
	switch mode {
	case rendering.AccessEveryone:
		for id := range active {
			grant.people = append(grant.people, id)
		}
	case rendering.AccessSome:
		for _, id := range req.Form["people"] {
			if !active[id] {
				return refuse("%q is not an active member of this workspace", id)
			}
			grant.people = append(grant.people, id)
		}
	default:
		return refuse("access must be only_me, some or everyone")
	}
	slices.Sort(grant.people)
	return grant, true
}

// grantPublishedAccess gives the chosen members their role in the Application just published; the publisher's own role was granted
// first and is not overwritten.
func grantPublishedAccess(ctx context.Context, store *data.Store, workspaceID, appID, publisherID string, grant accessGrant) error {
	for _, id := range grant.people {
		if id == publisherID {
			continue
		}
		if err := store.SetMemberAppRole(ctx, workspaceID, id, appID, grant.role); err != nil {
			return err
		}
	}
	return nil
}
