package web

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"time"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/installer"
)

// reloadCurrentWorkspace asks the process to re-read this request's own Workspace manifest (Deps.ReloadMetadata,
// K10). Per Workspace on purpose: the files that just changed belong to one, and re-reading the others would
// make this write's success depend on whether somebody's hand-edit elsewhere happens to load.
func reloadCurrentWorkspace(ctx context.Context, store *data.Store, reload func(slug string) error) error {
	row, ok := currentWorkspaceRow(ctx, store)
	if !ok {
		return errors.New("this request is in no workspace, so there is nothing to reload")
	}
	return reload(row.Slug)
}

// snapshotBeforeWrite saves the Workspace's installation before a write replaces files in it (installer.
// SnapshotWorkspace). Best effort: a snapshot that cannot be taken is logged and does not stop the write -- the
// write set still rolls back a write that does not load, and refusing an install because a backup directory is
// unwritable would trade a recoverable inconvenience for an outage.
func snapshotBeforeWrite(cfg config.Config, manifestPath string) {
	if cfg.BackupDir == "" {
		return
	}
	if out, err := installer.SnapshotWorkspace(manifestPath, cfg.BackupDir, time.Now()); err != nil {
		log.Printf("snapshot before writing %s failed: %v", filepath.Base(manifestPath), err)
	} else if out != "" {
		log.Printf("snapshot %s", out)
	}
}
