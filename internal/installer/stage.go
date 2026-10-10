package installer

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"menata.app/internal/metadata"
)

// maxStagedBytes bounds what StageWorkspace will copy: the metadata tree is a few hundred kilobytes, so a
// larger one means the configured directory is not what this function was written for.
const maxStagedBytes = 64 << 20

// StageWorkspace copies the metadata tree one Workspace loads from -- the library files beside the workspaces
// directory, and this Workspace's own manifest and directory, never another Workspace's -- into a temporary
// directory, and returns the staged manifest path with a cleanup. A dry run (ValidateFiles, aiassist.Preview)
// edits the copy; nothing it does can reach the live files or the running process.
func StageWorkspace(manifestPath string) (stagedManifest string, cleanup func(), err error) {
	manifestDir := filepath.Dir(manifestPath)
	root := filepath.Dir(manifestDir)
	slug := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	tmp, err := os.MkdirTemp("", "menata-stage-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(tmp) }
	var total int64
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		// Inside the workspaces directory only this Workspace's own files are staged.
		if parts := strings.Split(filepath.ToSlash(rel), "/"); parts[0] == filepath.Base(manifestDir) {
			if !(len(parts) == 2 && parts[1] == slug+".yaml") && !(len(parts) > 2 && parts[1] == slug) {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if total += info.Size(); total > maxStagedBytes {
			return fmt.Errorf("%s holds more than %d bytes of metadata, which is not what staging is for", root, maxStagedBytes)
		}
		dst := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, src)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		return err
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage workspace %s: %w", slug, err)
	}
	return filepath.Join(tmp, filepath.Base(manifestDir), slug+".yaml"), cleanup, nil
}

// ValidateFiles answers "would the Workspace still load with these files?" without writing anything live. files
// maps a path inside the Workspace's own directory (`document.yaml`, `applications/tracking.yaml`) to its new
// content: an existing file is replaced, a new one is added. The staged copy is read back through
// metadata.LoadApplication, the real loader, so the answer is the one a reload would give.
//
// The error, when there is one, is the loader's, with the staging directory taken out of its paths. A new file
// nothing loads is reported separately in unreferenced: the loader cannot fail on a file it never reads, and a
// "valid" that meant "ignored" would be the dangerous answer.
func ValidateFiles(manifestPath string, files map[string][]byte) (unreferenced []string, err error) {
	if len(files) == 0 {
		return nil, Rejected(fmt.Errorf("no files to validate"))
	}
	staged, cleanup, err := StageWorkspace(manifestPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	slug := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	ownDir := filepath.Join(filepath.Dir(staged), slug)
	for name, body := range files {
		clean := path.Clean(filepath.ToSlash(name))
		if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || !strings.HasSuffix(clean, ".yaml") {
			return nil, Rejected(fmt.Errorf("%q is not a .yaml path inside the workspace's own directory", name))
		}
		dst := filepath.Join(ownDir, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return nil, err
		}
	}
	loaded, err := metadata.LoadApplication(staged)
	if err != nil {
		return nil, Rejected(fmt.Errorf("%s", strings.ReplaceAll(err.Error(), filepath.Dir(filepath.Dir(staged))+string(filepath.Separator), "")))
	}
	known := map[string]bool{}
	for _, m := range loaded.Workspace.Machines {
		known[m.ID] = true
	}
	for _, a := range loaded.Workspace.Applications {
		known[a.ID] = true
	}
	for name, body := range files {
		var head struct {
			ID string `yaml:"id"`
		}
		if yaml.Unmarshal(body, &head) != nil || !known[head.ID] {
			unreferenced = append(unreferenced, name)
		}
	}
	return unreferenced, nil
}
