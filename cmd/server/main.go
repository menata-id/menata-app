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
	"sync"
	"sync/atomic"

	"github.com/joho/godotenv"

	"menata.app/internal/aiassist"
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

	// The tracer is paired with the pool here because it is the one place that may hold both:
	// internal/db must not import internal/data (plane rule), and internal/data does not build
	// pools. data.QueryTracer counts every statement against the request's own ReadLog.
	pool, err := db.Connect(ctx, cfg.DatabaseURL, data.NewQueryTracer())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()
	store := data.NewStore(pool)

	files, err := storage.NewStore(cfg.UploadsDir)
	if err != nil {
		log.Fatalf("failed to set up uploads directory: %v", err)
	}

	dh, err := newDynamicHandler(cfg, store, files, defaultWorkspaceID(ctx, store))
	if err != nil {
		log.Fatalf("failed to load metadata: %v", err)
	}

	log.Printf("menata-app listening on :%s (metadata: %s)", cfg.Port, cfg.MetadataPath)
	if err := http.ListenAndServe(":"+cfg.Port, dh); err != nil {
		log.Fatal(err)
	}
}

// dynamicHandler holds the whole route table behind an atomic pointer, so it can be rebuilt from
// metadata on disk and swapped in atomically -- the narrow, additive-only slice of
// metadata-hot-reload-safety.md's own design that the AI Metadata Assistant needs (Flow 2 gap
// study Tahap 8) and general hot-reload does not yet have.
//
// This is deliberately *not* the AppState-inside-Deps shape that design sketches (§3.4): every
// handler in internal/web closes over Deps's plain field values once, at web.Routes(d) call time,
// and there is no forcing case yet to change that everywhere it happens. Treating the *whole route
// table* as the swappable unit gets the same property (an in-flight request finishes on a fully
// self-consistent old snapshot; a request arriving after the swap sees the new one, atomically, no
// lock on the hot read path) by reusing web.Routes/web.Deps completely unchanged, at the cost of
// discarding a handful of in-memory-only counters (the login/registration rate limiters
// web.Routes constructs fresh each call) on the rare, deliberate admin action that triggers a
// reload -- an accepted, named trade-off, not an oversight.
type dynamicHandler struct {
	current atomic.Pointer[http.Handler]
	mu      sync.Mutex // serializes concurrent reload attempts (mirrors hot-reload-safety.md §3.5)

	cfg                config.Config
	store              *data.Store
	files              *storage.Store
	mailer             mail.Mailer
	aiClient           aiassist.Client
	defaultWorkspaceID string
}

func newDynamicHandler(cfg config.Config, store *data.Store, files *storage.Store, defaultWorkspaceID string) (*dynamicHandler, error) {
	dh := &dynamicHandler{
		cfg: cfg, store: store, files: files,
		mailer:             mail.NewMailerFromConfig(cfg),
		aiClient:           aiassist.NewClientFromConfig(cfg),
		defaultWorkspaceID: defaultWorkspaceID,
	}
	if err := dh.Reload(); err != nil {
		return nil, err
	}
	return dh, nil
}

func (dh *dynamicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := dh.current.Load()
	(*handler).ServeHTTP(w, r)
}

// Reload re-runs metadata.LoadWorkspaces from disk (the identical call this file always made at
// startup) and, only if it succeeds, builds a brand-new route table and swaps it in -- a parse or
// validation failure leaves whatever is currently live completely untouched and returns the error
// to the caller (internal/web's own AI-assistant publish handler, via Deps.ReloadMetadata), which
// must not report success.
//
// No data-compatibility gate (metadata-hot-reload-safety.md §3.3) runs here, and that is a
// deliberate, narrow decision rather than an oversight: every write this reload is ever asked to
// pick up comes from internal/aiassist, whose own writer never emits anything but a new file or an
// appended list entry (internal/aiassist's own doc comment). That class of change is exactly the
// row §3.3/§7.2's own classification table already calls safe without checking stored data. A
// general "reload after any hand-edit" endpoint would need that gate built first; this one does
// not, because it cannot be asked to make the kind of change the gate exists to catch.
func (dh *dynamicHandler) Reload() error {
	dh.mu.Lock()
	defer dh.mu.Unlock()

	machines, machineList, workspaces, err := loadMetadataState(dh.cfg)
	if err != nil {
		return err
	}

	deps := web.Deps{
		Machines:           machines,
		MachineList:        machineList,
		Store:              dh.store,
		Files:              dh.files,
		Mailer:             dh.mailer,
		Cfg:                dh.cfg,
		Workspaces:         workspaces,
		DefaultWorkspaceID: dh.defaultWorkspaceID,
		AIClient:           dh.aiClient,
		ReloadMetadata:     dh.Reload,
	}
	handler := web.Routes(deps)
	dh.current.Store(&handler)
	return nil
}

// loadMetadataState is main()'s own original inline logic (unchanged), factored out so both the
// first build and every later Reload share one implementation. Machines stay process-wide, unioned
// across every installed Workspace, while Applications are per-Workspace -- see the long-standing
// comment this carries forward: that split is deliberate and still not the finished shape (a
// Workspace's *Machine set* is not yet per-Workspace, only which Applications it shows).
func loadMetadataState(cfg config.Config) (machines map[string]*domain.Machine, machineList []*domain.Machine, workspaces map[string]domain.Workspace, err error) {
	installed, err := metadata.LoadWorkspaces(cfg.MetadataPath)
	if err != nil {
		return nil, nil, nil, err
	}

	workspaces = make(map[string]domain.Workspace, len(installed))
	machines = map[string]*domain.Machine{}
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
	return machines, machineList, workspaces, nil
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
