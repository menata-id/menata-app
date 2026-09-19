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
	// execution (005-runtime-lifecycle.md Phase 3-4).
	app, err := metadata.LoadApplication(cfg.MetadataPath)
	if err != nil {
		log.Fatalf("failed to load metadata: %v", err)
	}
	machines := make(map[string]*domain.Machine, len(app.Machines))
	for _, m := range app.Machines {
		machines[m.ID] = m
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
		MachineList: app.Machines,
		Store:       store,
		Files:       files,
		Cfg:         cfg,
		AppName:     app.Application.Name,
		// DefaultWorkspaceID is this manifest's own declared Workspace (ROADMAP.md Phase 21 Step
		// 2 -- "Workspace never enters the data path" closed) -- requireAuth's fallback when a
		// signed-in identity does not resolve to a real mch_user record, which is exactly the
		// shared admin credential's placeholder subject (config.AdminUserID) until it is
		// bootstrapped to a real one (Phase 7).
		DefaultWorkspaceID: app.Workspace.ID,
	})

	log.Printf("menata-app listening on :%s (metadata: %s)", cfg.Port, cfg.MetadataPath)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}
