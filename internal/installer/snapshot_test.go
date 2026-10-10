package installer

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotWorkspace_capturesManifestAndCopiesAndPrunes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "workspaces")
	if err := os.MkdirAll(filepath.Join(dir, "acme", "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "acme.yaml")
	for path, body := range map[string]string{
		manifest: "workspace: acme\n",
		filepath.Join(dir, "acme", "applications", "a.yaml"): "id: app_a\n",
		filepath.Join(dir, "other.yaml"):                     "workspace: other\n", // a different Workspace: not captured
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backups := filepath.Join(root, "backups")

	first, err := SnapshotWorkspace(manifest, backups, time.Unix(1_700_000_000, 0))
	if err != nil || first == "" {
		t.Fatalf("SnapshotWorkspace = %q, %v", first, err)
	}
	got := map[string]string{}
	f, _ := os.Open(first)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		got[h.Name] = string(b)
	}
	if got["acme.yaml"] != "workspace: acme\n" || got["acme/applications/a.yaml"] != "id: app_a\n" || len(got) != 2 {
		t.Errorf("snapshot holds %v, want exactly acme.yaml and acme/applications/a.yaml", got)
	}

	for i := 1; i <= SnapshotKeep+3; i++ {
		if _, err := SnapshotWorkspace(manifest, backups, time.Unix(1_700_000_000+int64(i), 0)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(backups, "acme"))
	if len(entries) != SnapshotKeep {
		t.Errorf("%d snapshots kept, want %d", len(entries), SnapshotKeep)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Error("the oldest snapshot survived pruning")
	}
}

func TestSnapshotWorkspace_aMissingManifestHasNothingToSave(t *testing.T) {
	out, err := SnapshotWorkspace(filepath.Join(t.TempDir(), "none.yaml"), t.TempDir(), time.Now())
	if out != "" || err != nil {
		t.Errorf("SnapshotWorkspace = %q, %v, want nothing and no error", out, err)
	}
}
