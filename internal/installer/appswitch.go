package installer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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

// RemovedApplication is what RemoveApplication took out of the Workspace: the Application's own file and the Machine files only it claimed.
type RemovedApplication struct {
	ApplicationID string
	MachineIDs    []string
	Files         []string // relative to the manifest's directory
}

// RemoveApplication takes one Application out of a Workspace's installation: its file, the Machine files that Application alone
// claims, and their lines in the manifest. It is the exact inverse of publishing a new Application, which only ever adds those
// files and those lines, and it touches nothing else -- no other Application, no shared Machine (a Machine file outside this
// Workspace's own directory is never removed), no record. Records are the caller's business: removing the Machines of an
// Application that holds records is the caller's refusal to make, because the installer has no store.
//
// The result must load; if it does not the files and the manifest are put back and the removal is refused.
func RemoveApplication(workspaceManifestPath, appID string) (RemovedApplication, error) {
	out := RemovedApplication{ApplicationID: appID}
	appRel, err := applicationFileOf(workspaceManifestPath, appID)
	if err != nil {
		return out, err
	}
	dir := filepath.Dir(workspaceManifestPath)
	slug := strings.TrimSuffix(filepath.Base(workspaceManifestPath), filepath.Ext(workspaceManifestPath))
	own := slug + "/"
	if !strings.HasPrefix(filepath.ToSlash(appRel), own) {
		return out, Rejected(fmt.Errorf("application %q lives in %s, outside this workspace's own directory, so it is not removed", appID, appRel))
	}
	appSrc, err := os.ReadFile(filepath.Join(dir, appRel))
	if err != nil {
		return out, err
	}
	var appDoc struct {
		Machines []string `yaml:"machines"`
	}
	if err := yaml.Unmarshal(appSrc, &appDoc); err != nil {
		return out, err
	}
	out.MachineIDs = appDoc.Machines

	manifest, err := os.ReadFile(workspaceManifestPath)
	if err != nil {
		return out, err
	}
	var mdoc struct {
		Machines []string `yaml:"machines"`
	}
	if err := yaml.Unmarshal(manifest, &mdoc); err != nil {
		return out, err
	}
	files := []string{appRel}
	for _, rel := range mdoc.Machines {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return out, err
		}
		var idDoc struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(b, &idDoc); err != nil {
			return out, err
		}
		if !slices.Contains(appDoc.Machines, idDoc.ID) {
			continue
		}
		if !strings.HasPrefix(filepath.ToSlash(rel), own) {
			return out, Rejected(fmt.Errorf("machine %s of %s is a shared file (%s), so the application is not removed", idDoc.ID, appID, rel))
		}
		files = append(files, rel)
	}
	next := manifest
	for _, rel := range files {
		var ok bool
		if next, ok = dropManifestLine(next, rel); !ok {
			return out, Rejected(fmt.Errorf("the manifest has no line for %s", rel))
		}
	}

	next = emptyListsAsFlow(next)

	written := NewWriteSet()
	fail := func(err error) (RemovedApplication, error) { written.Rollback(); return out, err }
	if err := written.Note(workspaceManifestPath); err != nil {
		return fail(err)
	}
	for _, rel := range files {
		if err := written.Note(filepath.Join(dir, rel)); err != nil {
			return fail(err)
		}
	}
	if err := WriteFileStrict[WorkspaceManifestCheckDoc](workspaceManifestPath, next); err != nil {
		return fail(err)
	}
	for _, rel := range files {
		if err := os.Remove(filepath.Join(dir, rel)); err != nil {
			return fail(err)
		}
	}
	if _, err := metadata.LoadApplication(workspaceManifestPath); err != nil {
		return fail(Rejected(fmt.Errorf("the Workspace does not load without %s, so it has been put back: %w", appID, err)))
	}
	out.Files = files
	return out, nil
}

// dropManifestLine removes the one list item naming rel from the manifest text, whatever quoting it has.
func dropManifestLine(manifest []byte, rel string) ([]byte, bool) {
	re := regexp.MustCompile(`(?m)^[ \t]*-[ \t]*["']?` + regexp.QuoteMeta(filepath.ToSlash(rel)) + `["']?[ \t]*\r?\n?`)
	if !re.Match(manifest) {
		return manifest, false
	}
	return re.ReplaceAllLiteral(manifest, nil), true
}

// emptyListLine matches a top-level `applications:` or `machines:` key that no list item follows.
var emptyListLine = regexp.MustCompile(`(?m)^(applications|machines):[ \t]*\r?\n([ \t]*(#[^\n]*)?\r?\n)*([^ \t\r\n-]|\z)`)

// emptyListsAsFlow writes a manifest list that lost its last item as `key: []`, the form an empty Workspace starts with and the one
// AppendBlockListItem appends to. Left as a bare `key:` it would load (as nothing) but the next publish would refuse to append to it.
func emptyListsAsFlow(manifest []byte) []byte {
	return emptyListLine.ReplaceAllFunc(manifest, func(m []byte) []byte {
		key := m[:bytes.IndexByte(m, ':')]
		tail := m[bytes.IndexAny(m, "\n")+1:]
		return append(append(append([]byte{}, key...), []byte(": []\n")...), tail...)
	})
}
