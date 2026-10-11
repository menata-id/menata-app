package aiassist

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"menata.app/internal/installer"
	"menata.app/internal/metadata"
)

// FileChange is one file a change would write, as the review screen shows it (D2, option G: "the review shows the
// YAML and the diff"). Added carries the whole file in Diff, every line marked "+"; a changed file carries a
// unified-style diff with three lines of context.
type FileChange struct {
	Path   string // relative to the workspaces directory, e.g. "acme.yaml" or "acme/document.yaml"
	Status string // "added" or "changed"
	Diff   string
}

// Preview shows what Write would put on disk for a change, without writing anything live: it stages a copy of the
// Workspace (installer.StageWorkspace), runs the real Write against the copy, and diffs the copy with the live
// files. So the preview *is* the writer's output, not a second rendering of the change that could drift from it --
// the reason a preview built from the Generated* structs would be wrong in exactly the cases that matter.
//
// A removal waiting on the owner's confirmation is previewed as if confirmed, since showing the file without it
// would hide the very thing the checkbox is for. A change the writer refuses returns the writer's error.
func Preview(workspaceManifestPath string, change GeneratedChange) ([]FileChange, error) {
	staged, cleanup, err := installer.StageWorkspace(workspaceManifestPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if change.Kind == KindUpdateApplication {
		change.ConfirmedRemovals = allRemovalKeys(staged, change)
	}
	if _, err := Write(staged, change, FileMachineResolver{WorkspaceManifestPath: staged}); err != nil {
		return nil, err
	}

	liveDir, stagedDir := filepath.Dir(workspaceManifestPath), filepath.Dir(staged)
	slug := strings.TrimSuffix(filepath.Base(workspaceManifestPath), filepath.Ext(workspaceManifestPath))
	var out []FileChange
	err = filepath.WalkDir(stagedDir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return werr
		}
		rel, _ := filepath.Rel(stagedDir, p)
		slash := filepath.ToSlash(rel)
		if slash != slug+".yaml" && !strings.HasPrefix(slash, slug+"/") {
			return nil
		}
		now, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		was, err := os.ReadFile(filepath.Join(liveDir, rel))
		switch {
		case os.IsNotExist(err):
			out = append(out, FileChange{Path: slash, Status: "added", Diff: installer.UnifiedDiff("", string(now))})
		case err != nil:
			return err
		case string(was) != string(now):
			out = append(out, FileChange{Path: slash, Status: "changed", Diff: installer.UnifiedDiff(string(was), string(now))})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// allRemovalKeys is every confirmation key the change's plan asks for, so a preview can show the removal itself.
func allRemovalKeys(manifestPath string, change GeneratedChange) []string {
	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil || change.Application == nil {
		return nil
	}
	cur, ok := ExistingStateFrom(loaded.Workspace).Applications[change.TargetAppID]
	if !ok {
		return nil
	}
	return PlanUpdate(DescribeApplication(change.TargetAppID, cur), *change.Application).Unconfirmed(nil)
}
