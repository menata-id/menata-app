package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/config"
)

// reloadTree builds metadata/{user.yaml, workspaces/<slug>.yaml} in a temp dir, using the repo's own user
// Machine, so the dynamicHandler can load real manifests without a database.
func reloadTree(t *testing.T, manifests map[string]string) string {
	t.Helper()
	root := t.TempDir()
	user, err := os.ReadFile(filepath.Join("..", "..", "metadata", "user.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "user.yaml"), user, 0o644)
	dir := filepath.Join(root, "workspaces")
	os.Mkdir(dir, 0o755)
	for name, body := range manifests {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	return dir
}

const okManifest = "workspace: %s\nmachines:\n  - ../user.yaml\napplications: []\n"

func manifestFor(slug string) string { return strings.Replace(okManifest, "%s", slug, 1) }

// TestReloadWorkspace is K10: re-reading one Workspace replaces only it; a manifest that does not load changes
// nothing; a broken Workspace becomes available the moment it loads; the others are never re-read.
func TestReloadWorkspace(t *testing.T) {
	dir := reloadTree(t, map[string]string{
		"alpha.yaml": manifestFor("alpha"),
		"beta.yaml":  manifestFor("beta"),
		"gamma.yaml": "workspace: [broken",
	})
	dh := &dynamicHandler{cfg: config.Config{MetadataPath: dir}}
	if err := dh.Reload(); err != nil {
		t.Fatalf("first build: %v", err)
	}
	if !dh.state.unavailable["gamma"] || len(dh.state.workspaces) != 2 {
		t.Fatalf("first build: unavailable=%v workspaces=%d, want gamma unavailable and two loaded", dh.state.unavailable, len(dh.state.workspaces))
	}

	// Break alpha on disk, then reload only beta: alpha is not re-read, so nothing changes for it.
	os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte("workspace: [now broken"), 0o644)
	if err := dh.ReloadWorkspace("beta"); err != nil {
		t.Fatalf("reloading beta was affected by alpha's broken file: %v", err)
	}
	if _, ok := dh.state.workspaces["alpha"]; !ok {
		t.Fatal("reloading beta dropped alpha, which is still serving its last good version")
	}
	// Reloading alpha itself fails, names its file, and leaves the live version in place.
	err := dh.ReloadWorkspace("alpha")
	if err == nil || !strings.Contains(err.Error(), "alpha.yaml") {
		t.Fatalf("reloading broken alpha: %v, want an error naming alpha.yaml", err)
	}
	if _, ok := dh.state.workspaces["alpha"]; !ok || dh.state.unavailable["alpha"] {
		t.Error("a failed reload took alpha out of service; it must keep serving what it had")
	}

	// Fix gamma: it becomes available, and the route table was rebuilt.
	os.WriteFile(filepath.Join(dir, "gamma.yaml"), []byte(manifestFor("gamma")), 0o644)
	if err := dh.ReloadWorkspace("gamma"); err != nil {
		t.Fatalf("reloading fixed gamma: %v", err)
	}
	if dh.state.unavailable["gamma"] {
		t.Error("gamma still unavailable after its manifest loaded")
	}
	rec := httptest.NewRecorder()
	dh.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("/login after reload = %d", rec.Code)
	}
}

func TestReloadWorkspace_refusesAPathShapedSlug(t *testing.T) {
	dh := &dynamicHandler{cfg: config.Config{MetadataPath: reloadTree(t, map[string]string{"a.yaml": manifestFor("a")})}}
	if err := dh.Reload(); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"", "../etc/passwd", "a/b", "a.b"} {
		if err := dh.ReloadWorkspace(slug); err == nil {
			t.Errorf("slug %q was accepted", slug)
		}
	}
}
