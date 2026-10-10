package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/installer"
)

// workspaceSnapshots lists the current Workspace's saved installations, newest first (installer.ListSnapshots).
// It is the read a restore screen draws from; there is no GET route for it yet because the screen is an
// Experience-Plane change (K10), and a handler for it should call this rather than read BACKUP_DIR itself.
func workspaceSnapshots(ctx context.Context, store *data.Store, cfg config.Config) ([]installer.Snapshot, error) {
	if cfg.BackupDir == "" {
		return nil, nil
	}
	manifestPath, _, err := workspaceInstallation(ctx, store, cfg)
	if err != nil {
		return nil, err
	}
	return installer.ListSnapshots(manifestPath, cfg.BackupDir)
}

// submitRestoreSnapshot is the workspace-admin action that puts this Workspace's installation back to a saved
// snapshot (`snapshot` form value, an id from workspaceSnapshots) and reloads it. The restore is load-verified and
// itself snapshotted first (installer.RestoreSnapshot), so pressing it by mistake is undone by restoring the newest
// snapshot; a snapshot that does not load answers 422 and changes nothing.
func submitRestoreSnapshot(store *data.Store, cfg config.Config, reload func(slug string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if cfg.BackupDir == "" || reload == nil {
			serverError(w, errors.New("this process has no backup directory or reload hook configured"))
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		manifestPath, _, err := workspaceInstallation(ctx, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		if err := installer.RestoreSnapshot(manifestPath, cfg.BackupDir, req.PostFormValue("snapshot"), time.Now()); err != nil {
			var rejected *installer.RejectedError
			if errors.As(err, &rejected) {
				http.Error(w, fmt.Sprintf("the snapshot was not restored: %v", err), http.StatusUnprocessableEntity)
				return
			}
			serverError(w, err)
			return
		}
		if err := reloadCurrentWorkspace(ctx, store, reload); err != nil {
			serverError(w, fmt.Errorf("the snapshot was restored on disk but the live reload failed -- a process restart will pick it up: %w", err))
			return
		}
		redirectTo(w, req, "/home")
	}
}
