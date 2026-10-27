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
