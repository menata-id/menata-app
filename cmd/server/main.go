// Command server is the Menata App entrypoint: a single binary that will grow to realize
// Runtime Metadata into a running application, per 002-architecture.md's Runtime Realization
// pipeline.
//
// This file is the composition root and nothing else. It reads configuration, loads and
// validates Runtime Metadata once, opens the pool, and hands the assembled dependencies to
// internal/web, which owns the route table and the handlers (ROADMAP.md Phase 19).
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/joho/godotenv"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/db"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
	"menata.app/internal/metadata"
	"menata.app/internal/storage"
	"menata.app/internal/web"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	if cfg.AdminUsername == "" || cfg.AdminPassword == "" || cfg.SessionSecret == "" {
		log.Fatal("ADMIN_USERNAME, ADMIN_PASSWORD, and SESSION_SECRET must all be set -- no safe default exists for a credential")
	}

	// Metadata is loaded and validated once at startup: invalid metadata must not enter
	// execution (005-runtime-lifecycle.md Phase 3-4). What is loaded is every *installed*
	// Workspace -- one manifest per Workspace, keyed by slug (2026-09-22). Loading stays at
	// startup; what changed is that there is no longer a single answer to "which Applications
	// exist", so the result is a map a request resolves into rather than one value.
	installed, err := metadata.LoadWorkspaces(cfg.MetadataPath)
	if err != nil {
		log.Fatalf("failed to load metadata: %v", err)
	}

	// Machines are still process-wide here, unioned across every installed Workspace, while
	// Applications are per-Workspace below. That split is deliberate and it is not the finished
	// shape: the Applications a Workspace shows were the bug the owner found (a brand-new
	// Workspace rendering two Applications nobody installed), and they are fixed. A Workspace's
	// *Machine set* is still shared, so /machines/... lists every Machine in every Workspace --
	// their records are workspace-scoped so the pages are empty, but the list itself is not yet
	// per-Workspace. See ROADMAP.md.
	workspaces := make(map[string]domain.Workspace, len(installed))
	machines := map[string]*domain.Machine{}
	var machineList []*domain.Machine
	for slug, ws := range installed {
		workspaces[slug] = ws.Workspace
		for _, m := range ws.Machines {
			if _, seen := machines[m.ID]; seen {
				continue
			}
			machines[m.ID] = m
			machineList = append(machineList, m)
		}
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()
	store := data.NewStore(pool)

	files, err := storage.NewStore(cfg.UploadsDir)
	if err != nil {
		log.Fatalf("failed to set up uploads directory: %v", err)
	}

	handler := web.Routes(web.Deps{
		Machines:    machines,
		MachineList: machineList,
		Store:       store,
		Files:       files,
		Mailer:      mail.NewMailerFromConfig(cfg),
		Cfg:         cfg,
		// Every installed Workspace, keyed by slug. Which one a request is in is resolved per
		// request (web's currentWorkspace middleware) from the Workspace the session is scoped to
		// -- the same shape currentApplication already had one level down.
		Workspaces: workspaces,
		// DefaultWorkspaceID is requireAuth's fallback when a signed-in identity does not resolve
		// to a real mch_user record, which is exactly the shared admin credential's placeholder
		// subject (config.AdminUserID) until it is bootstrapped to a real one (Phase 7). It is a
		// real `ws_...` id from the database now rather than a manifest's own declaration, because
		// a manifest no longer states one.
		DefaultWorkspaceID: defaultWorkspaceID(ctx, store),
	})

	log.Printf("menata-app listening on :%s (metadata: %s)", cfg.Port, cfg.MetadataPath)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}

// defaultWorkspaceID resolves requireAuth's fallback Workspace: the one slugged "default".
//
// It reads the database rather than a manifest, because a manifest no longer declares a Workspace
// id -- it names its target by slug, and the id is generated when the Workspace is created. Fatal
// when absent: the fallback exists so a session that resolves to no mch_user record still lands
// somewhere real, and a missing one would fail later, per request, as a confusing empty scope.
func defaultWorkspaceID(ctx context.Context, store *data.Store) string {
	ws, err := store.WorkspaceBySlug(ctx, "default")
	if err != nil {
		log.Fatalf("failed to resolve the default workspace (slug %q): %v", "default", err)
	}
	return ws.ID
}
