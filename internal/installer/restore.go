package installer

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"menata.app/internal/metadata"
)

// Snapshot is one saved copy of a Workspace's installation (SnapshotWorkspace).
type Snapshot struct {
	// ID names the snapshot to RestoreSnapshot: its file name without ".tar.gz", a UTC timestamp.
	ID      string
	TakenAt time.Time
	Size    int64
}

var snapshotID = regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}Z$`)

const snapshotLayout = "20060102T150405.000000000Z"

// ListSnapshots is the Workspace's snapshots, newest first. A Workspace never snapshotted has none and no error.
func ListSnapshots(manifestPath, backupDir string) ([]Snapshot, error) {
	slug := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	entries, err := os.ReadDir(filepath.Join(backupDir, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Snapshot
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".tar.gz")
		if e.IsDir() || id == e.Name() || !snapshotID.MatchString(id) {
			continue
		}
		at, err := time.Parse(snapshotLayout, id)
		info, ierr := e.Info()
		if err != nil || ierr != nil {
			continue
		}
		out = append(out, Snapshot{ID: id, TakenAt: at, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// RestoreSnapshot puts a Workspace's installation back to a saved snapshot: its manifest and its directory of
// copies are replaced wholesale, so a file added after the snapshot goes too (K10).
//
// The order is what makes it safe to press:
//  1. the id is checked against the snapshot name pattern, so it can name nothing but a file in backupDir/<slug>;
//  2. every entry is checked before anything is touched -- only this Workspace's manifest and its own directory,
//     only regular files, nothing escaping the directory;
//  3. the current state is snapshotted first, so a restore is itself undoable, and moved aside rather than deleted;
//  4. the restored files are read back through metadata.LoadApplication, the real loader, and if they do not load
//     the current state is put back and the restore is rejected with the loader's error.
//
// It does not reload the running process; the caller does (Deps.ReloadMetadata).
func RestoreSnapshot(manifestPath, backupDir, id string, now time.Time) error {
	if !snapshotID.MatchString(id) {
		return Rejected(fmt.Errorf("%q is not a snapshot id", id))
	}
	slug := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	root := filepath.Dir(manifestPath)
	files, err := readSnapshot(filepath.Join(backupDir, slug, id+".tar.gz"), slug)
	if err != nil {
		return Rejected(err)
	}
	if _, err := SnapshotWorkspace(manifestPath, backupDir, now); err != nil {
		return fmt.Errorf("snapshot before restoring: %w", err)
	}

	ownDir := filepath.Join(root, slug)
	asideManifest, asideDir := manifestPath+".restore-bak", ownDir+".restore-bak"
	_ = os.RemoveAll(asideManifest)
	_ = os.RemoveAll(asideDir)
	if err := os.Rename(manifestPath, asideManifest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(ownDir, asideDir); err != nil && !os.IsNotExist(err) {
		undoAside(manifestPath, asideManifest, ownDir, asideDir)
		return err
	}
	fail := func(err error) error {
		_ = os.Remove(manifestPath)
		_ = os.RemoveAll(ownDir)
		undoAside(manifestPath, asideManifest, ownDir, asideDir)
		return err
	}
	for _, f := range files {
		dst := filepath.Join(root, filepath.FromSlash(f.name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(dst, f.body, 0o644); err != nil {
			return fail(err)
		}
	}
	if _, err := metadata.LoadApplication(manifestPath); err != nil {
		return fail(Rejected(fmt.Errorf("the snapshot does not load, so the current installation was kept: %w", err)))
	}
	_ = os.RemoveAll(asideManifest)
	_ = os.RemoveAll(asideDir)
	return nil
}

func undoAside(manifestPath, asideManifest, ownDir, asideDir string) {
	if _, err := os.Stat(asideManifest); err == nil {
		_ = os.Rename(asideManifest, manifestPath)
	}
	if _, err := os.Stat(asideDir); err == nil {
		_ = os.Rename(asideDir, ownDir)
	}
}

type snapshotFile struct {
	name string
	body []byte
}

// readSnapshot reads and vets a whole archive before anything is written from it.
func readSnapshot(file, slug string) ([]snapshotFile, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("snapshot not found")
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("snapshot is not a readable archive: %w", err)
	}
	tr := tar.NewReader(gz)
	var out []snapshotFile
	sawManifest := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot is not a readable archive: %w", err)
		}
		name := path.Clean(hdr.Name)
		if hdr.Typeflag != tar.TypeReg || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") ||
			!(name == slug+".yaml" || strings.HasPrefix(name, slug+"/")) {
			return nil, fmt.Errorf("snapshot holds an entry %q that is not part of workspace %q", hdr.Name, slug)
		}
		body, err := io.ReadAll(io.LimitReader(tr, 32<<20))
		if err != nil {
			return nil, err
		}
		sawManifest = sawManifest || name == slug+".yaml"
		out = append(out, snapshotFile{name: name, body: body})
	}
	if !sawManifest {
		return nil, fmt.Errorf("snapshot holds no manifest for workspace %q", slug)
	}
	return out, nil
}
