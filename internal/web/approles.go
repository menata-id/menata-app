package web

import (
	"fmt"
	"net/http"
	"slices"

	"menata.app/internal/domain"
)

// appRoleField is the form field carrying one Application's role, namespaced by Application id so
// a single form can carry several without positional guessing.
func appRoleField(applicationID string) string {
	return "app_role[" + applicationID + "]"
}

// submittedAppRoles reads one role per Application out of a submitted form and checks each against
// that Application's own declared vocabulary (domain.Application.Roles).
//
// The validation is the point, not a side effect of the plumbing. Before Fase 3b the write paths
// took whatever string arrived in app_role and stored it: `workspace_members.app_role` accepted
// any value, so a typo or a crafted POST could store a role no screen could render and no
// vocabulary contained. `workspace_role` has always been checked this way ("admin"/"member");
// the Application role never was, because until roles were declared there was nothing to check
// against. Now there is. This closes a gap that predates this phase rather than one it opens.
//
// An empty value is legitimate and means "no role here" -- SetMemberAppRole deletes the row.
// An Application declaring no roles is skipped entirely: it offers no select, so a value arriving
// for it did not come from a form this app rendered.
func submittedAppRoles(req *http.Request, applications []domain.Application) (map[string]string, error) {
	roles := make(map[string]string, len(applications))
	for _, app := range applications {
		if len(app.Roles) == 0 {
			continue
		}
		value := req.FormValue(appRoleField(app.ID))
		if value == "" {
			roles[app.ID] = ""
			continue
		}
		if !slices.Contains(app.Roles, value) {
			return nil, fmt.Errorf("application %s does not declare role %q", app.ID, value)
		}
		roles[app.ID] = value
	}
	return roles, nil
}
