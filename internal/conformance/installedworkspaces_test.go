package conformance

import (
	"path/filepath"

	"menata.app/internal/metadata"
)

// loadInstalledWorkspaces is every Workspace the gates sweep: the ones installed under metadata/workspaces/ and the tracked
// samples under metadata/samples/.
//
// metadata/workspaces/ holds what people build through the UI and is git-ignored except `default`, so a clean clone (CI) has
// only that one. The samples are real installed copies of the library and of the hand-written Applications the gates measure
// (a linking Metric, a `page:` over documents, Production Contracts' computed trees); they stand in for the Workspaces a
// developer's own checkout happens to hold, and a gate that needs a shape names it through them. Slugs never collide: a sample's
// is prefixed `sample-`.
func loadInstalledWorkspaces() (metadata.Workspaces, error) {
	all, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "workspaces"))
	if err != nil {
		return nil, err
	}
	samples, err := metadata.LoadWorkspaces(filepath.Join(repoRoot(), "metadata", "samples"))
	if err != nil {
		return nil, err
	}
	for slug, ws := range samples {
		all[slug] = ws
	}
	return all, nil
}
