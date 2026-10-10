package installer

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// restoreFixture is a loadable Workspace "acme" beside a shared user Machine, with a backup directory.
func restoreFixture(t *testing.T) (manifest, backups, root string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, "workspaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "user.yaml"),
		[]byte("id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest = filepath.Join(dir, "acme.yaml")
	if err := os.WriteFile(manifest, []byte("workspace: acme\nmachines:\n  - ../user.yaml\napplications: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest, filepath.Join(root, "backups"), root
}

func writeArchive(t *testing.T, backups, slug, id string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(backups, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, id+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
}

func TestRestoreSnapshot_putsTheInstallationBackAndIsItselfUndoable(t *testing.T) {
	manifest, backups, _ := restoreFixture(t)
	own := filepath.Join(filepath.Dir(manifest), "acme")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "kept.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(manifest)
	snap, err := SnapshotWorkspace(manifest, backups, time.Unix(1_700_000_000, 0))
	if err != nil || snap == "" {
		t.Fatalf("SnapshotWorkspace = %q, %v", snap, err)
	}

	// The installation moves on: a changed manifest and a file the snapshot never had.
	os.WriteFile(manifest, append(original, []byte("# edited\n")...), 0o644)
	os.WriteFile(filepath.Join(own, "kept.txt"), []byte("after"), 0o644)
	os.WriteFile(filepath.Join(own, "added.txt"), []byte("new"), 0o644)

	list, _ := ListSnapshots(manifest, backups)
	if len(list) != 1 {
		t.Fatalf("ListSnapshots = %v, want the one snapshot", list)
	}
	if err := RestoreSnapshot(manifest, backups, list[0].ID, time.Unix(1_700_000_100, 0)); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	if got, _ := os.ReadFile(manifest); string(got) != string(original) {
		t.Errorf("manifest = %q, want the snapshot's", got)
	}
	if got, _ := os.ReadFile(filepath.Join(own, "kept.txt")); string(got) != "before" {
		t.Errorf("kept.txt = %q, want the snapshot's", got)
	}
	if _, err := os.Stat(filepath.Join(own, "added.txt")); !os.IsNotExist(err) {
		t.Error("a file added after the snapshot survived the restore")
	}
	after, _ := ListSnapshots(manifest, backups)
	if len(after) != 2 || after[0].ID <= after[1].ID {
		t.Fatalf("ListSnapshots after = %v, want two, newest first (the restore snapshots what it replaced)", after)
	}
	if err := RestoreSnapshot(manifest, backups, after[0].ID, time.Unix(1_700_000_200, 0)); err != nil {
		t.Fatalf("undoing the restore: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(own, "added.txt")); string(got) != "new" {
		t.Errorf("undoing the restore did not bring added.txt back: %q", got)
	}
	if _, err := os.Stat(manifest + ".restore-bak"); !os.IsNotExist(err) {
		t.Error("a restore left its aside copy behind")
	}
}

func TestRestoreSnapshot_aSnapshotThatDoesNotLoadKeepsTheCurrentInstallation(t *testing.T) {
	manifest, backups, _ := restoreFixture(t)
	current, _ := os.ReadFile(manifest)
	writeArchive(t, backups, "acme", "20231114T221320.000000000Z", map[string]string{
		"acme.yaml": "workspace: acme\nmachines:\n  - ../missing.yaml\napplications: []\n",
	})
	err := RestoreSnapshot(manifest, backups, "20231114T221320.000000000Z", time.Unix(1_700_000_500, 0))
	var rejected *RejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("RestoreSnapshot = %v, want a RejectedError", err)
	}
	if got, _ := os.ReadFile(manifest); string(got) != string(current) {
		t.Errorf("manifest = %q, the failed restore did not put the current one back", got)
	}
}

func TestRestoreSnapshot_refusesWhatIsNotThisWorkspacesOwn(t *testing.T) {
	manifest, backups, root := restoreFixture(t)
	// Every archive carries a manifest that loads, so the only reason to refuse is what the entry names.
	ok := "workspace: acme\nmachines:\n  - ../user.yaml\napplications: []\n"
	for name, files := range map[string]map[string]string{
		"20231114T221320.000000000Z": {"acme.yaml": ok, "../escape.yaml": "x"},
		"20231114T221321.000000000Z": {"acme.yaml": ok, "other.yaml": "workspace: other\n"},
		"20231114T221322.000000000Z": {"acme/readme.txt": "no manifest"},
	} {
		writeArchive(t, backups, "acme", name, files)
		if err := RestoreSnapshot(manifest, backups, name, time.Unix(1_700_000_500, 0)); err == nil {
			t.Errorf("%s: restore accepted an archive %v", name, files)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "escape.yaml")); err == nil {
		t.Error("an entry escaped the Workspace directory")
	}
	// A real archive one directory up, reachable only if the id may contain a path.
	writeArchive(t, backups, ".", "planted", map[string]string{"acme.yaml": ok})
	for _, bad := range []string{"../planted", "../../etc/passwd", "latest", ""} {
		if err := RestoreSnapshot(manifest, backups, bad, time.Now()); err == nil {
			t.Errorf("id %q was accepted", bad)
		}
	}
}
