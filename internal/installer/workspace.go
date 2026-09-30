package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"menata.app/internal/domain"
	"menata.app/internal/metadata"
)

// EnsureWorkspaceManifest makes sure metadata/workspaces/<slug>.yaml exists, writing the empty
// installation if it does not, and returns the Workspace that file installs, loaded from disk.
//
// Creating a Workspace used to write a `workspaces` row and no manifest (found 2026-09-30,
// menata-app-document's audits/2026-09-30-kajian-workspace-baru-tanpa-manifest.md). Reading
// tolerated that -- no manifest resolves to the zero Workspace -- but every write derived its path
// from that zero Workspace's empty slug, so the first publish into a new Workspace aimed at
// metadata/workspaces/.yaml. Every manifest that existed had been written by hand.
//
// The Workspace is returned from disk rather than taken from the request because a Workspace created
// since the last reload is still the zero Workspace on ctx, and planning against that is wrong: an
// install would add the shared Machine references this file already holds.
//
// A load failure removes a file this call created; an existing file is never touched.
func EnsureWorkspaceManifest(manifestDir, libraryDir, slug string) (ws domain.Workspace, created bool, err error) {
	if !metadata.ValidWorkspaceSlug(slug) {
		return domain.Workspace{}, false, fmt.Errorf("refusing workspace manifest for slug %q: not a valid workspace slug", slug)
	}
	if manifestDir == "" {
		return domain.Workspace{}, false, errors.New("no metadata directory is configured, so there is no workspace manifest to read or write")
	}
	path := filepath.Join(manifestDir, slug+".yaml")
	switch _, statErr := os.Stat(path); {
	case statErr == nil:
		app, err := metadata.LoadApplication(path)
		if err != nil {
			return domain.Workspace{}, false, err
		}
		return app.Workspace, false, nil
	case !errors.Is(statErr, os.ErrNotExist):
		return domain.Workspace{}, false, fmt.Errorf("check workspace manifest %s: %w", path, statErr)
	}

	if libraryDir == "" {
		return domain.Workspace{}, false, errors.New("no template library is configured, so the shared machines a new workspace manifest references cannot be located")
	}

	body, err := emptyManifest(manifestDir, libraryDir, slug)
	if err != nil {
		return domain.Workspace{}, false, err
	}
	written := NewWriteSet()
	defer func() {
		if err != nil {
			written.Rollback()
		}
	}()
	if err = written.Note(path); err != nil {
		return domain.Workspace{}, false, err
	}
	if err = WriteFileStrict[WorkspaceManifestCheckDoc](path, body); err != nil {
		return domain.Workspace{}, false, fmt.Errorf("write workspace manifest %s: %w", path, err)
	}
	app, err := metadata.LoadApplication(path)
	if err != nil {
		return domain.Workspace{}, false, fmt.Errorf("the new workspace manifest does not load, so it has been removed: %w", err)
	}
	return app.Workspace, true, nil
}

// emptyManifest is a Workspace with nothing installed: the three runtime-level Machines every
// Workspace references (SharedMachineIDs) and `applications: []`, which Install and aiassist.Write
// both append to.
func emptyManifest(manifestDir, libraryDir, slug string) ([]byte, error) {
	ids := make([]string, 0, len(SharedMachineIDs))
	for id := range SharedMachineIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	fmt.Fprintf(&b, "# Created with this Workspace. Installing an Application adds to the lists below.\nworkspace: %s\n\nmachines:\n", slug)
	for _, id := range ids {
		file := filepath.Join(libraryDir, sharedFileFor(id))
		if _, err := os.Stat(file); err != nil {
			return nil, fmt.Errorf("every workspace references %s, but %s is not in the library: %w", id, sharedFileFor(id), err)
		}
		rel, err := filepath.Rel(manifestDir, file)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "  - %s\n", toSlash(rel))
	}
	b.WriteString("\napplications: []\n")
	return []byte(b.String()), nil
}

// WorkspaceOwnDir is metadata/workspaces/<slug>/ for a manifest at metadata/workspaces/<slug>.yaml:
// where everything installed into that Workspace is written.
//
// Refuses a manifest path that does not name a valid slug. An empty one made this the manifest
// directory itself, so a generated Application's files were written straight into the directory the
// loader scans as manifests, and only the rollback kept them from stopping the process at the next
// load.
func WorkspaceOwnDir(workspaceManifestPath string) (string, error) {
	slug := strings.TrimSuffix(filepath.Base(workspaceManifestPath), filepath.Ext(workspaceManifestPath))
	if !metadata.ValidWorkspaceSlug(slug) {
		return "", fmt.Errorf("refusing to write into workspace manifest %s: %q is not a valid workspace slug", workspaceManifestPath, slug)
	}
	return filepath.Join(filepath.Dir(workspaceManifestPath), slug), nil
}

// RejectedError marks a write refused because of the metadata being written -- an id that would
// overwrite an existing file, or a result the real loader will not load. A different proposal can
// pass. Anything else a writer returns is the environment (a missing or unreadable file, a failed
// disk write), which no change to the metadata can fix.
//
// The distinction exists for the AI assistant's publish handler: it hands a rejection back to the
// conversation to be corrected, and until 2026-09-30 it handed back everything, so a missing
// manifest came back to the model as a mistake to fix and the model told the person to retry.
type RejectedError struct{ Err error }

func (e *RejectedError) Error() string { return e.Err.Error() }
func (e *RejectedError) Unwrap() error { return e.Err }

// Rejected wraps err as a RejectedError; nil stays nil.
func Rejected(err error) error {
	if err == nil {
		return nil
	}
	return &RejectedError{Err: err}
}
