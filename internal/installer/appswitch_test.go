package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/metadata"
)

const switchApp = `# The Notes application. A comment the switch must not lose.
id: app_notes
name: Notes
machines: [mch_note]
navigation:
  - id: nav_notes   # trailing comment
    label: Notes
    route: /machines/mch_note
`

func switchFixture(t *testing.T) (manifest, appFile string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "workspaces")
	for path, body := range map[string]string{
		filepath.Join(root, "user.yaml"):                         "id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n",
		filepath.Join(dir, "acme", "note.yaml"):                  "id: mch_note\nname: Note\nfields:\n  - id: fld_t\n    name: T\n    type: text\n",
		filepath.Join(dir, "acme", "applications", "notes.yaml"): switchApp,
		filepath.Join(dir, "acme.yaml"):                          "workspace: acme\nmachines:\n  - ../user.yaml\n  - acme/note.yaml\napplications:\n  - acme/applications/notes.yaml\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "acme.yaml"), filepath.Join(dir, "acme", "applications", "notes.yaml")
}

func inactiveAfterLoad(t *testing.T, manifest string) bool {
	t.Helper()
	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatalf("does not load: %v", err)
	}
	app, _ := loaded.Workspace.ApplicationByID("app_notes")
	return app.Inactive
}

// Switching off writes one line after the id and nothing else; switching on removes exactly that line; asking for the state it
// is already in writes nothing.
func TestSetApplicationInactive_editsOneLineAndBackAgain(t *testing.T) {
	manifest, file := switchFixture(t)
	changed, err := SetApplicationInactive(manifest, "app_notes", true)
	if err != nil || !changed {
		t.Fatalf("switch off: changed=%v err=%v", changed, err)
	}
	got, _ := os.ReadFile(file)
	if want := strings.Replace(switchApp, "id: app_notes\n", "id: app_notes\ninactive: true\n", 1); string(got) != want {
		t.Errorf("file after switching off:\n%s\nwant:\n%s", got, want)
	}
	if !inactiveAfterLoad(t, manifest) {
		t.Error("the loader does not read the key as inactive")
	}
	if changed, err := SetApplicationInactive(manifest, "app_notes", true); err != nil || changed {
		t.Errorf("switching off twice: changed=%v err=%v, want a no-op", changed, err)
	}
	if changed, err := SetApplicationInactive(manifest, "app_notes", false); err != nil || !changed {
		t.Fatalf("switch on: changed=%v err=%v", changed, err)
	}
	if back, _ := os.ReadFile(file); string(back) != switchApp {
		t.Errorf("switching on did not restore the file byte for byte:\n%s", back)
	}
	if changed, _ := SetApplicationInactive(manifest, "app_notes", false); changed {
		t.Error("switching on an active Application wrote")
	}
}

func TestSetApplicationInactive_refusesWhatIsNotInstalled(t *testing.T) {
	manifest, _ := switchFixture(t)
	_, err := SetApplicationInactive(manifest, "app_nope", true)
	var rejected *RejectedError
	if !errors.As(err, &rejected) {
		t.Errorf("an application not installed here: %v, want a RejectedError", err)
	}
}

// A Workspace that does not load is not made worse: the switch is put back and refused.
func TestSetApplicationInactive_putsTheFileBackWhenTheWorkspaceDoesNotLoad(t *testing.T) {
	manifest, file := switchFixture(t)
	broken := filepath.Join(filepath.Dir(manifest), "acme", "applications", "broken.yaml")
	os.WriteFile(broken, []byte("id: app_broken\nname: Broken\nmachines: [mch_missing]\nnavigation: []\n"), 0o644)
	os.WriteFile(manifest, []byte("workspace: acme\nmachines:\n  - ../user.yaml\n  - acme/note.yaml\napplications:\n  - acme/applications/notes.yaml\n  - acme/applications/broken.yaml\n"), 0o644)
	_, err := SetApplicationInactive(manifest, "app_notes", true)
	var rejected *RejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("got %v, want a RejectedError", err)
	}
	if now, _ := os.ReadFile(file); string(now) != switchApp {
		t.Errorf("the file was left changed after a refused switch:\n%s", now)
	}
}

