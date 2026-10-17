package metadata

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// The runtime's embedded pages load, every one names a runtime screen, and a Workspace is offered the ones whose
// Datasets it declares: the shared notification Machine brings ds_my_notifications, a manifest without it has no
// /notifications page (the route answers 404) instead of a broken one.
func TestRuntimePage_isOfferedOnlyToAWorkspaceThatHasItsDatasets(t *testing.T) {
	pages, err := loadRuntimePages()
	if err != nil || len(pages) == 0 {
		t.Fatalf("loadRuntimePages = %d pages, %v", len(pages), err)
	}
	for _, p := range pages {
		if p.Page == nil || p.Route == "" || p.Heading == "" {
			t.Errorf("runtime page %q is missing its page, route or heading: %+v", p.ID, p)
		}
	}
	with, err := LoadApplication(filepath.Join("..", "..", "metadata", "workspaces", "default.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasPage(with.Workspace.RuntimePages, "nav_notifications") {
		t.Error("the default Workspace, which installs the notification Machine, was not offered the notifications page")
	}
	dir := t.TempDir()
	writeFile(t, dir, "doc.yaml", "\nid: mch_doc\nname: Doc\nfields:\n  - { id: fld_title, name: Title, type: text }\n")
	writeFile(t, dir, "bare.yaml", "\nworkspace: bare\nmachines:\n  - doc.yaml\napplications: []\n")
	bare, err := LoadApplication(filepath.Join(dir, "bare.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if hasPage(bare.Workspace.RuntimePages, "nav_notifications") {
		t.Error("a Workspace with no notification Machine was offered the notifications page")
	}
}

func hasPage(pages []domain.NavigationItem, id string) bool {
	for _, p := range pages {
		if p.ID == id {
			return true
		}
	}
	return false
}

func TestRuntimeMachines_loadAndAreHeldToTheirPromises(t *testing.T) {
	ms, err := loadRuntimeMachines()
	if err != nil || len(ms) == 0 {
		t.Fatalf("loadRuntimeMachines = %d, %v", len(ms), err)
	}
	const base = "id: mch_x\nname: X\nruntime: %s\nfields:\n  - { id: fld_name, name: Name, type: text }\n%s"
	for name, tc := range map[string]struct{ runtime, extra, want string }{
		"a source nobody provides":        {"nothing", "", "not a source"},
		"a missing filled field":          {"groups", "", ""},
		"a rule block on a read-only one": {"groups", "permissions:\n  - { id: prm_x, action: edit }\n", "read-only"},
	} {
		doc := fmt.Sprintf(base, tc.runtime, tc.extra)
		if name == "a missing filled field" {
			doc = "id: mch_x\nname: X\nruntime: groups\nfields:\n  - { id: fld_other, name: O, type: text }\n"
			tc.want = "fills fld_name"
		}
		m, err := Parse([]byte(doc))
		if err != nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if err := Validate(m); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

func TestRuntimeMachines_aWorkspaceCannotDeclareOneOrCollideWithOne(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rt.yaml", "\nid: mch_rt\nname: RT\nruntime: groups\nfields:\n  - { id: fld_name, name: N, type: text }\n")
	writeFile(t, dir, "a.yaml", "\nworkspace: a\nmachines:\n  - rt.yaml\napplications: []\n")
	if _, err := LoadApplication(filepath.Join(dir, "a.yaml")); err == nil || !strings.Contains(err.Error(), "only the runtime's own") {
		t.Errorf("a Workspace machine declaring runtime: %v", err)
	}
	writeFile(t, dir, "g.yaml", "\nid: mch_group\nname: G\nfields:\n  - { id: fld_t, name: T, type: text }\n")
	writeFile(t, dir, "b.yaml", "\nworkspace: b\nmachines:\n  - g.yaml\napplications: []\n")
	if _, err := LoadApplication(filepath.Join(dir, "b.yaml")); err == nil || !strings.Contains(err.Error(), "runtime's own machines use") {
		t.Errorf("a Workspace machine reusing a runtime machine's id: %v", err)
	}
}

func TestRuntimeMachines_onlyTheRuntimesOwnPagesMayBindOne(t *testing.T) {
	ws := strings.Replace(validWorkspacePage, "dataset: ds_rows", "dataset: ds_runtime_groups", 1)
	if err := workspacePageFixture(t, ws); err == nil || !strings.Contains(err.Error(), "provided by the runtime") {
		t.Errorf("a Workspace page binding a runtime dataset: %v", err)
	}
}

func TestActionBinding_isHeldToWhatTheActionIs(t *testing.T) {
	group := &domain.Machine{ID: "mch_group", Runtime: "groups"}
	other := &domain.Machine{ID: "mch_x"}
	node := func(typ, action string) domain.PageNode {
		return domain.PageNode{Kind: "component", Type: typ, Binding: &domain.PageBinding{Write: domain.PageWriteAction, Action: action}}
	}
	for name, tc := range map[string]struct {
		n    domain.PageNode
		item *domain.Machine
		want string // "" means no issue
	}{
		"a create form at the root":           {node("Form", "create_group"), nil, ""},
		"a delete button in a group list":     {node("Button", "delete_group"), group, ""},
		"an action nobody performs":           {node("Button", "drop_everything"), nil, "not an operation"},
		"a form for an action that asks none": {node("Form", "delete_group"), group, "asks nothing"},
		"a button for an action that asks":    {node("Button", "create_group"), nil, "asks for"},
		"a create action inside a template":   {node("Form", "create_group"), group, "not about a record"},
		"a record action outside a template":  {node("Button", "delete_group"), nil, "inside the item template"},
		"a record action over another source": {node("Button", "delete_group"), other, "inside the item template"},
	} {
		issues := actionBindingIssues(tc.n, tc.item, "where")
		switch {
		case tc.want == "" && len(issues) > 0:
			t.Errorf("%s: unexpected issues %v", name, issues)
		case tc.want != "" && (len(issues) != 1 || !strings.Contains(issues[0], tc.want)):
			t.Errorf("%s: %v, want one containing %q", name, issues, tc.want)
		}
	}
	extra := node("Form", "create_group")
	extra.Binding.Dataset = "ds_runtime_groups"
	if issues := actionBindingIssues(extra, nil, "where"); len(issues) != 1 || !strings.Contains(issues[0], "nothing else") {
		t.Errorf("an action naming a dataset: %v", issues)
	}
}

func TestActionBinding_onlyTheRuntimesOwnPagesMayPerformOne(t *testing.T) {
	ws := strings.Replace(validWorkspacePage, "        - static: link\n          to: nav_docs\n", "        - component: Form\n          submit: Go\n          binding: { write: action, action: create_group }\n", 1)
	if err := workspacePageFixture(t, ws); err == nil || !strings.Contains(err.Error(), "runtime's own pages only") {
		t.Errorf("a Workspace page performing a runtime action: %v", err)
	}
}
