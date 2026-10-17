package web

import (
	"context"

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
