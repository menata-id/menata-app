package conformance

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
	"menata.app/internal/installer"
	"menata.app/internal/ir"
	"menata.app/internal/metadata"
	"menata.app/internal/registry"
)

// The two gates here are the `page:` block's post-primitive gates (007 §12.4, §15.3), installed after the
// primitive and its first consumers exist -- build, migrate, then gate -- so each tests a failure class that
// can now happen.
//
// **Probe, not prediction (2026-10-07).** With `validatePages` disabled at its one call in
// `metadata.LoadApplication`, the suites for `metadata`, `conformance`, `composition` and `ir` were run: exactly
// one test failed, `metadata.TestPage_faultsAreRefusedAtLoad`, a unit test over a fixture. No gate over the
// *installed* manifests noticed, so a Workspace could have shipped a `page:` the loader no longer checked and
// the suite would have stayed green until a visitor got a 500. The first gate closes that.

// TestEveryInstalledPageLowersAndValidates sweeps every installed Workspace, the way the loader scans the
// directory, and checks each declared `page:` against `ir.Validate` and the Component validators itself.
//
// It is not redundant with `metadata.validatePages` being a load error, for the reason
// `TestEveryCastRoleProvidesItsEngineDatasets` states: a load error fires only if the loader still makes the
// call. This holds the *outcome* against the real manifests -- and fails if no Workspace declares a page, so
// it cannot pass by measuring nothing.
func TestEveryInstalledPageLowersAndValidates(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	ir.RegisterComponentTypes(registry.ComponentTypeNames())
	ir.RegisterSlottedComponentTypes(registry.SlottedComponentTypeNames())

	checked := 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		datasets := map[string]domain.Dataset{}
		owner := map[string]*domain.Machine{}
		for _, m := range ws.Machines {
			for _, ds := range m.Datasets {
				datasets[ds.ID] = ds
				owner[ds.ID] = m
			}
		}
		for _, app := range ws.Applications {
			for _, item := range app.AllNavigation {
				if item.Page == nil {
					continue
				}
				checked++
				where := slug + "/" + app.ID + "/" + item.ID
				if want := "/pages/" + item.ID; item.Route != want {
					t.Errorf("%s: route is %q but a page is rendered at %q", where, item.Route, want)
				}
				res := metadata.PlaceholderResolver(app.LinkTargets(ws.Navigation), datasets)
				rows, records, total := res.Rows, res.Records, res.Total
				declared := func(b domain.PageBinding) {
					if _, ok := datasets[b.Dataset]; !ok {
						t.Errorf("%s: binding names dataset %q, which this Workspace does not declare -- the page would answer 500 on every visit", where, b.Dataset)
					}
				}
				res.Source = func(id string) (string, bool) {
					ds, ok := datasets[id]
					if !ok {
						t.Errorf("%s: list_of names dataset %q, which this Workspace does not declare -- the link would point nowhere", where, id)
					}
					return ds.Source, ok
				}
				res.Rows = func(b domain.PageBinding) ([]ir.Row, error) { declared(b); return rows(b) }
				res.Records = func(b domain.PageBinding) (ir.RecordSet, error) { declared(b); return records(b) }
				// The placeholder total answers any Measure id, so whether a total Metric's Measure -- or its
				// hint_figure's -- is one the Dataset declares is asked here, independently of the loader.
				res.Total = func(b domain.PageBinding) (string, error) {
					declared(b)
					if ds, ok := datasets[b.Dataset]; ok && !datasetDeclaresMeasure(ds, b.Measure) {
						t.Errorf("%s: a total asks dataset %q for measure %q, which it does not declare -- the figure would be a silent 0", where, b.Dataset, b.Measure)
					}
					return total(b)
				}
				tree, err := ir.Lower(*item.Page, res)
				if err != nil {
					t.Errorf("%s: does not lower: %v", where, err)
					continue
				}
				for _, msg := range ir.Validate(tree) {
					t.Errorf("%s: %s", where, msg)
				}
				for _, msg := range recordsBindingProblems(*item.Page, owner) {
					t.Errorf("%s: %s", where, msg)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no installed Workspace declares a `page:` -- this gate is measuring nothing, which is how a loader that stopped validating them would go unseen")
	}
}

// TestDeclaredPagePipelineHasNoPerScreenBranch is the `page:` twin of `TestLayoutRenderersHaveNoPerScreenBranch`,
// and the only real test of whether the declared-page path is a primitive: a generic handler, lowering or
// renderer that must know *which screen* it serves is one screen's shape with a new name, which is 007 §12.4's
// MUST NOT from the other side.
//
// It covers every file that touches a declared page between the YAML and the pixels, comments stripped first
// (their own explanations name the ids they must not contain). A Dataset or Field id is held out as well as a
// navigation, Machine and Application id: the Binding is the one place a page names data, and it names it in
// YAML, so a `ds_` or `fld_` literal in this pipeline means a screen's data got hardcoded beside its
// declaration.
func TestDeclaredPagePipelineHasNoPerScreenBranch(t *testing.T) {
	files := []string{
		"internal/domain/page.go",
		"internal/ir/page.go",
		"internal/metadata/page.go",
		"internal/composition/declaredpage.go",
		"internal/web/declaredpage.go",
		"internal/rendering/declaredscreen.templ",
	}
	comment := regexp.MustCompile(`(?m)//.*$`)
	for pattern, why := range map[string]string{
		`"nav_[a-z0-9_]+"`: "a navigation id -- the pipeline would be serving one screen",
		`"mch_[a-z0-9_]+"`: "a Machine id, which is an Application's vocabulary",
		`"app_[a-z0-9_]+"`: "an Application id, never an identity to branch on (2026-09-28)",
		`"ds_[a-z0-9_]+"`:  "a Dataset id -- a Binding names its Dataset in YAML, so this is a screen's data hardcoded beside its declaration",
		`"fld_[a-z0-9_]+"`: "a Field id, which composition resolves through declarations",
	} {
		re := regexp.MustCompile(pattern)
		for _, f := range files {
			body, err := os.ReadFile(filepath.Join(repoRoot(), f))
			if err != nil {
				t.Fatalf("read %s: %v -- the declared-page pipeline moved; update this list rather than letting it measure nothing", f, err)
			}
			if m := re.FindString(comment.ReplaceAllString(string(body), "")); m != "" {
				t.Errorf("%s contains %s: %s", f, m, why)
			}
		}
	}
}

// TestInstalledPageDatasetsExistInTheLibraryTemplate holds the half of "installing copies" that the load
// error cannot: that error fires for a Workspace whose own copy lacks a bound Dataset, and says nothing
// about the *template* the next Workspace will be copied from. A page's Dataset reached the two Nana
// Workspaces' copies on 2026-10-07 while `metadata/document.yaml` had lost it (dfbc09c removed it with the
// Dashboard that was its last Go reader), so anyone installing Document Approval afresh would have been
// handed a template that could not carry the page its siblings run.
//
// Scope, stated so a green run is not over-read: it checks a Dataset's presence by id on the library Machine
// of the same id. A Machine that is not a library one (generated, or renamed on install) is skipped, and it
// does not check that the library *also* offers the page -- only that nothing the page needs is missing.
func TestInstalledPageDatasetsExistInTheLibraryTemplate(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	templates, err := installer.Templates(filepath.Join(repoRoot(), "metadata"))
	if err != nil {
		t.Fatalf("read the template library: %v", err)
	}
	library := map[string]*domain.Machine{}
	for _, tm := range templates {
		for _, m := range tm.Machines {
			library[m.Machine.ID] = m.Machine
		}
	}

	checked := 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		owner := map[string]string{} // dataset id -> Machine id declaring it, in this Workspace
		for _, m := range ws.Machines {
			for _, ds := range m.Datasets {
				owner[ds.ID] = m.ID
			}
		}
		for _, app := range ws.Applications {
			for _, item := range app.AllNavigation {
				if item.Page == nil {
					continue
				}
				for _, b := range pageBindings(*item.Page) {
					lib, ok := library[owner[b.Dataset]]
					if !ok || !isCopyOf(ws, owner[b.Dataset], lib) {
						continue
					}
					checked++
					found := false
					for _, ds := range lib.Datasets {
						found = found || ds.ID == b.Dataset
					}
					if !found {
						t.Errorf("%s/%s binds %q, which the library's %s does not declare -- a Workspace installing that template afresh could not take this page",
							slug, item.ID, b.Dataset, lib.ID)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no installed page binds a Dataset owned by a library Machine -- this gate is measuring nothing")
	}
}

// isCopyOf says whether the Workspace's Machine of that id is an installed copy of the library one, not a different Machine that happens
// to share its id (an assistant-generated one: "mch_task" with its own fld_task_* Fields). A copy declares nothing the library Machine does
// not -- installing is a byte-identical copy unless an id collides, and then the id is renamed -- so a Field the library lacks means it is not.
// Without this the gate read a generated Machine as the library's and demanded Datasets of it that only the generated one has (2026-10-25).
func isCopyOf(ws domain.Workspace, id string, lib *domain.Machine) bool {
	have := map[string]bool{}
	for _, f := range lib.Fields {
		have[f.ID] = true
	}
	for _, m := range ws.Machines {
		if m.ID != id {
			continue
		}
		for _, f := range m.Fields {
			if !have[f.ID] {
				return false
			}
		}
	}
	return true
}

func pageBindings(n domain.PageNode) []domain.PageBinding {
	var out []domain.PageBinding
	if n.Binding != nil {
		out = append(out, *n.Binding)
	}
	for _, c := range n.Children {
		out = append(out, pageBindings(c)...)
	}
	return out
}

// recordsBindingProblems re-checks every `rows: records` Binding of a page against the Machine that owns its
// Dataset, **without calling `metadata`'s own check**: the probe of 2026-10-08 disabled that call and only a
// unit test over a fixture failed, so a Workspace shipping a list the loader no longer vetted would have stayed
// green until a visitor got a 500 or a blank cell.
//
// It holds the two things a list can be wrong about that `ir.Lower` over a placeholder cannot see: the Dataset
// selects records, and every `from:` role under the Binding is one the Machine declares in `card_fields` whose Field is not a reference (007 §20).
func recordsBindingProblems(page domain.PageNode, owner map[string]*domain.Machine) []string {
	var out []string
	var walk func(n domain.PageNode)
	walk = func(n domain.PageNode) {
		if n.Binding != nil && n.Binding.Rows == domain.PageRowsRecords {
			m := owner[n.Binding.Dataset]
			if m == nil {
				out = append(out, "records binding names dataset "+n.Binding.Dataset+", which no Machine of this Workspace declares")
			} else {
				var ds domain.Dataset
				for _, d := range m.Datasets {
					if d.ID == n.Binding.Dataset {
						ds = d
					}
				}
				if ds.Select != domain.SelectRecords {
					out = append(out, "records binding over "+ds.ID+", which is an aggregate")
				}
				out = append(out, groupProblems(n.Binding.Group, ds, m, len(n.Children))...)
				out = append(out, eachProblems(n.Children, m)...)
				for _, role := range fromRolesUnder(n.Children) {
					// A Field read by id (`field:`, `tone:`, `steps:`, 2026-10-17): a Field of the Machine, never a group or
					// a file; a reference among them is read bounded, by the ids the listed records name.
					if id, byField := domain.PageFieldOfRole(role); byField {
						// A file Field is read only through file:/filehref: (its name with its size, its address), which in turn
						// are only for a file Field; every other role refuses a file (its value is a storage key) and a group.
						isFileRole := strings.HasPrefix(role, "file:") || strings.HasPrefix(role, "filehref:")
						switch f, ok := m.FieldByID(id); {
						case !ok:
							out = append(out, "from: "+role+" -- "+m.ID+" has no such Field")
						case isFileRole && f.Type != domain.FieldTypeFile:
							out = append(out, "from: "+role+" -- a "+string(f.Type)+" Field has no file to name")
						case !isFileRole && (f.Type == domain.FieldTypeGroup || f.Type == domain.FieldTypeFile):
							out = append(out, "from: "+role+" -- a "+string(f.Type)+" Field is not shown on a page")
						}
						continue
					}
					fieldID := m.CardFieldFor(domain.CardFieldRole(role))
					f, ok := m.FieldByID(fieldID)
					switch {
					case fieldID == "" || !ok:
						out = append(out, "from: "+role+" -- "+m.ID+" declares no such card_fields role")
					case f.IsReference() && f.RelatedMachine != domain.UserMachineID && domain.CardFieldRole(role) != domain.CardFieldRoleContainer:
						out = append(out, "from: "+role+" -- the Field behind it is a reference, which would read a whole Machine (007 §20); only a person and a container are read bounded")
					}
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(page)
	return out
}

// groupProblems holds a `group:` binding to what grouping needs, stated again here and not by calling metadata: it
// groups by the one dimension built, the Machine names the Field to bucket by, the Dataset closes a window on that
// Field with exactly one gte and one lte (the days a page draws are read from them), and the Collection holds the
// heading template and the item template and nothing else.
func groupProblems(group string, ds domain.Dataset, m *domain.Machine, children int) []string {
	if group == "" {
		return nil
	}
	var out []string
	if group != domain.PageGroupDate {
		return []string{"group: " + group + " is not a dimension a records binding groups by"}
	}
	field := m.CardFieldFor(domain.CardFieldRoleDate)
	if field == "" {
		return []string{"group: date -- " + m.ID + " declares no date card_fields role"}
	}
	lo, hi := 0, 0
	if ds.Where != nil && len(ds.Where.Any) == 0 {
		for _, c := range ds.Where.All {
			if c.Field == field && c.Op == expression.OpGreaterOrEqual {
				lo++
			}
			if c.Field == field && c.Op == expression.OpLessOrEqual {
				hi++
			}
		}
	}
	if lo != 1 || hi != 1 {
		out = append(out, "group: date -- "+ds.ID+" must close its window on "+field+" with exactly one gte and one lte in an all: ("+strconv.Itoa(lo)+" and "+strconv.Itoa(hi)+")")
	}
	if children != 2 {
		out = append(out, "group: date -- a grouped Collection holds a heading template and an item template, and has "+strconv.Itoa(children)+" children")
	}
	return out
}

// eachProblems holds an `each: tags` node to what it needs, stated again here and not by calling metadata: its Machine
// declares `card_tags:`, and it reads only the tag's two roles. Such a node's `from:` is not the record's, so
// fromRolesUnder leaves its subtree out.
func eachProblems(nodes []domain.PageNode, m *domain.Machine) []string {
	var out []string
	for _, n := range nodes {
		if n.Each != "" {
			if n.Each != domain.PageEachTags {
				out = append(out, "each: "+n.Each+" is not a list a record owns")
			}
			if m.CardTags == nil {
				out = append(out, "each: "+n.Each+" -- "+m.ID+" declares no card_tags:")
			}
			for _, role := range n.From {
				if role != string(domain.CardFieldRoleTitle) && role != string(domain.CardFieldRoleColor) {
					out = append(out, "each: "+n.Each+" -- from: "+role+" is not a role of a tag")
				}
			}
		}
		out = append(out, eachProblems(n.Children, m)...)
	}
	return out
}

func fromRolesUnder(nodes []domain.PageNode) []string {
	var out []string
	for _, n := range nodes {
		if n.Each != "" {
			continue
		}
		for prop, role := range n.From {
			// The reserved word is the record's route, not a Projection role; it is judged where it is used
			// (a link's href) by TestEveryInstalledLinkNamesItsDestinationOnce, and by ir.Lower for any other prop.
			if domain.PageDerivedRole(role) || domain.PageBucketRole(role) {
				continue
			}
			if role == domain.PageRecordRole && n.Kind == "static" && n.Type == string(domain.StaticLink) && prop == "href" {
				continue
			}
			out = append(out, role)
		}
		out = append(out, fromRolesUnder(n.Children)...)
	}
	return out
}

// A gate that cannot fail is the failure this file exists to prevent, so the helper is held against faults.
func TestRecordsBindingProblemsSeesEachFault(t *testing.T) {
	m := &domain.Machine{
		ID:     "mch_x",
		Fields: []domain.Field{{ID: "fld_t", Type: domain.FieldTypeText}, {ID: "fld_r", Type: domain.FieldTypeRelation, RelatedMachine: "mch_y"}},
		CardFields: []domain.CardField{
			{Field: "fld_t", Role: domain.CardFieldRoleTitle},
			{Field: "fld_r", Role: domain.CardFieldRolePerson},
		},
		Datasets: []domain.Dataset{
			{ID: "ds_list", Source: "mch_x", Select: domain.SelectRecords, Limit: 5},
			{ID: "ds_agg", Source: "mch_x", Dimension: "fld_t"},
		},
	}
	owner := map[string]*domain.Machine{"ds_list": m, "ds_agg": m}
	list := func(ds, role string) domain.PageNode {
		return domain.PageNode{Kind: "layout", Type: "stack", Children: []domain.PageNode{{
			Kind: "component", Type: "Collection", Binding: &domain.PageBinding{Dataset: ds, Rows: domain.PageRowsRecords},
			Children: []domain.PageNode{{Kind: "static", Type: "paragraph", From: map[string]string{"text": role}}},
		}}}
	}
	if got := recordsBindingProblems(list("ds_list", "title"), owner); len(got) != 0 {
		t.Errorf("a sound list reported %v", got)
	}
	eachList := func(each, role string) domain.PageNode {
		l := list("ds_list", "title")
		l.Children[0].Children = []domain.PageNode{{Kind: "component", Type: "Tag", Each: each, From: map[string]string{"label": role}}}
		return l
	}
	tagged := *m
	tagged.CardTags = &domain.CardTags{Machine: "mch_j", Via: "fld_a", Tag: "fld_b"}
	if got := recordsBindingProblems(eachList("tags", "title"), map[string]*domain.Machine{"ds_list": &tagged}); len(got) != 0 {
		t.Errorf("a sound each: tags reported %v", got)
	}
	// A reference to the runtime's own people is the one reference a page resolves (PersonNames, no Machine read).
	people := &domain.Machine{
		ID:         "mch_p",
		Fields:     []domain.Field{{ID: "fld_u", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID}},
		CardFields: []domain.CardField{{Field: "fld_u", Role: domain.CardFieldRolePerson}},
		Datasets:   []domain.Dataset{{ID: "ds_people", Source: "mch_p", Select: domain.SelectRecords, Limit: 5}},
	}
	if got := recordsBindingProblems(list("ds_people", "person"), map[string]*domain.Machine{"ds_people": people}); len(got) != 0 {
		t.Errorf("a person role reported %v", got)
	}
	for name, page := range map[string]domain.PageNode{
		"aggregate dataset":                   list("ds_agg", "title"),
		"unknown dataset":                     list("ds_none", "title"),
		"undeclared role":                     list("ds_list", "money"),
		"reference role":                      list("ds_list", "person"),
		"each on a Machine with no card_tags": eachList("tags", "title"),
		"each over another list":              eachList("comments", "title"),
		"each reading a record role":          eachList("tags", "status"),
	} {
		if got := recordsBindingProblems(page, owner); len(got) == 0 {
			t.Errorf("%s: no problem reported", name)
		}
	}
}

// TestEveryInstalledLinkNamesItsDestinationOnce holds Tahap 2a's rule against the real manifests:
// a `static: link` on an installed page names its destination with `to:` (or `list_of:` / a record's `href`), that id is one the same Application
// declares, and the node carries no `href` of its own. It asserts the *declaration* directly instead of
// leaning on `ir.Lower`'s refusals, so it still fails if Lower stopped checking, and it fails when no installed
// page holds a link, so it cannot pass by measuring nothing.
func TestEveryInstalledLinkNamesItsDestinationOnce(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	links, recordLinks, listLinks, metricLinks := 0, 0, 0, 0
	for _, slug := range sortedKeys(wss) {
		datasets := map[string]domain.Dataset{}
		for _, m := range wss[slug].Workspace.Machines {
			for _, ds := range m.Datasets {
				datasets[ds.ID] = ds
			}
		}
		for _, app := range wss[slug].Workspace.Applications {
			declared := map[string]bool{}
			pageOf := map[string]*domain.PageNode{}
			// A link into another Application (`in:`) names a screen of that Application when it is installed in this
			// Workspace, and nothing is owed when it is not (the link is not drawn).
			foreign := map[string]bool{}
			for _, other := range wss[slug].Workspace.Applications {
				if other.ID == app.ID {
					continue
				}
				for _, it := range other.AllNavigation {
					foreign[domain.ForeignLinkID(other.ID, it.ID)] = it.RecordOf == ""
				}
			}
			installed := func(appID string) bool {
				for _, other := range wss[slug].Workspace.Applications {
					if other.ID == appID {
						return true
					}
				}
				return false
			}
			for _, it := range app.LinkTargets(wss[slug].Workspace.Navigation) {
				declared[it.ID] = true
				pageOf[it.ID] = it.Page
			}
			var walk func(n domain.PageNode, where string)
			walk = func(n domain.PageNode, where string) {
				if n.Kind == "static" && n.Type == string(domain.StaticLink) {
					links++
					switch role, fromHref := n.From["href"]; {
					case n.ListOf != "" && (n.To != "" || fromHref):
						t.Errorf("%s: link names list_of: %s and a second destination", where, n.ListOf)
					case n.ListOf != "":
						listLinks++
						if n.Props["text"] == "" {
							t.Errorf("%s: a list_of link has no menu label to default its text to, so it must write text:", where)
						}
					case fromHref && n.To != "":
						t.Errorf("%s: link names both to: %s and from: href", where, n.To)
					case fromHref && role != domain.PageRecordRole && !strings.HasPrefix(role, "filehref:"):
						t.Errorf("%s: link takes href from %q; only the reserved %q or a file Field's address (filehref:) is one", where, role, domain.PageRecordRole)
					case fromHref:
						recordLinks++
					case n.In != "":
						if installed(n.In) && !foreign[domain.ForeignLinkID(n.In, n.To)] {
							t.Errorf("%s: link into %s names %q, which that Application has no linkable screen of", where, n.In, n.To)
						}
					case !declared[n.To]:
						t.Errorf("%s: link to %q names no navigation item of this Application", where, n.To)
					}
					if _, typed := n.Props["href"]; typed {
						t.Errorf("%s: link carries a typed href; its destination is to: <navigation id>", where)
					}
				} else if n.Kind == "component" && n.Type == string(domain.ComponentMetric) && n.Binding != nil && n.Binding.Rows == domain.PageRowsDimension && (n.To != "" || n.Param != "") {
					// A bound Metric that links: the row's value goes to the destination as ?param=, so the
					// destination has to exist, name the parameter, and bind a Dataset that reads it.
					metricLinks++
					switch {
					case n.To == "" || n.Param == "":
						t.Errorf("%s: a linking Metric names both to: and param:", where)
					case !declared[n.To]:
						t.Errorf("%s: Metric to %q names no navigation item of this Application", where, n.To)
					case pageOf[n.To] == nil || !pageBindsParameter(*pageOf[n.To], n.Param, datasets):
						t.Errorf("%s: Metric sends ?%s= to %s, whose page binds no Dataset filtering on $parameters.%s -- the link would filter nothing", where, n.Param, n.To, n.Param)
					}
					if _, typed := n.Props["href"]; typed {
						t.Errorf("%s: Metric carries a typed href; its destination is to: with param:", where)
					}
				} else if n.To != "" || n.ListOf != "" || n.Param != "" {
					t.Errorf("%s: to:/list_of: on a node that is not a link", where)
				}
				for _, c := range n.Children {
					walk(c, where)
				}
			}
			for _, item := range app.AllNavigation {
				if item.Page != nil {
					walk(*item.Page, slug+"/"+app.ID+"/"+item.ID)
				}
			}
		}
	}
	if links == 0 {
		t.Fatal("no installed page declares a `static: link` -- this gate is measuring nothing")
	}
	if recordLinks == 0 {
		t.Fatal("no installed page declares a link to a record -- half of this gate is measuring nothing")
	}
	if listLinks == 0 {
		t.Fatal("no installed page declares a list_of link -- that third of this gate is measuring nothing")
	}
	if metricLinks == 0 {
		t.Fatal("no installed page declares a linking Metric -- that fourth of this gate is measuring nothing")
	}
}

// pageBindsParameter reports whether any Dataset a page binds filters on `$parameters.<param>`. It reads the
// Dataset's own `where:` rather than asking the loader's check, so it still fails if that check is removed.
func pageBindsParameter(n domain.PageNode, param string, datasets map[string]domain.Dataset) bool {
	if n.Binding != nil {
		for _, c := range datasets[n.Binding.Dataset].Where.Comparisons() {
			if c.Value == "$parameters."+param {
				return true
			}
		}
	}
	for _, c := range n.Children {
		if pageBindsParameter(c, param, datasets) {
			return true
		}
	}
	return false
}

// windowDatasets are Datasets whose `limit:` is their *meaning*, not a safety cap: "recent" is defined by the
// bound, so a notice that more exist would state something false (the argument in
// `composition.Selection.Limit`). Nothing declares which kind a `limit:` is, so a page opts in with
// `complete: true` and this list is the other half -- the judgement that a particular Dataset is not one a
// page may claim to be complete over. A closed list with a reason per entry, `perViewerDatasets`' shape,
// because "is this limit a window" is a judgement no scan can make.
var windowDatasets = map[string]string{
	"ds_recent_documents": "limit: 5 is what \"recent\" means; the page lists the five latest, not every document",
	"ds_recent_activity":  "limit: 10 is what \"recent\" means; the panel shows the ten newest events, not the whole log",
}

// TestNoPageClaimsCompletenessOverAWindow sweeps the installed pages for `complete: true` and refuses one whose
// Collection is bound to a window. It also fails if nothing claims completeness at all (a gate measuring
// nothing) and if an entry names a Dataset no installed Workspace declares (a stale reason).
func TestNoPageClaimsCompletenessOverAWindow(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	claims, declared := 0, map[string]bool{}
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		for _, m := range ws.Machines {
			for _, ds := range m.Datasets {
				declared[ds.ID] = true
			}
		}
		for _, app := range ws.Applications {
			for _, item := range app.AllNavigation {
				if item.Page == nil {
					continue
				}
				var walk func(n domain.PageNode)
				walk = func(n domain.PageNode) {
					if n.Props["complete"] == "true" {
						claims++
						if n.Binding != nil {
							if why, window := windowDatasets[n.Binding.Dataset]; window {
								t.Errorf("%s/%s/%s: complete: true over %s, which is a window -- %s", slug, app.ID, item.ID, n.Binding.Dataset, why)
							}
						}
					}
					for _, c := range n.Children {
						walk(c)
					}
				}
				walk(*item.Page)
			}
		}
	}
	if claims == 0 {
		t.Fatal("no installed page declares `complete: true` -- this gate is measuring nothing")
	}
	for id := range windowDatasets {
		if !declared[id] {
			t.Errorf("windowDatasets names %s, which no installed Workspace declares -- delete the entry", id)
		}
	}
}

// TestEveryInstalledFormNamesNoRouteOrField holds T1's rule against the real manifests: an installed `Form`
// declares its words (`submit`), a Dataset and the write mode, and nothing else. No `action`, `href`, `name`,
// `method` or `id` -- the route is `domain.FormRoute(<the Dataset's Machine>)` and the controls are the
// Machine's own askable Fields, so a page that typed either would be a second write path beside the generic
// one. A Form bound `write: update` (T3) additionally sits inside a records Collection and names **no** Dataset:
// the record is the Collection's and the Machine is that Collection's Dataset's, so a Dataset there could only
// name a different one. It reads the declaration directly instead of leaning on `ir.lowerForm`'s refusals, re-derives that the
// Machine can be asked (`CreateFormInputs`), and fails when no installed page holds a Form, so it cannot pass by
// measuring nothing. It does **not** require the Dataset to select records: the loader treats a write binding's
// Dataset as a handle to its Machine, and inventing a stricter rule here would be a gate ahead of the primitive.
func TestEveryInstalledFormNamesNoRouteOrField(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	forms, updates := 0, 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		machineOf := map[string]*domain.Machine{}
		for i := range ws.Machines {
			for _, ds := range ws.Machines[i].Datasets {
				machineOf[ds.ID] = ws.Machines[i]
			}
		}
		for _, app := range ws.Applications {
			var walk func(n domain.PageNode, where string, item *domain.Machine)
			walk = func(n domain.PageNode, where string, item *domain.Machine) {
				if n.Kind == "component" && n.Type == string(domain.ComponentForm) {
					forms++
					for _, key := range []string{"action", "href", "name", "method", "id", "kind", "options", "value", "required"} {
						if _, typed := n.Props[key]; typed {
							t.Errorf("%s: a Form writes %q itself; the route and the controls come from the Dataset's Machine", where, key)
						}
					}
					if n.Props["submit"] == "" {
						t.Errorf("%s: a Form has no submit words", where)
					}
					if len(n.Children) != 0 {
						t.Errorf("%s: a Form lists its own children; its Fields are the Machine's", where)
					}
					switch {
					case n.Binding != nil && n.Binding.Write == domain.PageWriteUpdate:
						updates++
						switch {
						case item == nil:
							t.Errorf("%s: write: update outside the item template of a records Collection", where)
						case n.Binding.Dataset != "" || n.Binding.Rows != "" || n.Binding.Measure != "":
							t.Errorf("%s: write: update edits the Collection's own record and names no dataset, rows or measure", where)
						case len(item.EditFormInputs(nil)) == 0:
							t.Errorf("%s: machine %s has no Field an edit form can ask for", where, item.ID)
						}
					case n.Binding != nil && n.Binding.Write == domain.PageWriteTransition:
						// A Form that takes a declared edge with what the edge asks (2026-10-17, plan B2): inside a record
						// template, naming an edit edge of that record's Machine, no dataset.
						found := false
						if item != nil {
							for _, e := range item.Transitions {
								found = found || (e.ID == n.Binding.Transition && e.Action == domain.ActionEdit)
							}
						}
						if !found || n.Binding.Dataset != "" {
							t.Errorf("%s: a transition Form sits in a record template and names an edit edge of its record's Machine (%q)", where, n.Binding.Transition)
						}
					case n.Binding == nil || n.Binding.Write != domain.PageWriteCreate:
						t.Errorf("%s: a Form is bound with write: %s or %s", where, domain.PageWriteCreate, domain.PageWriteUpdate)
					case n.Binding.Rows != "" || n.Binding.Measure != "":
						t.Errorf("%s: a Form's binding writes; it names no rows or measure", where)
					case machineOf[n.Binding.Dataset] == nil:
						t.Errorf("%s: a Form's Dataset %q is not in this Workspace", where, n.Binding.Dataset)
					default:
						inputs, unsupported := machineOf[n.Binding.Dataset].CreateFormInputs()
						if len(inputs) == 0 {
							t.Errorf("%s: machine %s has no Field a form can ask for", where, machineOf[n.Binding.Dataset].ID)
						}
						for _, f := range unsupported {
							if f.Required {
								t.Errorf("%s: machine %s requires %s, a %s the form cannot ask", where, machineOf[n.Binding.Dataset].ID, f.ID, f.Type)
							}
						}
					}
				} else if n.Kind == "component" && n.Type == string(domain.ComponentFormInput) {
					t.Errorf("%s: Input is produced by lowering a Form and is never written", where)
				}
				childItem := item
				if n.Binding != nil && n.Binding.Rows == domain.PageRowsRecords {
					childItem = machineOf[n.Binding.Dataset]
				}
				for _, c := range n.Children {
					walk(c, where, childItem)
				}
			}
			for _, nav := range app.AllNavigation {
				if nav.Page != nil {
					walk(*nav.Page, slug+"/"+app.ID+"/"+nav.ID, nil)
				}
			}
		}
	}
	if forms == 0 {
		t.Fatal("no installed page declares a Form -- this gate is measuring nothing")
	}
	if updates == 0 {
		t.Fatal("no installed page declares a `write: update` Form -- the update half of this gate is measuring nothing")
	}
}

// TestEveryInstalledButtonIsADeclaredRecordWrite holds T3b's and T5's rule against the real manifests: an
// installed `Button` on a page is a record write -- `binding: {write: delete}`, `{write: move}` or (K13, 2026-10-10)
// `{write: transition}`, which names `becomes:` -- an option of the Machine's status Field or `$done`/`$reopen` read from
// its `completion:` -- and asks nothing -- inside a
// records Collection, of a Machine that is not append-only, and nothing else that names a request: no `action`,
// `method`, `href`, `name` or `value`. A delete carries the sentence it asks first (`confirm`); a move carries
// `direction: up|down`, asks nothing, and sits only over a Dataset that lists the Machine's own order (no `where:`,
// no `sort:`), because a record's neighbour in any other list is not its neighbour in the order a move changes.
// The route is the record's own generic one and the verb is derived, so a page that typed either would be a
// second write path beside the generic one. It reads the declaration directly rather than leaning on
// `ir.lowerDeleteButton`'s or `ir.lowerMoveButton`'s refusals, and fails when no installed page holds a delete or
// a move, so neither half can pass by measuring nothing.
func TestEveryInstalledButtonIsADeclaredRecordWrite(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	type source struct {
		machine *domain.Machine
		dataset domain.Dataset
	}
	deletes, moves, transitions, prints, toggles := 0, 0, 0, 0, 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		sourceOf := map[string]source{}
		for i := range ws.Machines {
			for _, ds := range ws.Machines[i].Datasets {
				sourceOf[ds.ID] = source{ws.Machines[i], ds}
			}
		}
		for _, app := range ws.Applications {
			var walk func(n domain.PageNode, where string, item *source)
			walk = func(n domain.PageNode, where string, item *source) {
				if n.Kind == "component" && n.Type == string(domain.ComponentToggle) {
					// The completion circle (2026-10-19): a transition write that names no status -- the Machine's `completion:` is
					// the whole of it, written the other way from where the record stands. Re-derived here from the Machine.
					toggles++
					switch {
					case n.Binding == nil || n.Binding.Write != domain.PageWriteTransition:
						t.Errorf("%s: a Toggle is bound with write: %s", where, domain.PageWriteTransition)
					case item == nil:
						t.Errorf("%s: a Toggle sits inside a record template (a Table's cell:, or a records Collection's item)", where)
					case n.Binding.Dataset != "" || n.Binding.Rows != "" || n.Binding.Measure != "" || n.Binding.Transition != "":
						t.Errorf("%s: a Toggle acts on its own record's completion and names no dataset, rows, measure or edge", where)
					case n.From["describes"] == "":
						t.Errorf("%s: a Toggle repeats down a list, so it names its record with from: {describes: <role>}", where)
					case item.machine.AppendOnly || item.machine.Completion == nil:
						t.Errorf("%s: machine %s is append-only or declares no completion:, so a Toggle has nothing to write", where, item.machine.ID)
					default:
						m := item.machine
						field, ok := m.FieldByID(m.CardFieldFor(domain.CardFieldRoleStatus))
						if !ok || !slices.Contains(field.Options, m.Completion.Done) || !slices.Contains(field.Options, m.ReopenValue()) {
							t.Errorf("%s: machine %s's status Field cannot hold both the completion value %q and the reopen value %q", where, m.ID, m.Completion.Done, m.ReopenValue())
						}
					}
					for _, key := range []string{"checked", "action", "name", "value", "becomes", "confirm"} {
						if _, typed := n.Props[key]; typed {
							t.Errorf("%s: a Toggle derives %q from the Machine and the record; a page never writes it", where, key)
						}
					}
				}
				if n.Kind == "component" && n.Type == string(domain.ComponentButton) {
					for _, key := range []string{"action", "method", "href", "name", "value"} {
						if _, typed := n.Props[key]; typed {
							t.Errorf("%s: a Button writes %q itself; the route and the verb are derived from the record", where, key)
						}
					}
					write := ""
					if n.Binding != nil {
						write = n.Binding.Write
					}
					switch {
					case write == domain.PageWritePrint:
						// Prints the page: no record, no Dataset, no question (a page's own Print button, 2026-10-18).
						prints++
						if n.Binding.Dataset != "" || n.Binding.Rows != "" || n.Binding.Measure != "" || n.Props["confirm"] != "" {
							t.Errorf("%s: write: %s prints the page and names no dataset, rows, measure or confirm", where, write)
						}
					case write == domain.PageWritePDF:
						// Downloads the page as a PDF: the same shape as Print.
						prints++
						if n.Binding.Dataset != "" || n.Binding.Rows != "" || n.Binding.Measure != "" || n.Props["confirm"] != "" {
							t.Errorf("%s: write: %s outputs the page and names no dataset, rows, measure or confirm", where, write)
						}
					case write != domain.PageWriteDelete && write != domain.PageWriteMove && write != domain.PageWriteTransition:
						t.Errorf("%s: a Button on a page is bound with write: %s, %s, %s or %s", where, domain.PageWriteDelete, domain.PageWriteMove, domain.PageWriteTransition, domain.PageWritePrint)
					case item == nil:
						t.Errorf("%s: write: %s outside the item template of a records Collection", where, write)
					case n.Binding.Dataset != "" || n.Binding.Rows != "" || n.Binding.Measure != "":
						t.Errorf("%s: write: %s acts on the Collection's own record and names no dataset, rows or measure", where, write)
					case item.machine.AppendOnly:
						t.Errorf("%s: machine %s is append-only, so no record of it can be changed", where, item.machine.ID)
					case write == domain.PageWriteDelete:
						deletes++
						if strings.TrimSpace(n.Props["confirm"]) == "" {
							t.Errorf("%s: a delete states its consequence in confirm", where)
						}
					case write == domain.PageWriteTransition:
						transitions++
						if n.From["describes"] == "" {
							t.Errorf("%s: a transition Button repeats down a list, so it names its record with from: {describes: <role>} -- otherwise a screen reader hears the same bare label once per row", where)
						}
						// A transition asks only when it cannot be taken back: nothing leads out of the status it reaches. One that can be
						// undone by its opposite asks nothing -- a question there is noise a person learns to click through.
						if _, asks := n.Props["confirm"]; asks {
							edge, found := domain.Transition{}, false
							for _, e := range item.machine.Transitions {
								if e.ID == n.Binding.Transition {
									edge, found = e, true
								}
							}
							if !found {
								t.Errorf("%s: a transition that asks names its edge (transition: trn_x), so the gate can see whether it can be undone", where)
							}
							for _, e := range item.machine.Transitions {
								if found && e.Field == edge.Field && e.From == edge.To {
									t.Errorf("%s: a transition to %q asks, though %s leads out of it -- it is undone by its opposite and asks nothing", where, edge.To, e.ID)
								}
							}
						}
						// A Button naming its edge (`transition: trn_x`) moves that edge's own Field: the edge must be one the
						// Machine declares for the edit action, re-derived here and not from metadata's own check.
						if edge := n.Binding.Transition; edge != "" {
							found := false
							for _, e := range item.machine.Transitions {
								found = found || (e.ID == edge && e.Action == domain.ActionEdit)
							}
							if !found {
								t.Errorf("%s: transition: %s is no edit edge of machine %s", where, edge, item.machine.ID)
							}
							break
						}
						// Re-derived from the Machine, not from metadata's own check: a `becomes:` the Machine cannot
						// reach is a Button that is never drawn, which reads exactly like a page with nothing to do.
						m := item.machine
						field, ok := m.FieldByID(m.CardFieldFor(domain.CardFieldRoleStatus))
						target := n.Props["becomes"]
						switch target {
						case domain.PageTransitionDone:
							if m.Completion == nil {
								t.Errorf("%s: becomes: %s on machine %s, which declares no completion:", where, target, m.ID)
							} else {
								target = m.Completion.Done
							}
						case domain.PageTransitionReopen:
							if m.Completion == nil {
								t.Errorf("%s: becomes: %s on machine %s, which declares no completion:", where, target, m.ID)
							} else {
								target = m.ReopenValue()
							}
						}
						if !ok || !slices.Contains(field.Options, target) {
							t.Errorf("%s: becomes: %q is no option of machine %s's status Field", where, n.Props["becomes"], m.ID)
						}
					default:
						moves++
						if d := n.Props["direction"]; d != domain.MoveUp && d != domain.MoveDown {
							t.Errorf("%s: a move names direction: %s or %s, not %q", where, domain.MoveUp, domain.MoveDown, d)
						}
						if _, asks := n.Props["confirm"]; asks {
							t.Errorf("%s: a move is undone by its opposite and asks nothing", where)
						}
						if item.dataset.Where != nil || len(item.dataset.Sort) > 0 {
							t.Errorf("%s: a move sits over dataset %s, which declares a where: or sort:, so its list is not the Machine's order", where, item.dataset.ID)
						}
					}
				} else if n.Binding != nil && (n.Binding.Write == domain.PageWriteDelete || n.Binding.Write == domain.PageWriteMove || n.Binding.Write == domain.PageWriteTransition) &&
					!(n.Type == string(domain.ComponentForm) && n.Binding.Write == domain.PageWriteTransition && n.Binding.Transition != "") &&
					!(n.Type == string(domain.ComponentToggle) && n.Binding.Write == domain.PageWriteTransition) {
					// A Form may take a declared edge with what it asks (a proof with "Mark as paid"); nothing else but a
					// Button deletes, moves or transitions.
					t.Errorf("%s: write: %s is bound to a %s; only a Button deletes, moves or transitions", where, n.Binding.Write, n.Type)
				}
				childItem := item
				if n.Binding != nil && n.Binding.Rows == domain.PageRowsRecords {
					if src, ok := sourceOf[n.Binding.Dataset]; ok {
						childItem = &src
					} else {
						childItem = nil
					}
				}
				for _, c := range n.Children {
					walk(c, where, childItem)
				}
			}
			for _, nav := range app.AllNavigation {
				if nav.Page != nil {
					walk(*nav.Page, slug+"/"+app.ID+"/"+nav.ID, nil)
				}
			}
		}
	}
	if deletes == 0 {
		t.Fatal("no installed page declares a delete Button -- the delete half of this gate is measuring nothing")
	}
	if moves == 0 {
		t.Fatal("no installed page declares a move Button -- the move half of this gate is measuring nothing")
	}
	if transitions == 0 {
		t.Fatal("no installed page declares a transition Button -- the transition half of this gate is measuring nothing")
	}
	if prints == 0 {
		t.Fatal("no installed page declares a print Button -- the print half of this gate is measuring nothing")
	}
	if toggles == 0 {
		t.Fatal("no installed page declares a Toggle -- the completion half of this gate is measuring nothing")
	}
}

// TestEveryInstalledTotalNamesAMeasureItsDatasetDeclares is the `rows: total` Binding's gate (2026-10-10, Stage 1
// of the Dashboard's move to `page:`), installed after its one consumer exists. It re-checks every installed
// total against the Machine that owns its Dataset, **without calling `metadata`'s own check**, for the reason
// `recordsBindingProblems` states: a loader that stopped vetting would otherwise leave a tile that answers 500.
//
// What a total can be wrong about, and nothing a placeholder resolver can see: the Dataset is an aggregate (a
// `select: records` Dataset has no Measures to total), it declares the named Measure, and the figure's label is
// written while its value is not. It also holds that no installed Measure still carries a literal `$done` --
// Normalize replaces it with the Machine's `completion:` value, so one that survived would be compared as the
// text "$done" and count nothing, which reads exactly like a page with no finished work.
func TestEveryInstalledTotalNamesAMeasureItsDatasetDeclares(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	totals := 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		owner := map[string]domain.Dataset{}
		for _, m := range ws.Machines {
			for _, ds := range m.Datasets {
				owner[ds.ID] = ds
				for _, ms := range ds.Measures {
					for _, c := range ms.Where.Comparisons() {
						if c.Value == "$done" {
							t.Errorf("%s: dataset %s measure %s still compares against the literal $done", slug, ds.ID, ms.ID)
						}
					}
				}
			}
		}
		for _, app := range ws.Applications {
			for _, item := range app.AllNavigation {
				if item.Page == nil {
					continue
				}
				for _, p := range totalBindingProblems(*item.Page, owner, &totals) {
					t.Errorf("%s/%s/%s: %s", slug, app.ID, item.ID, p)
				}
			}
		}
	}
	if totals == 0 {
		t.Fatal("no installed page declares a `rows: total` Metric -- this gate is measuring nothing")
	}
}

func totalBindingProblems(n domain.PageNode, datasets map[string]domain.Dataset, count *int) []string {
	var out []string
	if b := n.Binding; b != nil && b.Rows == domain.PageRowsTotal {
		*count++
		ds, ok := datasets[b.Dataset]
		switch {
		case !ok:
			out = append(out, "total names dataset "+b.Dataset+", which no Machine of this Workspace declares")
		case ds.Select != "":
			out = append(out, "total over "+ds.ID+", which selects records and so has no Measure to total")
		default:
			// A FilterChips over named views (2026-10-18) totals one Measure per chip, named in counts:.
			measures := []string{b.Measure}
			if n.Type == string(domain.ComponentFilterChips) {
				measures = nil
				for _, key := range sortedKeys(n.Counts) {
					measures = append(measures, n.Counts[key])
				}
				if len(measures) == 0 {
					out = append(out, "a FilterChips over named views names each chip's Measure in counts:")
				}
			}
			for _, m := range measures {
				found := false
				for _, ms := range ds.Measures {
					found = found || ms.ID == m
				}
				if !found {
					out = append(out, "total names measure "+m+", which "+ds.ID+" does not declare")
				}
			}
		}
		if strings.TrimSpace(n.Props["label"]) == "" {
			out = append(out, "a total writes its own label: there is no Dimension value to name it")
		}
		if _, typed := n.Props["value"]; typed {
			out = append(out, "a total's value comes from its Measure and is never typed")
		}
	}
	for _, c := range n.Children {
		out = append(out, totalBindingProblems(c, datasets, count)...)
	}
	return out
}

// The gate must bite on each fault it names; a gate never shown to fail is not yet a gate.
func TestTotalBindingProblemsSeesEachFault(t *testing.T) {
	datasets := map[string]domain.Dataset{
		"ds_agg":  {ID: "ds_agg", Measures: []domain.Measure{{ID: "msr_n"}}},
		"ds_recs": {ID: "ds_recs", Select: domain.SelectRecords},
	}
	node := func(ds, measure string, props map[string]string) domain.PageNode {
		return domain.PageNode{Kind: "component", Type: "Metric", Props: props,
			Binding: &domain.PageBinding{Dataset: ds, Measure: measure, Rows: domain.PageRowsTotal}}
	}
	cases := map[string]domain.PageNode{
		"missing dataset": node("ds_gone", "msr_n", map[string]string{"label": "x"}),
		"records dataset": node("ds_recs", "msr_n", map[string]string{"label": "x"}),
		"missing measure": node("ds_agg", "msr_gone", map[string]string{"label": "x"}),
		"no label":        node("ds_agg", "msr_n", nil),
		"typed value":     node("ds_agg", "msr_n", map[string]string{"label": "x", "value": "9"}),
	}
	for name, n := range cases {
		var c int
		if len(totalBindingProblems(n, datasets, &c)) == 0 {
			t.Errorf("%s: no problem reported", name)
		}
	}
	var c int
	if got := totalBindingProblems(node("ds_agg", "msr_n", map[string]string{"label": "x"}), datasets, &c); len(got) != 0 || c != 1 {
		t.Errorf("a sound total reported %v (counted %d)", got, c)
	}
}

func datasetDeclaresMeasure(ds domain.Dataset, id string) bool {
	for _, ms := range ds.Measures {
		if ms.ID == id {
			return true
		}
	}
	return false
}

// TestEveryPercentageNamesTwoCountedMeasures: a percentage whose numerator or denominator names nothing is a silent
// 0% at request time, not an error, so every installed Workspace's and every library template's percentages are
// re-checked here, independently of metadata.percentageIssues.
func TestEveryPercentageNamesTwoCountedMeasures(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	templates, err := installer.Templates(filepath.Join(repoRoot(), "metadata"))
	if err != nil {
		t.Fatalf("read the template library: %v", err)
	}
	machines := map[string]*domain.Machine{}
	for _, slug := range sortedKeys(wss) {
		for _, m := range wss[slug].Workspace.Machines {
			machines[slug+"/"+m.ID] = m
		}
	}
	for _, tm := range templates {
		for _, m := range tm.Machines {
			machines["library/"+m.Machine.ID] = m.Machine
		}
	}
	checked := 0
	for _, key := range sortedKeys(machines) {
		for _, ds := range machines[key].Datasets {
			for _, ms := range ds.Measures {
				if ms.Aggregate != domain.AggregatePercentage {
					continue
				}
				checked++
				for _, part := range []string{ms.Numerator, ms.Denominator} {
					ok := false
					for _, other := range ds.Measures {
						if other.ID == part && other.Aggregate != domain.AggregatePercentage {
							ok = true
						}
					}
					if !ok {
						t.Errorf("%s: %s.%s is a percentage of %q, which is not a counted or summed measure of that dataset", key, ds.ID, ms.ID, part)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no percentage measure found -- the gate measured nothing")
	}
}

// TestEveryLibraryApplicationInstallsIntoAnEmptyWorkspace takes each Application in the template library
// through the real installer (PlanInstall, then Install, which load-verifies through metadata.LoadApplication)
// into a scratch Workspace. It exists because nothing else ever loaded a library Application's `page:`: the
// installed copies are swept, the library files are not, and a break-read of K15 D4 (2026-10-11) set a library
// `nav_dashboard` route back to a retired path and bound a page to a Dataset that did not exist -- neither
// failed a single test, and either would have surfaced only when someone installed the template. Installing is
// the one check that runs the loader's own page validation over the library's files exactly as an admin's
// install would. Empty scratch Workspace, so a collision refusal here means the template collides with itself.
func TestEveryLibraryApplicationInstallsIntoAnEmptyWorkspace(t *testing.T) {
	library, err := filepath.Abs(filepath.Join(repoRoot(), "metadata"))
	if err != nil {
		t.Fatal(err)
	}
	templates, err := installer.Templates(library)
	if err != nil {
		t.Fatalf("read the template library: %v", err)
	}
	if len(templates) == 0 {
		t.Fatal("the template library holds no Application -- this gate is measuring nothing")
	}
	pages := 0
	// A companion cannot load alone: it is installed (and so measured) with the template that names it.
	for _, tmpl := range installer.Listed(templates) {
		id := tmpl.Application.ID
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			manifest := filepath.Join(dir, "scratch.yaml")
			rel, err := filepath.Rel(dir, filepath.Join(library, "user.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifest, []byte("workspace: scratch\nmachines:\n  - "+filepath.ToSlash(rel)+"\napplications: []\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			ws, err := metadata.LoadApplication(manifest)
			if err != nil {
				t.Fatalf("the empty scratch Workspace must load: %v", err)
			}
			plan := installer.PlanInstall(tmpl, ws.Workspace)
			if !plan.OK() {
				t.Fatalf("PlanInstall refused the library's own template in an empty Workspace: %v", plan.Refusals)
			}
			if _, err := installer.Install(plan, library, manifest); err != nil {
				t.Fatalf("installing %s into an empty Workspace failed: %v", id, err)
			}
		})
		for _, one := range append([]installer.Template{tmpl}, tmpl.Companions...) {
			for _, item := range one.Application.AllNavigation {
				if item.Page != nil {
					pages++
				}
			}
		}
	}
	if pages == 0 {
		t.Fatal("no library Application declares a `page:` -- the load this gate runs is not reaching one")
	}
}

// TestWeekDatasetIsTheLibrarysAndEveryCopysAlike: `ds_tasks_due_week` is read by `nav_calendar` in every Workspace that installed Project Management, and
// a fresh install copies the *library's* Dataset, which no web test serves. A probe changed the library's `$week+6`
// to `$week+5`, to `$today`, dropped its `sort:` and pointed its page at another Dataset, and nothing failed. So the
// library's Dataset is resolved here against a fixed date and read for what it means (Monday..Sunday, by due date),
// and every Workspace copy must equal it.
func TestWeekDatasetIsTheLibrarysAndEveryCopysAlike(t *testing.T) {
	templates, err := installer.Templates(filepath.Join(repoRoot(), "metadata"))
	if err != nil {
		t.Fatalf("read the template library: %v", err)
	}
	find := func(machines []*domain.Machine) *domain.Dataset {
		for _, m := range machines {
			for i := range m.Datasets {
				if m.Datasets[i].ID == "ds_tasks_due_week" {
					return &m.Datasets[i]
				}
			}
		}
		return nil
	}
	var libMachines []*domain.Machine
	var pageDataset, pageEmpty string
	for _, tm := range templates {
		for _, m := range tm.Machines {
			libMachines = append(libMachines, m.Machine)
		}
		for _, item := range tm.Application.AllNavigation {
			if item.ID == "nav_calendar" && item.Page != nil {
				for _, b := range pageBindings(*item.Page) {
					// The first records binding is the week's; the month's (`ds_tasks_due_month`) follows it.
					if b.Rows == domain.PageRowsRecords && pageDataset == "" {
						pageDataset = b.Dataset
					}
				}
				var find func(n domain.PageNode)
				find = func(n domain.PageNode) {
					if n.Binding != nil && n.Binding.Rows == domain.PageRowsRecords && pageEmpty == "" {
						pageEmpty = n.Props["empty"]
					}
					for _, c := range n.Children {
						find(c)
					}
				}
				find(*item.Page)
			}
		}
	}
	lib := find(libMachines)
	if lib == nil {
		t.Fatal("the library declares no ds_tasks_due_week")
	}
	if pageDataset != lib.ID || pageEmpty == "" {
		t.Errorf("the library's nav_calendar binds %q with empty %q, want %q and some words", pageDataset, pageEmpty, lib.ID)
	}

	// 2026-10-08 is a Thursday; ?week=1 puts the window on 2026-10-12..2026-10-18.
	ctx := expression.Context{Today: "2026-10-08", Parameters: map[string]string{"week": "1"}}
	var lo, hi string
	for _, c := range lib.Where.Comparisons() {
		v, ok := ctx.Resolve(c.Value)
		switch {
		case c.Optional && c.Field == "fld_project":
			// The view group's scope (vg_tasks): left out when the request names no project, which is this window's request.
		case !ok:
			t.Errorf("the library's where value %q does not resolve", c.Value)
		case c.Field != "fld_due_date":
			t.Errorf("the library's where names %q, want only fld_due_date", c.Field)
		case c.Op == expression.OpGreaterOrEqual:
			lo = v
		case c.Op == expression.OpLessOrEqual:
			hi = v
		default:
			t.Errorf("the library's where uses op %q, want gte and lte", c.Op)
		}
	}
	if lo != "2026-10-12" || hi != "2026-10-18" {
		t.Errorf("the library's window resolves to %s..%s, want 2026-10-12..2026-10-18 (Monday..Sunday)", lo, hi)
	}
	if len(lib.Sort) == 0 || lib.Sort[0].Field != "fld_due_date" || lib.Sort[0].Direction != "asc" || lib.Limit <= 0 {
		t.Errorf("the library's sort/limit = %+v / %d, want fld_due_date asc first and a limit", lib.Sort, lib.Limit)
	}

	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	copies := 0
	for _, slug := range sortedKeys(wss) {
		cp := find(wss[slug].Workspace.Machines)
		if cp == nil {
			continue
		}
		copies++
		if !reflect.DeepEqual(cp, lib) {
			t.Errorf("%s: ds_tasks_due_week differs from the library's:\n got %+v\nwant %+v", slug, *cp, *lib)
		}
	}
	if copies == 0 {
		t.Error("no Workspace holds a copy, so the equality above measured nothing")
	}
}

// TestPageParameterWeekIsTheExpressionOne: `domain.PageParameterWeek` is the name a page's `step:` and `when:` give
// and `expression.WeekParameter` the query key `$week` reads. They are one word in two packages because domain
// cannot import expression; if they drift, a page's toolbar moves a parameter the Dataset never reads.
func TestPageParameterWeekIsTheExpressionOne(t *testing.T) {
	if domain.PageParameterWeek != expression.WeekParameter {
		t.Errorf("domain.PageParameterWeek = %q, expression.WeekParameter = %q; they must be one name", domain.PageParameterWeek, expression.WeekParameter)
	}
}

// TestPageParametersAreTheExpressionOnes: the month parameter a page steps and tests is the key `$month` reads, and the range's two words are the
// ones a calendar's SegmentedControl offers; one word in two packages, held equal for the reason the week's is.
func TestPageParametersAreTheExpressionOnes(t *testing.T) {
	if domain.PageParameterMonth != expression.MonthParameter {
		t.Errorf("domain.PageParameterMonth = %q, expression.MonthParameter = %q; they must be one name", domain.PageParameterMonth, expression.MonthParameter)
	}
	for _, p := range []string{domain.PageParameterWeek, domain.PageParameterMonth, domain.PageParameterRange} {
		if !domain.ParamReserved(p) {
			t.Errorf("%q is a parameter the runtime names, so a SearchBox, a link's pass: or a scope may not take it", p)
		}
	}
}

// TestEveryInstalledWorkspacePageLowersAndValidates is the sweep above for the pages a Workspace declares itself
// (`navigation:` on its manifest): each lowers against the Workspace's whole link list, validates as a UI tree, binds only
// Datasets the Workspace declares, and sits at /pages/<id>. It fails if no Workspace declares one, so it cannot pass by
// measuring nothing.
func TestEveryInstalledWorkspacePageLowersAndValidates(t *testing.T) {
	wss, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		t.Fatalf("load workspaces: %v", err)
	}
	ir.RegisterComponentTypes(registry.ComponentTypeNames())
	ir.RegisterSlottedComponentTypes(registry.SlottedComponentTypeNames())
	checked := 0
	for _, slug := range sortedKeys(wss) {
		ws := wss[slug].Workspace
		datasets := map[string]domain.Dataset{}
		for _, m := range ws.Machines {
			for _, ds := range m.Datasets {
				datasets[ds.ID] = ds
			}
		}
		for _, item := range ws.Navigation {
			if item.Page == nil {
				continue
			}
			checked++
			where := slug + "/" + item.ID
			if want := "/pages/" + item.ID; item.Route != want {
				t.Errorf("%s: route is %q but a page is rendered at %q", where, item.Route, want)
			}
			tree, err := ir.Lower(*item.Page, metadata.PlaceholderResolver(ws.LinkTargets(), datasets))
			if err != nil {
				t.Errorf("%s: does not lower: %v", where, err)
				continue
			}
			if issues := ir.Validate(tree); len(issues) != 0 {
				t.Errorf("%s: lowers to an invalid tree: %v", where, issues)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no installed Workspace declares a page of its own: this gate measured nothing")
	}
}
