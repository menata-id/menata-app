package installer

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SnapshotKeep is how many snapshots of one Workspace are retained.
const SnapshotKeep = 20

// SnapshotWorkspace writes a gzip tar of one Workspace's installation -- its manifest and the directory of
// copies beside it -- into backupDir/<slug>/<UTC timestamp>.tar.gz, and prunes that Workspace's older ones
// beyond SnapshotKeep (K10; audit option B: "backup direktori metadata/workspaces/").
//
// Metadata lives in files, and a write through the installer or the assistant rewrites them in place. The
// write set rolls back a write that fails to load; this is for the other kind -- a write that loads and was
// wrong. Only the Workspace being written is captured, never the shared library, which is only ever read.
// A manifest that does not exist yet has nothing to snapshot and returns "" with no error.
func SnapshotWorkspace(manifestPath, backupDir string, now time.Time) (string, error) {
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	slug := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	root := filepath.Dir(manifestPath)
	dir := filepath.Join(backupDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(dir, now.UTC().Format("20060102T150405.000000000Z")+".tar.gz")
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(path string) error {
		return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(rel)
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			src, err := os.Open(p)
			if err != nil {
				return err
			}
			defer src.Close()
			_, err = io.Copy(tw, src)
			return err
		})
	}
	err = add(manifestPath)
	if err == nil {
		if _, statErr := os.Stat(filepath.Join(root, slug)); statErr == nil {
			err = add(filepath.Join(root, slug))
		}
	}
	for _, c := range []io.Closer{tw, gz, f} {
		if cerr := c.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		os.Remove(out)
		return "", fmt.Errorf("snapshot %s: %w", slug, err)
	}
	pruneSnapshots(dir, SnapshotKeep)
	return out, nil
}

func pruneSnapshots(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tar.gz") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // timestamped names sort oldest first
	for len(names) > keep {
		os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// SnapshotWorkspaceIfChanged is SnapshotWorkspace unless the newest snapshot already holds exactly what is on disk
// (same files, same bytes), in which case it returns "" and writes nothing. It is for the moments that are not a
// write -- a reload after a hand edit -- where an unconditional snapshot would let repeated presses of the button
// push the snapshots taken before real changes out of the SnapshotKeep window.
func SnapshotWorkspaceIfChanged(manifestPath, backupDir string, now time.Time) (string, error) {
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	slug := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	if existing, err := ListSnapshots(manifestPath, backupDir); err == nil && len(existing) > 0 {
		if saved, err := readSnapshot(filepath.Join(backupDir, slug, existing[0].ID+".tar.gz"), slug); err == nil {
			if same, _ := sameAsDisk(manifestPath, slug, saved); same {
				return "", nil
			}
		}
	}
	return SnapshotWorkspace(manifestPath, backupDir, now)
}

// sameAsDisk reports whether the saved files are exactly the Workspace's manifest and own directory on disk.
func sameAsDisk(manifestPath, slug string, saved []snapshotFile) (bool, error) {
	root := filepath.Dir(manifestPath)
	onDisk := map[string][]byte{}
	add := func(p string) error {
		return filepath.WalkDir(p, func(f string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err
			}
			rel, _ := filepath.Rel(root, f)
			b, err := os.ReadFile(f)
			onDisk[filepath.ToSlash(rel)] = b
			return err
		})
	}
	if err := add(manifestPath); err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(root, slug)); err == nil {
		if err := add(filepath.Join(root, slug)); err != nil {
			return false, err
		}
	}
	if len(onDisk) != len(saved) {
		return false, nil
	}
	for _, f := range saved {
		if b, ok := onDisk[f.name]; !ok || string(b) != string(f.body) {
			return false, nil
		}
	}
	return true, nil
}
