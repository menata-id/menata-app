package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Version names the state of one Workspace's installation: a hash over its manifest and every file under its own
// directory, in path order. A plan carries the version it was made against, so an apply made after anyone else wrote
// is recognised instead of overwriting them.
func Version(workspaceManifestPath string) (string, error) {
	slug := strings.TrimSuffix(filepath.Base(workspaceManifestPath), filepath.Ext(workspaceManifestPath))
	own := filepath.Join(filepath.Dir(workspaceManifestPath), slug)
	h := sha256.New()
	add := func(p string) error {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h.Write([]byte(p + "\x00"))
		h.Write(b)
		h.Write([]byte{0})
		return nil
	}
	if err := add(workspaceManifestPath); err != nil {
		return "", err
	}
	err := filepath.WalkDir(own, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		return add(p) // WalkDir visits in lexical order, so the hash is stable
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
