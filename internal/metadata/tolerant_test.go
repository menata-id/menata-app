package metadata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifestDir builds a temp manifest directory holding a copy of the shared user Machine and the given
// manifests, so a test can break exactly one of them.
func writeManifestDir(t *testing.T, manifests map[string]string) string {
	t.Helper()
	root := t.TempDir()
	user, err := os.ReadFile(filepath.Join("..", "..", "metadata", "user.yaml"))
	if err != nil {
		t.Fatalf("read user.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "user.yaml"), user, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "workspaces")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range manifests {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const goodManifest = "workspace: good\nmachines:\n  - ../user.yaml\napplications: []\n"

// TestLoadWorkspacesTolerant_oneBrokenManifestDoesNotStopTheOthers is K09's central probe: the broken
// Workspace is reported with its own file, the good one still loads.
func TestLoadWorkspacesTolerant_oneBrokenManifestDoesNotStopTheOthers(t *testing.T) {
	dir := writeManifestDir(t, map[string]string{
		"good.yaml":    goodManifest,
		"broken.yaml":  "workspace: broken\nmachines:\n  - ../no-such-machine.yaml\napplications: []\n",
		"garbage.yaml": "workspace: [unclosed",
	})
	loaded, failures, err := LoadWorkspacesTolerant(dir)
	if err != nil {
		t.Fatalf("LoadWorkspacesTolerant: %v", err)
	}
	if _, ok := loaded["good"]; !ok || len(loaded) != 1 {
		t.Errorf("loaded = %v, want exactly the good Workspace", loaded)
	}
	if len(failures) != 2 {
		t.Fatalf("failures = %d, want 2 (broken, garbage)", len(failures))
	}
	for _, f := range failures {
		if !strings.Contains(f.Err.Error(), f.Path) {
			t.Errorf("failure for %q does not name its file %q: %v", f.Slug, f.Path, f.Err)
		}
	}
	if failures[0].Slug != "broken" || failures[1].Slug != "garbage" {
		t.Errorf("failures out of file-name order: %q, %q", failures[0].Slug, failures[1].Slug)
	}
}

// TestLoadWorkspaces_staysStrict keeps the all-or-nothing entry point the gates and installer rely on.
func TestLoadWorkspaces_staysStrict(t *testing.T) {
	dir := writeManifestDir(t, map[string]string{
		"good.yaml":   goodManifest,
		"broken.yaml": "workspace: broken\nmachines:\n  - ../no-such-machine.yaml\n",
	})
	if _, err := LoadWorkspaces(dir); err == nil {
		t.Fatal("LoadWorkspaces accepted a directory holding a broken manifest")
	}
}

func TestLoadWorkspacesTolerant_secondManifestForOneWorkspaceFailsAlone(t *testing.T) {
	dir := writeManifestDir(t, map[string]string{
		"a.yaml": goodManifest,
		"b.yaml": goodManifest,
	})
	loaded, failures, err := LoadWorkspacesTolerant(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || len(failures) != 1 || failures[0].Slug != "b" {
		t.Errorf("loaded=%d failures=%v, want the first manifest kept and only b.yaml failed", len(loaded), failures)
	}
}

func TestRunStages_reportsEveryStageAndSurvivesAPanic(t *testing.T) {
	err := runStages([]stage{
		{"one", func() error { return &ValidationError{Issues: []string{"first fault", "second fault"}} }},
		{"two", func() error { return errors.New("third fault") }},
		{"three", func() error { var m map[string]int; m["x"] = 1; return nil }},
		{"four", func() error { return nil }},
	})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	got := strings.Join(ve.Issues, "\n")
	for _, want := range []string{"first fault", "second fault", "third fault", `stage "three" panicked`} {
		if !strings.Contains(got, want) {
			t.Errorf("issues lack %q:\n%s", want, got)
		}
	}
	if runStages([]stage{{"ok", func() error { return nil }}}) != nil {
		t.Error("runStages reported a fault for clean stages")
	}
}
