package installer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"menata.app/internal/metadata"
)

// topLevelInactive matches the `inactive:` key at the left margin of an Application file, whatever value it holds.
var topLevelInactive = regexp.MustCompile(`(?m)^inactive:[^\n]*\n?`)

// topLevelID matches the Application's own `id:` line, after which the switch is written.
var topLevelID = regexp.MustCompile(`(?m)^id:[^\n]*\n`)

// SetApplicationInactive switches one installed Application off (`inactive: true`) or on (the key removed) in this Workspace's
// own copy of its file, and load-verifies the result: a file that does not load is put back and the change is refused. It is a
// one-line text edit, not a re-encoding of the document, so a long hand-written Application file keeps every comment and its
// formatting. It returns whether the file changed -- asking for the state it is already in writes nothing, so it takes no
// snapshot's worth of churn. Records, Machines and every other key are untouched (domain.Application.Inactive).
func SetApplicationInactive(workspaceManifestPath, appID string, inactive bool) (changed bool, err error) {
	rel, err := applicationFileOf(workspaceManifestPath, appID)
	if err != nil {
		return false, err
	}
	path := filepath.Join(filepath.Dir(workspaceManifestPath), rel)
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var next []byte
	switch {
	case inactive:
		if !topLevelID.Match(src) {
			return false, Rejected(fmt.Errorf("%s has no top-level id: line to write the switch after", rel))
		}
		line := []byte("inactive: true\n")
		if topLevelInactive.Match(src) {
			next = topLevelInactive.ReplaceAll(src, line)
		} else {
			loc := topLevelID.FindIndex(src)
			next = append(append(append([]byte{}, src[:loc[1]]...), line...), src[loc[1]:]...)
		}
	default:
		next = topLevelInactive.ReplaceAll(src, nil)
	}
	if bytes.Equal(src, next) {
		return false, nil
	}
	written := NewWriteSet()
	if err := written.Note(path); err != nil {
		return false, err
	}
	if err := WriteFileStrict[FullApplicationCheckDoc](path, next); err != nil {
		written.Rollback()
		return false, err
	}
	if _, err := metadata.LoadApplication(workspaceManifestPath); err != nil {
		written.Rollback()
		return false, Rejected(fmt.Errorf("the switch was written but the Workspace does not load, so it has been put back: %w", err))
	}
	return true, nil
}

// applicationFileOf is the path, relative to the manifest, of the file that declares the Application.
func applicationFileOf(workspaceManifestPath, appID string) (string, error) {
	manifest, err := os.ReadFile(workspaceManifestPath)
	if err != nil {
		return "", err
	}
	var doc struct {
		Applications []string `yaml:"applications"`
	}
	if err := yaml.Unmarshal(manifest, &doc); err != nil {
		return "", err
	}
	for _, rel := range doc.Applications {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(workspaceManifestPath), rel))
		if err != nil {
			return "", err
		}
		var idDoc struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(b, &idDoc); err != nil {
			return "", err
		}
		if idDoc.ID == appID {
			return rel, nil
		}
	}
	return "", Rejected(fmt.Errorf("application %q is not installed in this workspace", appID))
}
