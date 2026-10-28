package web

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
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
func builderNodesProvider(store *data.Store, cfg config.Config) composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		selected, tab := l.SourceParam("builder_nodes", "node"), "Details"
		switch l.SourceParam("builder_nodes", "tab") {
		case "code":
			tab = "Code"
		case "edit":
			tab = "Edit"
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		machines := machinesFor(ctx)
		var out []*data.Record
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			add := func(place, path, kind, detail string, own ...[2]string) {
				id := app.ID + "/" + path
				tab := tab
				switch {
				case tab == "Edit" && kind == "Machine":
					tab = "Add" // a Machine's Edit tab is the form that adds a Field to it
				case tab == "Edit" && kind != "Field":
					tab = "Fixed" // the Edit tab has a form for a Field and a Machine only, and says so for anything else
				}
				out = append(out, &data.Record{ID: id, MachineID: "mch_builder_node", WorkspaceID: workspaceID, Values: map[string]any{
					"fld_id": id, "fld_app": app.ID, "fld_title": place, "fld_about": kind + " · " + detail, "fld_kind": kind, "fld_path": path,
					"fld_link": "/builder/" + url.PathEscape(app.ID) + "?" + url.Values{"node": {id}}.Encode(), "fld_tab": tab,
					"fld_code": codeOf(wantsCode(id, selected, tab), ctx, store, cfg, app.ID, path),
				}})
				for _, kv := range own {
					out[len(out)-1].Values[kv[0]] = kv[1]
				}
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
					add(place+"/"+builderPlaceName(f.Name), "machines/"+m.ID+"/fields/"+f.ID, "Field", detail,
						[2]string{"fld_name", f.Name}, [2]string{"fld_required", strconv.FormatBool(f.Required)}, [2]string{"fld_options", choiceLines(m, f)})
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
					ctx := rendering.WithBuilderMode(req.Context())
					if as := req.URL.Query().Get("as"); as != "" {
						// Previewed as a role the Application declares, and only one: any other name holds no role there, which fails closed.
						if app, ok := rendering.CurrentApplication(ctx); ok && slices.Contains(app.Roles, as) {
							ctx = rendering.WithPreviewRole(ctx, as)
						} else {
							ctx = rendering.WithPreviewRole(ctx, rendering.PreviewNoRole)
						}
					}
					req = req.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, req)
		})
	}
}

// codeOf is the text of the metadata file that declares a part, for the Code tab of the one part the request selected: the Application's file for its
// screens and menu, the Machine's own file for a Machine, a Field or a workflow. "" when the tab is not Code or the file cannot be found (the tab then
// says so); only the selected part's file is read.
func codeOf(wanted bool, ctx context.Context, store *data.Store, cfg config.Config, appID, path string) string {
	if !wanted || cfg.MetadataPath == "" {
		return ""
	}
	manifest, _, err := workspaceInstallation(ctx, store, cfg)
	if err != nil {
		return ""
	}
	var rel string
	if machineID, ok := strings.CutPrefix(path, "machines/"); ok {
		machineID, _, _ = strings.Cut(machineID, "/")
		rel, err = aiassist.FileMachineResolver{WorkspaceManifestPath: manifest}.MachineFile(machineID)
	} else {
		rel, err = installer.ApplicationFileOf(manifest, appID)
	}
	if err != nil {
		return ""
	}
	text, err := os.ReadFile(filepath.Join(filepath.Dir(manifest), rel))
	if err != nil {
		return ""
	}
	return string(text)
}

// wantsCode says whether a part's file is read: only the one part the request selected, and only on the Code tab, so a tree of hundreds of parts reads at
// most one file.
func wantsCode(id, selected, tab string) bool {
	return selected != "" && id == selected && tab == "Code"
}

// builderRolesProvider is the reader of the `builder_roles` source: for every Application, the ways the builder's preview can be looked at -- as the
// admin who is building it (no role), then as someone who holds each role the Application declares. Each says whether it is the one the request chose
// (`?as=`) and carries the builder's own address with that choice and the part and tab already chosen kept, so changing the viewer never loses your place.
func builderRolesProvider() composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		workspaceID, _ := data.WorkspaceScope(ctx)
		node, tab, chosen := l.SourceParam("builder_roles", "node"), l.SourceParam("builder_roles", "tab"), l.SourceParam("builder_roles", "as")
		var out []*data.Record
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			add := func(role, title string) {
				q := url.Values{}
				for k, v := range map[string]string{"node": node, "tab": tab, "as": role} {
					if v != "" {
						q.Set(k, v)
					}
				}
				state := "Other"
				if role == chosen {
					state = "Current"
				}
				id := app.ID + "/" + role
				out = append(out, &data.Record{ID: id, MachineID: "mch_builder_role", WorkspaceID: workspaceID, Values: map[string]any{
					"fld_id": id, "fld_app": app.ID, "fld_title": title, "fld_state": state,
					"fld_link": "/builder/" + url.PathEscape(app.ID) + "?" + q.Encode(),
				}})
			}
			add("", "As the builder")
			for _, role := range app.Roles {
				add(role, "As "+role)
			}
		}
		return out, nil
	}
}

// choiceLines is a Field's choices as the Edit tab's box shows them, one a line, each followed by "| category" when the Machine colours it.
func choiceLines(m *domain.Machine, f domain.Field) string {
	lines := make([]string, len(f.Options))
	for i, o := range f.Options {
		lines[i] = o
		if m.StatusCategories != nil && m.StatusCategories.Field == f.ID {
			if c, ok := m.StatusCategories.Values[o]; ok {
				lines[i] += " | " + string(c)
			}
		}
	}
	return strings.Join(lines, "\n")
}
