package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"menata.app/internal/domain"
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
				res := metadata.PlaceholderResolver(app.AllNavigation, datasets)
				rows, records := res.Rows, res.Records
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
					if !ok {
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
				for _, role := range fromRolesUnder(n.Children) {
					fieldID := m.CardFieldFor(domain.CardFieldRole(role))
					f, ok := m.FieldByID(fieldID)
					switch {
					case fieldID == "" || !ok:
						out = append(out, "from: "+role+" -- "+m.ID+" declares no such card_fields role")
					case f.IsReference():
						out = append(out, "from: "+role+" -- the Field behind it is a reference, which would read a whole Machine (007 §20)")
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

func fromRolesUnder(nodes []domain.PageNode) []string {
	var out []string
	for _, n := range nodes {
		for prop, role := range n.From {
			// The reserved word is the record's route, not a Projection role; it is judged where it is used
			// (a link's href) by TestEveryInstalledLinkNamesItsDestinationOnce, and by ir.Lower for any other prop.
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
	for name, page := range map[string]domain.PageNode{
		"aggregate dataset": list("ds_agg", "title"),
		"unknown dataset":   list("ds_none", "title"),
		"undeclared role":   list("ds_list", "money"),
		"reference role":    list("ds_list", "person"),
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
			for _, it := range app.AllNavigation {
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
					case fromHref && role != domain.PageRecordRole:
						t.Errorf("%s: link takes href from %q; only the reserved %q is an address", where, role, domain.PageRecordRole)
					case fromHref:
						recordLinks++
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
