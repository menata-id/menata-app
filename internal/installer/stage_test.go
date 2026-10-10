package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const stagedDoc = "id: mch_doc\nname: Doc\nfields:\n  - id: fld_title\n    name: Title\n    type: text\n"

func stageFixture(t *testing.T) (manifest string) {
	t.Helper()
	manifest, _, _ = restoreFixture(t)
	own := filepath.Join(filepath.Dir(manifest), "acme")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(own, "doc.yaml"), []byte(stagedDoc), 0o644)
	os.WriteFile(manifest, []byte("workspace: acme\nmachines:\n  - ../user.yaml\n  - acme/doc.yaml\napplications: []\n"), 0o644)
	// Another Workspace that must never be staged, and must not be able to break this one.
	os.WriteFile(filepath.Join(filepath.Dir(manifest), "other.yaml"), []byte("this: is: not yaml"), 0o644)
	return manifest
}

func TestValidateFiles_answersWithoutTouchingTheLiveFiles(t *testing.T) {
	manifest := stageFixture(t)
	live := filepath.Join(filepath.Dir(manifest), "acme", "doc.yaml")

	unref, err := ValidateFiles(manifest, map[string][]byte{"doc.yaml": []byte(stagedDoc + "  - id: fld_note\n    name: Note\n    type: text\n")})
	if err != nil || len(unref) != 0 {
		t.Fatalf("a good replacement: unreferenced=%v err=%v", unref, err)
	}
	if got, _ := os.ReadFile(live); string(got) != stagedDoc {
		t.Error("validation changed the live file")
	}

	_, err = ValidateFiles(manifest, map[string][]byte{"doc.yaml": []byte(strings.Replace(stagedDoc, "type: text", "type: bogus", 1))})
	var rejected *RejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("a bad type: err = %v, want a RejectedError", err)
	}
	if strings.Contains(err.Error(), "menata-stage") {
		t.Errorf("the staging directory leaked into the error: %v", err)
	}
	if got, _ := os.ReadFile(live); string(got) != stagedDoc {
		t.Error("a refused validation changed the live file")
	}
}

func TestValidateFiles_saysWhenAFileIsNeverLoaded(t *testing.T) {
	manifest := stageFixture(t)
	unref, err := ValidateFiles(manifest, map[string][]byte{"orphan.yaml": []byte("id: mch_orphan\nname: Orphan\nfields:\n  - id: fld_a\n    name: A\n    type: text\n")})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(unref) != 1 || unref[0] != "orphan.yaml" {
		t.Fatalf("unreferenced = %v, want the orphan reported: valid must not mean ignored", unref)
	}
}

func TestValidateFiles_refusesPathsOutsideTheWorkspaceDirectory(t *testing.T) {
	manifest := stageFixture(t)
	for _, name := range []string{"../user.yaml", "/etc/x.yaml", "doc.txt", "..", ""} {
		if _, err := ValidateFiles(manifest, map[string][]byte{name: []byte("id: x\n")}); err == nil {
			t.Errorf("%q was accepted", name)
		}
	}
	if _, err := ValidateFiles(manifest, nil); err == nil {
		t.Error("an empty upload was accepted")
	}
}

func TestStageWorkspace_copiesOnlyThisWorkspacesOwnFiles(t *testing.T) {
	manifest := stageFixture(t)
	staged, cleanup, err := StageWorkspace(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	dir := filepath.Dir(staged)
	for rel, want := range map[string]bool{"acme.yaml": true, "acme/doc.yaml": true, "other.yaml": false, "../user.yaml": true} {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if (err == nil) != want {
			t.Errorf("staged %s present=%v, want %v", rel, err == nil, want)
		}
	}
}
