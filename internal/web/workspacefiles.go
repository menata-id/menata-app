package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ownedYAMLFiles lists, as slash paths relative to the Workspace's own directory, every .yaml file in it -- the
// set installer.ApplyFiles will agree to replace. A Workspace that has no directory yet owns none.
func ownedYAMLFiles(ownDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(ownDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".yaml") {
			rel, relErr := filepath.Rel(ownDir, p)
			if relErr != nil {
				return relErr
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	slices.Sort(out)
	return out, err
}
