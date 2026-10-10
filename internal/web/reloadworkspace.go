package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
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

// submitReloadWorkspace is the workspace-admin action that re-reads this Workspace's manifest from disk -- what
// to press after fixing a hand-edit, or after a Workspace was marked unavailable. A manifest that fails to load
// changes nothing live and answers 422 with the error, which names the file (metadata.LoadApplication).
//
// Sits in the admin group beside /install-application and, like it, redirects home on success. The button that
// posts here is an Experience-Plane change and is not part of this handler.
func submitReloadWorkspace(store *data.Store, cfg config.Config, reload func(slug string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if reload == nil {
			serverError(w, errors.New("this process has no reload hook configured"))
			return
		}
		// A hand edit is not a write the runtime made, so no snapshot was taken for it: take one now, before the
		// reload, unless the newest already holds exactly this state.
		if manifestPath, _, err := workspaceInstallation(req.Context(), store, cfg); err == nil && cfg.BackupDir != "" {
			if out, err := installer.SnapshotWorkspaceIfChanged(manifestPath, cfg.BackupDir, time.Now()); err != nil {
				log.Printf("snapshot before reloading %s failed: %v", filepath.Base(manifestPath), err)
			} else if out != "" {
				log.Printf("snapshot %s", out)
			}
		}
		if err := reloadCurrentWorkspace(req.Context(), store, reload); err != nil {
			http.Error(w, fmt.Sprintf("this workspace was not reloaded and keeps serving what it had: %v", err), http.StatusUnprocessableEntity)
			return
		}
		redirectTo(w, req, "/home")
	}
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
