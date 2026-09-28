package installer

import "testing"

// These two moved here with AppendBlockListItem itself on 2026-09-28 (from internal/aiassist, whose
// generated-Application publish was its only caller until a template install needed the same edit).

func TestAppendBlockListItem_missingKey(t *testing.T) {
	if _, err := AppendBlockListItem([]byte("workspace: default\n"), "applications:", "x.yaml"); err == nil {
		t.Fatal("AppendBlockListItem() = nil error, want one for a missing key")
	}
}

// TestAppendBlockListItem_emptyFlowList covers the real, documented shape of a brand-new
// Workspace's own manifest (CLAUDE.md: "An empty applications: [] is valid and normal") --
// converting it to block style in place rather than refusing it.
func TestAppendBlockListItem_emptyFlowList(t *testing.T) {
	src := "workspace: default\nmachines:\n  - ../user.yaml\napplications: []\n"
	got, err := AppendBlockListItem([]byte(src), "applications:", "../applications/leave_requests.yaml")
	if err != nil {
		t.Fatalf("AppendBlockListItem() error = %v", err)
	}
	want := "workspace: default\nmachines:\n  - ../user.yaml\napplications:\n  - ../applications/leave_requests.yaml\n"
	if string(got) != want {
		t.Errorf("AppendBlockListItem() =\n%s\nwant\n%s", got, want)
	}
}
