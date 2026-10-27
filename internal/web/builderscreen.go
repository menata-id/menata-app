package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// The sections of the builder's tree, in the order the mockups (U01-U04) draw them. A part's place is "<section>/<Machine>/<Field>": the
// Tree reads the title as a path, so a name that holds a slash is written with a different mark.
const (
	builderSectionData     = "Data & form"
	builderSectionViews    = "Views"
	builderSectionMenu     = "Menu"
	builderSectionWorkflow = "Workflow"
)

// builderPlaceName is a name as one segment of a part's place: a slash would read as a folder.
func builderPlaceName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "/", "-"))
	if name == "" {
		return "(unnamed)"
	}
	return name
}

// builderNodesProvider is the reader of the `builder_nodes` source: for every Application of the Workspace, its parts in the order the tree shows
// them. The id of a part is the Application's id and the part's metadata path, so one part of one Application is one record whichever Applications
// share a Machine; the address selects it with `?node=`. Nothing here is written: the builder's writes are `builder.Apply` through its own routes.
func builderNodesProvider() composition.RuntimeProvider {
	return func(ctx context.Context, _ *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		workspaceID, _ := data.WorkspaceScope(ctx)
		machines := machinesFor(ctx)
		var out []*data.Record
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			add := func(place, path, kind, detail string) {
				id := app.ID + "/" + path
				out = append(out, &data.Record{ID: id, MachineID: "mch_builder_node", WorkspaceID: workspaceID, Values: map[string]any{
					"fld_id": id, "fld_app": app.ID, "fld_title": place, "fld_kind": kind, "fld_detail": detail, "fld_path": path,
					"fld_link": "/builder/" + url.PathEscape(app.ID) + "?" + url.Values{"node": {id}}.Encode(),
				}})
			}
			var owned []*domain.Machine
			for _, id := range app.Machines {
				if m := machines[id]; m != nil {
					owned = append(owned, m)
				}
			}
			for _, m := range owned {
				place := builderSectionData + "/" + builderPlaceName(m.Name)
				add(place, "machines/"+m.ID, "Machine", plural2(len(m.Fields), "field"))
				for _, f := range m.Fields {
					detail := string(f.Type)
					if f.Required {
						detail += ", required"
					}
					add(place+"/"+builderPlaceName(f.Name), "machines/"+m.ID+"/fields/"+f.ID, "Field", detail)
				}
			}
			for _, item := range app.AllNavigation {
				kind := "Screen"
				if item.Page != nil {
					kind = "Declared page"
				}
				add(builderSectionViews+"/"+builderPlaceName(item.Label), "navigation/"+item.ID, kind, item.Route)
			}
			add(builderSectionMenu+"/Application menu", "navigation", "Menu", strconv.Itoa(len(app.Navigation))+" entries in the menu")
			for _, m := range owned {
				if f := statusFieldOf(m); f != nil {
					add(builderSectionWorkflow+"/"+builderPlaceName(m.Name), "machines/"+m.ID+"/workflow", "Workflow", plural2(len(f.Options), "status"))
				}
			}
		}
		return out, nil
	}
}

// statusFieldOf is the Machine's status Field, nil when it has none.
func statusFieldOf(m *domain.Machine) *domain.Field {
	for i := range m.Fields {
		if m.Fields[i].Type == domain.FieldTypeStatus {
			return &m.Fields[i]
		}
	}
	return nil
}

// builderMode turns on the builder preview for a request that asks for it (`?builder=1`) and comes from a Workspace admin: the page is then drawn as
// always, with each part that is one piece of the Application's metadata naming itself (rendering.NodePathAttr) and its clicks reported to the frame.
// For anyone else the parameter is ignored, so a member who adds it to a link sees the ordinary page. It reads the membership the request already
// resolved (`membershipFor`), so it costs no query.
func builderMode(store *data.Store, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodGet && req.URL.Query().Get("builder") == "1" {
				userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
				workspaceID, _ := data.WorkspaceScope(req.Context())
				m, err := membershipFor(req.Context(), store, workspaceID, userID)
				// The same admission as requireWorkspaceAdmin: an admin's membership, or the configured admin identity that has none.
				if err == nil && ((m != nil && m.WorkspaceRole == "admin") || (m == nil && userID != "" && userID == cfg.AdminUserID)) {
					req = req.WithContext(rendering.WithBuilderMode(req.Context()))
				}
			}
			next.ServeHTTP(w, req)
		})
	}
}
