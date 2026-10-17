package metadata

import (
	"path/filepath"
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