// twoApps adds a second Application with its own Machine beside Notes; related makes that Machine point at Notes' Machine.
func twoApps(t *testing.T, related bool) (manifest, notesApp, notesMachine, otherApp, otherMachine string) {
	t.Helper()
	manifest, notesApp = switchFixture(t)
	dir := filepath.Dir(manifest)
	notesMachine = filepath.Join(dir, "acme", "note.yaml")
	otherMachine = filepath.Join(dir, "acme", "task.yaml")
	otherApp = filepath.Join(dir, "acme", "applications", "tasks.yaml")
	fields := "  - id: fld_t\n    name: T\n    type: text\n"
	if related {
		fields += "  - id: fld_note\n    name: Note\n    type: reference\n    machine: mch_note\n"
	}
	os.WriteFile(otherMachine, []byte("id: mch_task\nname: Task\nfields:\n"+fields), 0o644)
	os.WriteFile(otherApp, []byte("id: app_tasks\nname: Tasks\nmachines: [mch_task]\nnavigation:\n  - id: nav_tasks\n    label: Tasks\n    route: /machines/mch_task\n"), 0o644)
	os.WriteFile(manifest, []byte("workspace: acme\nmachines:\n  - ../user.yaml\n  - acme/note.yaml\n  - acme/task.yaml\napplications:\n  - acme/applications/notes.yaml\n  - acme/applications/tasks.yaml\n"), 0o644)
	return
}

// Removing an Application takes out its file, its own Machine files and their manifest lines, and nothing of its neighbour's.
func TestRemoveApplication_takesOutOnlyItsOwn(t *testing.T) {
	manifest, notesApp, notesMachine, otherApp, otherMachine := twoApps(t, false)
	otherBefore, _ := os.ReadFile(otherApp)
	removed, err := RemoveApplication(manifest, "app_notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Files) != 2 || len(removed.MachineIDs) != 1 || removed.MachineIDs[0] != "mch_note" {
		t.Errorf("removed %+v, want the application file and mch_note's file", removed)
	}
	for _, gone := range []string{notesApp, notesMachine} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s is still there", gone)
		}
	}
	for _, kept := range []string{otherApp, otherMachine, filepath.Join(filepath.Dir(manifest), "..", "user.yaml")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed with its neighbour", kept)
		}
	}
	if after, _ := os.ReadFile(otherApp); string(after) != string(otherBefore) {
		t.Error("the neighbour's file changed")
	}
	m, _ := os.ReadFile(manifest)
	if strings.Contains(string(m), "notes.yaml") || strings.Contains(string(m), "acme/note.yaml") || !strings.Contains(string(m), "tasks.yaml") || !strings.Contains(string(m), "../user.yaml") {
		t.Errorf("manifest after:\n%s", m)
	}
	loaded, err := metadata.LoadApplication(manifest)
	if err != nil {
		t.Fatalf("does not load: %v", err)
	}
	if _, ok := loaded.Workspace.ApplicationByID("app_notes"); ok {
		t.Error("the Application is still loaded")
	}
}

// A Workspace that cannot do without the Application is put back exactly as it was.
func TestRemoveApplication_putsEverythingBackWhenTheRestDoesNotLoad(t *testing.T) {
	manifest, _, _, _, _ := twoApps(t, true)
	before := snapshotTree(t, filepath.Dir(manifest))
	_, err := RemoveApplication(manifest, "app_notes")
	var rejected *RejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("got %v, want a RejectedError", err)
	}
	if after := snapshotTree(t, filepath.Dir(manifest)); len(after) != len(before) {
		t.Fatalf("files before %d, after %d", len(before), len(after))
	} else {
		for k, v := range before {
			if after[k] != v {
				t.Errorf("%s differs after the refused removal", k)
			}
		}
	}
}

