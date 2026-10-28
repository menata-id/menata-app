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
	"time"

	"menata.app/internal/aiassist"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/execution"
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
	builderSectionRules    = "Rules"
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
		case "view":
			tab = "View"
		case "test":
			tab = "Test"
		case "rules":
			tab = "Rules"
		}
		rec := l.SourceParam("builder_nodes", "rec")
		workspaceID, _ := data.WorkspaceScope(ctx)
		machines := machinesFor(ctx)
		var out []*data.Record
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			add := func(place, path, kind, detail string, own ...[2]string) {
				id := app.ID + "/" + path
				tab := tab
				switch {
				case tab == "Rules" && kind == "Machine":
					tab = "RuleNew" // a rule started from an example
				case tab == "Rules":
					tab = "Fixed"
				case tab == "Test" && (kind == "Rule" || kind == "Fixed rule"):
					tab = "RuleTest" // a rule tried on a real record, in words
				case tab == "Test":
					tab = "Fixed"
				case tab == "View" && kind == "List":
					tab = "ListEdit" // a list the assistant wrote: its columns, order, chips and search
				case tab == "View":
					tab = "Fixed"
				case tab == "Edit" && (kind == "Screen" || kind == "Declared page" || kind == "List"):
					tab = "MenuItem" // a screen the menu lists: its name, its place, taking it out
				case tab == "Edit" && kind == "Menu":
					tab = "MenuAdd" // the menu itself: a new item
				case tab == "Edit" && kind == "Rule":
					tab = "Switch" // a rule: switched on or off
				case tab == "Edit" && kind == "Workflow":
					tab = "Flow" // the lifecycle: a new status, a new move between statuses
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
				add(place, "machines/"+m.ID, "Machine", plural2(len(m.Fields), "field"), [2]string{"fld_list_machine", m.ID})
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
				listMachine, list := aiassist.ListOf(item.ID, item.Page, machines)
				own := [][2]string{}
				if list != nil {
					kind = "List"
					if lm := machines[listMachine]; lm != nil {
						names := map[string]string{}
						for _, f := range lm.Fields {
							names[f.ID] = f.Name
						}
						own = listValues(lm.ID, names, list)
					}
				}
				add(builderSectionViews+"/"+builderPlaceName(item.Label), "navigation/"+item.ID, kind, item.Route, append(own,
					[2]string{"fld_name", item.Label}, [2]string{"fld_position", strconv.Itoa(i + 1)}, [2]string{"fld_count", strconv.Itoa(len(app.AllNavigation))},
					[2]string{"fld_parent", item.Parent}, [2]string{"fld_visible_to", strings.Join(item.VisibleTo, "\n")}, [2]string{"fld_badge", item.Badge})...)
			}
			if len(owned) > 0 {
				ids := make([]string, len(owned))
				for i, m := range owned {
					ids[i] = m.ID
				}
				latest, _ := store.LatestEventRuns(ctx, ids)
				for _, m := range owned {
					for _, e := range m.Events {
						last := "never ran"
						if r, ok := latest[m.ID+"/"+e.ID]; ok {
							last = "last " + r.Outcome + " " + r.RanAt.Format("2006-01-02 15:04")
						}
						state := "on"
						if e.Disabled {
							state = "off"
						}
						kind := "Rule"
						if !aiassist.EventDescribed(e) {
							kind = "Fixed rule" // carried through unchanged by the builder: it is shown and its runs are read, but it has no switch here yet
						}
						add(builderSectionRules+"/"+builderPlaceName(m.Name)+"/"+builderPlaceName(ruleWords(m, e)), "machines/"+m.ID+"/events/"+e.ID, kind, "what happens when something changes · "+state+" · "+last,
							[2]string{"fld_list_machine", m.ID}, [2]string{"fld_enabled", strconv.FormatBool(!e.Disabled)})
					}
				}
			}
			for i, c := range builderChecks(app, owned) {
				add(builderSectionChecks+"/"+builderPlaceName(c.Text), "checks/"+strconv.Itoa(i+1), "Check", c.Detail)
			}
			add(builderSectionMenu+"/Application menu", "navigation", "Menu", strconv.Itoa(len(app.Navigation))+" entries in the menu")
			for _, m := range owned {
				if f := statusFieldOf(m); f != nil {
					add(builderSectionWorkflow+"/"+builderPlaceName(m.Name), "machines/"+m.ID+"/workflow", "Workflow", plural2(len(f.Options), "status")+" and the moves between them",
						[2]string{"fld_list_machine", m.ID})
				}
			}
		}
		// The Test tab of a rule: what the rule would do for the record the request names, in words. It reads the record and the people the rule would tell, and writes nothing.
		if tab == "Test" && rec != "" {
			appID, rest, _ := strings.Cut(selected, "/")
			if p := strings.Split(rest, "/"); len(p) == 4 && p[0] == "machines" && p[2] == "events" {
				if m := machinesFor(ctx)[p[1]]; m != nil {
					for _, e := range m.Events {
						if e.ID != p[3] {
							continue
						}
						record, err := store.GetRecord(ctx, m.ID, rec)
						said := "That record is not one of this Machine's."
						if err == nil {
							said = strings.Join(execution.DryRun(ctx, store, machines, m, record, e, time.Now()), "\n")
						}
						for _, r := range out {
							if r.ID == selected {
								r.Values["fld_code"] = said
							}
						}
					}
				}
			}
			_ = appID
		}
		// A menu item's Edit tab likewise starts from the menu as the viewer's draft leaves it.
		if tab == "Edit" {
			appID, rest, _ := strings.Cut(selected, "/")
			if item, ok := strings.CutPrefix(rest, "navigation/"); ok && item != "" {
				if raw, err := store.ListAppDrafts(ctx, workspaceID, viewer.ID, "the menu item form"); err == nil && raw[appID] != nil {
					if app, err := stagedApplication(ctx, store, cfg, appID, viewer.ID); err == nil {
						for at, n := range app.Navigation {
							if n.ID != item {
								continue
							}
							for _, r := range out {
								if r.ID == selected {
									r.Values["fld_name"], r.Values["fld_position"], r.Values["fld_count"] = n.Label, strconv.Itoa(at+1), strconv.Itoa(len(app.Navigation))
									r.Values["fld_parent"], r.Values["fld_visible_to"], r.Values["fld_badge"] = n.Parent, strings.Join(n.VisibleTo, "\n"), n.Badge
									if n.List != nil {
										if sm := stagedMachine(app, n.MachineID); sm != nil {
											names := map[string]string{}
											for _, f := range sm.Fields {
												names[f.ID] = f.Name
											}
											for _, kv := range listValues(sm.ID, names, n.List) {
												r.Values[kv[0]] = kv[1]
											}
										}
									}
								}
							}
						}
					}
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
		node, tab, chosen, try := l.SourceParam("builder_roles", "node"), l.SourceParam("builder_roles", "tab"), l.SourceParam("builder_roles", "as"), l.SourceParam("builder_roles", "try")
		var out []*data.Record
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			add := func(role, title string) {
				q := url.Values{}
				for k, v := range map[string]string{"node": node, "tab": tab, "as": role, "try": try} {
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

// listValues are what the View tab's form starts from for a list: the Machine it lists, its columns by their Field names (one a line, in order), how it is ordered, the status
// its chips count by and the text Fields its search looks in.
func listValues(machineID string, names map[string]string, l *aiassist.GeneratedList) [][2]string {
	var cols []string
	for _, id := range l.Columns {
		cols = append(cols, names[id])
	}
	dir := l.Direction
	if dir == "" {
		dir = "desc"
	}
	sort := l.SortField
	if sort == "" {
		sort = "created_at"
	}
	density := l.Density
	if density == "" {
		density = "regular"
	}
	return [][2]string{{"fld_list_machine", machineID}, {"fld_columns", strings.Join(cols, "\n")}, {"fld_sort", sort}, {"fld_direction", dir}, {"fld_filter", l.FilterField}, {"fld_search", strings.Join(l.SearchFields, "\n")},
		{"fld_density", density}, {"fld_group_by", l.GroupBy}, {"fld_row_tone", l.RowTone}}
}

// builderStepsProvider is the reader of the `builder_steps` source: for the workflow part the request selects (`machines/<machine>/workflow` of an Application of this
// Workspace), one record per status of the Machine's lifecycle -- its meaning, how many records stand in it, the edges that leave it and who may take each, and what is
// wrong with it. The workflow is composition.ResolveProcess's reading of `transitions:` and `status_categories:`; nothing here is stored. Counts are one cheap statement per
// status, asked only for the selected workflow.
func builderStepsProvider(store *data.Store) composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		selected, try, as := l.SourceParam("builder_steps", "node"), l.SourceParam("builder_steps", "try"), l.SourceParam("builder_steps", "as")
		appID, rest, _ := strings.Cut(selected, "/")
		parts := strings.Split(rest, "/")
		if len(parts) != 3 || parts[0] != "machines" || parts[2] != "workflow" {
			return nil, nil
		}
		owned := false
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			owned = owned || (app.ID == appID && slices.Contains(app.Machines, parts[1]))
		}
		m := machinesFor(ctx)[parts[1]]
		if !owned || m == nil {
			return nil, nil
		}
		counts := map[string]int{}
		if field := m.StatusField(); field != "" {
			for _, f := range m.Fields {
				if f.ID != field {
					continue
				}
				for _, o := range f.Options {
					n, err := store.CountRecordsSelect(ctx, m.ID, "workflow:"+o, []data.FieldPredicate{{Field: field, Value: o}})
					if err != nil {
						return nil, err
					}
					counts[o] = n
				}
			}
		}
		g, ok := composition.ResolveProcess(m, counts)
		if !ok {
			return nil, nil
		}
		edge := map[string]composition.ProcessEdge{}
		for _, e := range g.Edges {
			edge[e.ID] = e
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		var out []*data.Record
		for _, n := range g.Nodes {
			var leaves []string
			for _, id := range n.Out {
				e := edge[id]
				line := e.Name + " → " + e.To
				if len(e.Roles) > 0 {
					line += " · " + strings.Join(e.Roles, ", ")
				}
				if e.ActorField != "" {
					line += " · the person in " + e.ActorField
				}
				leaves = append(leaves, line)
			}
			var issues []string
			for _, i := range g.Issues {
				if i.Status == n.Status {
					issues = append(issues, i.Words)
				}
			}
			category := "Unmarked"
			if n.Category != "" {
				category = strings.ToUpper(n.Category[:1]) + n.Category[1:]
			}
			// Trying the flow writes nothing: standing at `try` as the role `as`, a status is "you are here", or one a move you may take leads to.
			words := plural2(n.Count, "record")
			switch {
			case try != "" && n.Status == try:
				words += " · you are here"
			case try != "":
				for _, e := range g.Edges {
					if e.From == try && e.To == n.Status && edgeTakenBy(e, as) {
						words += " · " + e.Name + " gets you here"
						break
					}
				}
			}
			q := url.Values{"node": {selected}, "try": {n.Status}}
			if as != "" {
				q.Set("as", as)
			}
			out = append(out, &data.Record{ID: selected + "#" + n.Status, MachineID: "mch_builder_step", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": selected + "#" + n.Status, "fld_node": selected, "fld_title": n.Status, "fld_category": category,
				"fld_count": words, "fld_edges": strings.Join(leaves, "\n"), "fld_issue": strings.Join(issues, "\n"),
				"fld_link": "/builder/" + url.PathEscape(appID) + "?" + q.Encode(),
			}})
		}
		return out, nil
	}
}

// edgeTakenBy says whether someone holding the Application role `as` could take a move: a move that names no roles is anyone's, one that names some is theirs who hold one,
// and with no role chosen (the builder looking) every move can be taken. A move the runtime writes itself is no one's; one performed by another Action than `edit` keeps its rule in that Action's Permission, which a trial does not read, so it is offered. A move that names a person Field of the record is takeable by the person in it, which a trial has no
// record to know, so it is offered.
func edgeTakenBy(e composition.ProcessEdge, as string) bool {
	if e.Action == "" {
		return false // an edge the runtime itself writes ("System") is taken by no one
	}
	return as == "" || len(e.Roles) == 0 || slices.Contains(e.Roles, as)
}

// ruleWords says a rule in a line a person would: when it runs and what it does.
func ruleWords(m *domain.Machine, e domain.Event) string {
	name := func(id string) string {
		for _, f := range m.Fields {
			if f.ID == id {
				return f.Name
			}
		}
		return id
	}
	var when string
	switch {
	case e.Schedule != nil:
		when = name(e.Schedule.DateField) + " is " + e.Schedule.When
	case e.OnCreate:
		when = "a record is created"
	case e.WhenEquals != "":
		when = name(e.On) + " becomes " + e.WhenEquals
	default:
		when = name(e.On) + " changes"
	}
	var then string
	switch e.Then.Name {
	case domain.ServiceSendNotification:
		then = "tell someone"
		if n := e.Then.Notify; n != nil {
			switch {
			case n.RecipientRole != "":
				then = "tell everyone who is " + n.RecipientRole
			case n.RecipientGroupField != "":
				then = "tell the group in " + name(n.RecipientGroupField)
			default:
				then = "tell the person in " + name(n.RecipientField)
			}
		}
	case domain.ServiceLogActivity:
		then = "write an activity entry"
	case domain.ServiceSetField:
		then = "set a field"
		if e.Then.Set != nil {
			then = "set " + name(e.Then.Set.TargetField) + " to " + e.Then.Set.Value
		}
	case domain.ServiceRollupParentStatus:
		then = "update the parent's status"
	default:
		then = e.Then.Name
	}
	return "When " + when + ": " + then
}

// builderRunsProvider is the reader of the `builder_runs` source: for the rule the request selects (`machines/<machine>/events/<event>` of an Application of this Workspace), its last
// runs, newest first. It is what tells a rule that is quiet from one that is broken: a run says whether the rule ran, had nobody or nothing to act on, or failed, and why.
func builderRunsProvider(store *data.Store) composition.RuntimeProvider {
	return func(ctx context.Context, l *composition.Loader, _ domain.Actor) ([]*data.Record, error) {
		selected := l.SourceParam("builder_runs", "node")
		appID, rest, _ := strings.Cut(selected, "/")
		parts := strings.Split(rest, "/")
		if len(parts) != 4 || parts[0] != "machines" || parts[2] != "events" {
			return nil, nil
		}
		owned := false
		for _, app := range rendering.CurrentWorkspace(ctx).Applications {
			owned = owned || (app.ID == appID && slices.Contains(app.Machines, parts[1]))
		}
		if !owned {
			return nil, nil
		}
		runs, err := store.RecentEventRuns(ctx, parts[1], parts[3], 30)
		if err != nil {
			return nil, err
		}
		workspaceID, _ := data.WorkspaceScope(ctx)
		var out []*data.Record
		for i, r := range runs {
			outcome := strings.ToUpper(r.Outcome[:1]) + r.Outcome[1:]
			id := selected + "#" + strconv.Itoa(i)
			out = append(out, &data.Record{ID: id, MachineID: "mch_builder_run", WorkspaceID: workspaceID, Values: map[string]any{
				"fld_id": id, "fld_node": selected, "fld_title": r.RanAt.Format("2006-01-02 15:04"), "fld_outcome": outcome, "fld_record": "record " + r.RecordID, "fld_detail": r.Detail,
			}})
		}
		return out, nil
	}
}
