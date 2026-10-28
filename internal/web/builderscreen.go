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
	builderSectionChecks   = "Needs checking"
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
	return func(ctx context.Context, l *composition.Loader, viewer domain.Actor) ([]*data.Record, error) {
		selected, tab := l.SourceParam("builder_nodes", "node"), "Details"
		switch l.SourceParam("builder_nodes", "tab") {
		case "code":
			tab = "Code"
		case "edit":
			tab = "Edit"
		case "ask":
			tab = "Ask"
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		machines := machinesFor(ctx)
		var out []*data.Record
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			add := func(place, path, kind, detail string, own ...[2]string) {
				id := app.ID + "/" + path
				tab := tab
				switch {
				case tab == "Edit" && (kind == "Screen" || kind == "Declared page"):
					tab = "MenuItem" // a screen the menu lists: its name, its place, taking it out
				case tab == "Edit" && kind == "Menu":
					tab = "MenuAdd" // the menu itself: a new item
				case tab == "Edit" && kind == "Permission":
					tab = "Access" // a Permission's Edit tab is the roles that may do what it governs
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
						[2]string{"fld_name", f.Name}, [2]string{"fld_required", strconv.FormatBool(f.Required)}, [2]string{"fld_options", choiceLines(m, f)}, [2]string{"fld_editable_by", strings.Join(f.EditableBy, "\n")}, [2]string{"fld_show_when", showWhenText(f.ShowWhen)})
				}
			}
			for _, m := range owned {
				for _, p := range m.Permissions {
					add(builderSectionData+"/"+builderPlaceName(m.Name)+"/Who may "+builderPlaceName(p.Action), "machines/"+m.ID+"/permissions/"+p.ID, "Permission",
						strings.Join(p.Roles, ", "), [2]string{"fld_roles", strings.Join(p.Roles, "\n")})
				}
			}
			for i, item := range app.AllNavigation {
				kind := "Screen"
				if item.Page != nil {
					kind = "Declared page"
				}
				add(builderSectionViews+"/"+builderPlaceName(item.Label), "navigation/"+item.ID, kind, item.Route,
					[2]string{"fld_name", item.Label}, [2]string{"fld_position", strconv.Itoa(i + 1)}, [2]string{"fld_count", strconv.Itoa(len(app.AllNavigation))})
			}
			for i, c := range builderChecks(app, owned) {
				add(builderSectionChecks+"/"+builderPlaceName(c.Text), "checks/"+strconv.Itoa(i+1), "Check", c.Detail)
			}
			add(builderSectionMenu+"/Application menu", "navigation", "Menu", strconv.Itoa(len(app.Navigation))+" entries in the menu")
			for _, m := range owned {
				if f := statusFieldOf(m); f != nil {
					add(builderSectionWorkflow+"/"+builderPlaceName(m.Name), "machines/"+m.ID+"/workflow", "Workflow", plural2(len(f.Options), "status"))
				}
			}
		}
		// The Edit tab starts from the Field as the viewer's draft leaves it, so a form shows what is staged and a change staged earlier can be undone.
		if tab == "Edit" {
			appID, rest, _ := strings.Cut(selected, "/")
			if p := strings.Split(rest, "/"); len(p) == 4 && p[0] == "machines" && p[2] == "fields" {
				if raw, err := store.ListAppDrafts(ctx, workspaceID, viewer.ID, "the field form"); err == nil && raw[appID] != nil {
					if app, err := stagedApplication(ctx, store, cfg, appID, viewer.ID); err == nil {
						if m := stagedMachine(app, p[1]); m != nil {
							for _, f := range m.Fields {
								if f.ID != p[3] {
									continue
								}
								for _, r := range out {
									if r.ID == selected {
										r.Values["fld_name"], r.Values["fld_required"] = f.Name, strconv.FormatBool(f.Required)
										r.Values["fld_options"], r.Values["fld_editable_by"] = stagedChoiceLines(m, f), strings.Join(f.EditableBy, "\n")
										r.Values["fld_show_when"] = ""
										if f.ShowWhen != nil {
											r.Values["fld_show_when"] = f.ShowWhen.Field + ":" + f.ShowWhen.Equals
										}
									}
								}
							}
						}
					}
				}
			}
		}
		return out, nil
	}
}

// stagedChoiceLines is choiceLines for a Field as the draft leaves it.
func stagedChoiceLines(m *aiassist.GeneratedMachine, f aiassist.GeneratedField) string {
	lines := make([]string, len(f.Options))
	for i, o := range f.Options {
		lines[i] = o
		if m.StatusCategories != nil && m.StatusCategories.Field == f.ID {
			for _, v := range m.StatusCategories.Values {
				if v.Option == o {
					lines[i] += " | " + v.Category
				}
			}
		}
	}
	return strings.Join(lines, "\n")
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

// builderCheck is one thing about an Application that loads and runs but is worth a look (the mockups' "1 hal perlu dicek"): not a fault -- a fault does
// not load -- but a thing declared and doing nothing. Detail is what to do about it, in a line.
type builderCheck struct{ Text, Detail string }

// builderChecks lists what is worth a look in an Application, in a stable order: a status choice the Machine does not give a colour to, and a role that
// no permission, transition or event of the Application names (so holding it changes nothing). Each is derived from the metadata alone, so the list is
// never a second record of anything.
func builderChecks(app domain.Application, owned []*domain.Machine) []builderCheck {
	var out []builderCheck
	for _, m := range owned {
		sf := statusFieldOf(m)
		if sf == nil || m.StatusCategories == nil || m.StatusCategories.Field != sf.ID {
			continue
		}
		var plain []string
		for _, o := range sf.Options {
			if _, ok := m.StatusCategories.Values[o]; !ok {
				plain = append(plain, o)
			}
		}
		if len(plain) > 0 {
			out = append(out, builderCheck{Text: builderPlaceName(m.Name) + ": no colour for " + strings.Join(plain, ", "),
				Detail: "These choices of " + sf.Name + " are drawn plain while the others carry a colour; write them as `Choice | done` in the field's Edit tab when they are added."})
		}
	}
	named := map[string]bool{}
	for _, m := range owned {
		for _, p := range m.Permissions {
			for _, r := range p.Roles {
				named[r] = true
			}
		}
		for _, t := range m.Transitions {
			for _, r := range t.Roles {
				named[r] = true
			}
		}
		for _, e := range m.Events {
			if e.Then.Notify != nil {
				named[e.Then.Notify.RecipientRole] = true
			}
		}
		for _, f := range m.Fields {
			for _, r := range f.EditableBy {
				named[r] = true
			}
		}
	}
	for _, r := range app.Roles {
		if !named[r] {
			out = append(out, builderCheck{Text: "Role " + r + " is named by nothing", Detail: "No permission, transition, event or field of this application names the role " + r + ", so holding it changes nothing."})
		}
	}
	return out
}

// showWhenText is a Field's `show_when:` as the Edit tab's picker holds it, "<field>:<option>", empty for a Field asked always.
func showWhenText(c *domain.FieldCondition) string {
	if c == nil {
		return ""
	}
	return c.Field + ":" + c.Equals
}
