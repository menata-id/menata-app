package aiassist

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// copyTree copies src into dst, keeping the relative layout the manifests' "../" paths rely on.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// missingKeys lists every mapping key (as a path) present in before and absent from after. A list
// element is matched by its "id" when it has one, so reordering is not a loss. Scalars are not
// compared: the update is allowed to change a value, never to drop a key.
func missingKeys(path string, before, after any) []string {
	var out []string
	switch b := before.(type) {
	case map[string]any:
		a, ok := after.(map[string]any)
		if !ok {
			return []string{path + " (no longer a mapping)"}
		}
		for k, bv := range b {
			av, present := a[k]
			if !present {
				out = append(out, path+"."+k)
				continue
			}
			out = append(out, missingKeys(path+"."+k, bv, av)...)
		}
	case []any:
		a, _ := after.([]any)
		byID := map[string]any{}
		for _, e := range a {
			if m, ok := e.(map[string]any); ok {
				if id, ok := m["id"].(string); ok {
					byID[id] = e
				}
			}
		}
		for i, e := range b {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id == "" {
				continue
			}
			ae, present := byID[id]
			if !present {
				out = append(out, path+"["+id+"]")
				continue
			}
			_ = i
			out = append(out, missingKeys(path+"["+id+"]", e, ae)...)
		}
	}
	return out
}

func readGeneric(t *testing.T, p string) any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return v
}

// TestUpdate_keepsEveryKeyOfAHandWrittenApplication is roadmap stage 1, W1: the assistant is shown
// only the projection DescribeApplication expresses, so the question is whether a real Application
// written by hand (Desain Request in menata-indonesia: datasets, card_fields, views, completion,
// send_notification events, `page:` menu items) loses anything when the assistant renames things,
// adds a field and a status option, and rewords a menu item. A key present before and absent after
// is a defect that was alive in production.
func TestUpdate_keepsEveryKeyOfAHandWrittenApplication(t *testing.T) {
	src := filepath.Join("..", "..", "metadata")
	root := t.TempDir()
	copyTree(t, src, root)
	manifest := filepath.Join(root, "workspaces", "menata-indonesia.yaml")
	wsDir := filepath.Join(root, "workspaces", "menata-indonesia")
	files := []string{
		filepath.Join(wsDir, "design_request.yaml"),
		filepath.Join(wsDir, "applications", "design_request.yaml"),
	}
	before := make([]any, len(files))
	for i, f := range files {
		before[i] = readGeneric(t, f)
	}

	state, _ := stateOf(t, manifest)
	desired := DescribeApplication("app_design_request", state.Applications["app_design_request"])
	desired.Name = "Desain Request Baru"
	for i := range desired.Machines {
		if desired.Machines[i].ID != "mch_design_request" {
			continue
		}
		desired.Machines[i].Name = "Request Desain Baru"
		desired.Machines[i].Fields = append(desired.Machines[i].Fields, GeneratedField{ID: "fld_catatan_baru", Name: "Catatan Baru", Type: "text"})
		for j := range desired.Machines[i].Fields {
			if desired.Machines[i].Fields[j].Type == "status" {
				desired.Machines[i].Fields[j].Options = append(desired.Machines[i].Fields[j].Options, "ditinjau_ulang")
			}
		}
	}
	for i := range desired.Navigation {
		desired.Navigation[i].Label += " (baru)"
	}
	change := GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_design_request", Application: &desired}
	publish(t, manifest, change)

	// The probe is vacuous unless the update really happened.
	appAfter := readGeneric(t, files[1]).(map[string]any)
	if appAfter["name"] != "Desain Request Baru" {
		t.Fatalf("the update did not rename the Application: %v", appAfter["name"])
	}
	machAfter, _ := os.ReadFile(files[0])
	for _, want := range []string{"fld_catatan_baru", "ditinjau_ulang", "Request Desain Baru"} {
		if !strings.Contains(string(machAfter), want) {
			t.Fatalf("the update did not write %q into the Machine file", want)
		}
	}
	for i, f := range files {
		after := readGeneric(t, f)
		if lost := missingKeys(filepath.Base(f), before[i], after); len(lost) > 0 {
			t.Errorf("the update dropped keys from %s:\n  %v", f, lost)
		}
	}
}

// TestUpdate_refusesToDropAPageMenuItem: a menu item that opens a declared `page:` is shown to the
// model with no Machine (it opens "a screen that could not be added back"), so a model that leaves
// it out of the desired menu is refused rather than deleting a screen the assistant cannot rewrite.
func TestUpdate_refusesToDropAPageMenuItem(t *testing.T) {
	root := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "metadata"), root)
	manifest := filepath.Join(root, "workspaces", "menata-indonesia.yaml")
	state, _ := stateOf(t, manifest)
	desired := DescribeApplication("app_design_request", state.Applications["app_design_request"])
	var kept []GeneratedMenuItem
	for _, n := range desired.Navigation {
		if n.MachineID != "" {
			kept = append(kept, n)
		}
	}
	if len(kept) == len(desired.Navigation) {
		t.Fatal("the fixture has no page menu item, so the test proves nothing")
	}
	desired.Navigation = kept
	change := GeneratedChange{Kind: KindUpdateApplication, TargetAppID: "app_design_request", Application: &desired}
	if plan := PlanUpdate(DescribeApplication("app_design_request", state.Applications["app_design_request"]), desired); len(plan.Refusals) == 0 {
		t.Fatalf("dropping page menu items was not refused: %+v", plan)
	}
	if _, err := Write(manifest, change, FileMachineResolver{WorkspaceManifestPath: manifest}); err == nil {
		t.Fatal("Write accepted an update that drops page menu items")
	}
}
