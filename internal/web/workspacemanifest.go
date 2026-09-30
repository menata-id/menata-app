package web

import (
	"context"
	"errors"
	"log"
	"path/filepath"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/installer"
)

// workspaceInstallation is what a handler writing into this request's Workspace writes to: its
// manifest path and the Workspace that manifest installs, read from disk.
//
// The slug comes from the `workspaces` row, never from rendering.CurrentWorkspace. A Workspace with
// no manifest resolves to the zero Workspace there, whose slug is empty, and building a path from it
// aimed the first publish into every new Workspace at metadata/workspaces/.yaml (2026-09-30).
//
// A missing manifest is written here rather than refused: Workspaces created before creation began
// writing one have none, and this is where they heal, on their first write.
func workspaceInstallation(ctx context.Context, store *data.Store, cfg config.Config) (string, domain.Workspace, error) {
	row, ok := currentWorkspaceRow(ctx, store)
	if !ok {
		return "", domain.Workspace{}, errors.New("this request is in no workspace, so there is no installation to write to")
	}
	ws, _, err := installer.EnsureWorkspaceManifest(cfg.MetadataPath, cfg.TemplatePath, row.Slug)
	if err != nil {
		return "", domain.Workspace{}, err
	}
	return filepath.Join(cfg.MetadataPath, row.Slug+".yaml"), ws, nil
}

// ensureNewWorkspaceManifest gives a just-created Workspace its empty installation, so it is a
// Workspace the write paths can install into from its first request.
//
// Deliberately not followed by a reload. Reloading rebuilds the whole route table, which resets the
// registration rate limiter, and /register is a public route: reloading on every sign-up would let
// sign-ups clear their own limit. Nothing needs it -- an empty installation reads exactly like the
// zero Workspace a request already resolves to, and every write loads the manifest from disk.
//
// Written after the `workspaces` row, and a failure is logged rather than failing the creation: the
// row without a manifest is exactly what workspaceInstallation heals on the first write. The reverse
// order would leave a manifest for a Workspace that does not exist, and the loader would load it.
func ensureNewWorkspaceManifest(cfg config.Config, slug string) {
	if _, _, err := installer.EnsureWorkspaceManifest(cfg.MetadataPath, cfg.TemplatePath, slug); err != nil {
		log.Printf("workspace %q was created without its manifest; its first install or publish will write it: %v", slug, err)
	}
}