func TestRemoveApplication_refusesWhatItDoesNotOwn(t *testing.T) {
	manifest, _, _, _, _ := twoApps(t, false)
	var rejected *RejectedError
	if _, err := RemoveApplication(manifest, "app_nope"); !errors.As(err, &rejected) {
		t.Errorf("an Application not installed here: %v", err)
	}
	// A Machine in a shared file (beside the Workspace, not inside it) is never removed with an Application that claims it.
	dir := filepath.Dir(manifest)
	shared := filepath.Join(filepath.Dir(dir), "shared.yaml")
	os.WriteFile(shared, []byte("id: mch_shared\nname: Shared\nfields:\n  - id: fld_t\n    name: T\n    type: text\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "acme", "applications", "tasks.yaml"), []byte("id: app_tasks\nname: Tasks\nmachines: [mch_task, mch_shared]\nnavigation:\n  - id: nav_tasks\n    label: Tasks\n    route: /machines/mch_task\n"), 0o644)
	m, _ := os.ReadFile(manifest)
	os.WriteFile(manifest, []byte(strings.Replace(string(m), "  - acme/task.yaml\n", "  - acme/task.yaml\n  - ../shared.yaml\n", 1)), 0o644)
	before := snapshotTree(t, filepath.Dir(dir))
	if _, err := RemoveApplication(manifest, "app_tasks"); !errors.As(err, &rejected) {
		t.Fatalf("an Application claiming a shared Machine: %v", err)
	}
	for k, v := range snapshotTree(t, filepath.Dir(dir)) {
		if before[k] != v {
			t.Errorf("%s changed in a refused removal", k)
		}
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

// Taking out the last Application leaves `applications: []`, so the next one can still be published into the Workspace.
func TestRemoveApplication_theLastOneLeavesAnEmptyListTheNextCanBeAppendedTo(t *testing.T) {
	manifest, _ := switchFixture(t)
	if _, err := RemoveApplication(manifest, "app_notes"); err != nil {
		t.Fatal(err)
	}
	m, _ := os.ReadFile(manifest)
	if !strings.Contains(string(m), "applications: []") {
		t.Fatalf("manifest after removing the last Application:\n%s", m)
	}
	if _, err := AppendBlockListItem(m, "applications:", "acme/applications/next.yaml"); err != nil {
		t.Errorf("appending the next Application: %v", err)
	}
	// A list that still has items is left alone.
	if strings.Contains(string(m), "machines: []") {
		t.Errorf("machines still lists ../user.yaml, but was written as empty:\n%s", m)
	}
}

// A manifest line is dropped only when the item is that file: another item that merely begins the same way is left alone, and a trailing comment does not hide it.
func TestDropManifestLineMatchesTheWholeItem(t *testing.T) {
	m := []byte("machines:\n  - ../user.yaml\n  - acme/note.yaml   # the notes\n  - acme/note.yaml.bak\n  - 'acme/notes.yaml'\n")
	got, ok := dropManifestLine(m, "acme/note.yaml")
	if !ok {
		t.Fatal("the item was not found")
	}
	if want := "machines:\n  - ../user.yaml\n  - acme/note.yaml.bak\n  - 'acme/notes.yaml'\n"; string(got) != want {
		t.Errorf("after dropping acme/note.yaml:\n%s\nwant:\n%s", got, want)
	}
	if _, ok := dropManifestLine([]byte("machines:\n  - acme/note.yaml.bak\n"), "acme/note.yaml"); ok {
		t.Error("an item that only begins with the name was dropped")
	}
	if got, ok := dropManifestLine([]byte("machines:\n  - acme/note.yaml"), "acme/note.yaml"); !ok || string(got) != "machines:\n" {
		t.Errorf("the last item with no final newline: %q, %v", got, ok)
	}
}
