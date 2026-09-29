package web

import (
	"net/http"

	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/rendering"
)

// showInference renders what the runtime inferred for this Workspace -- the diagnostics surface 001
// Principle #6's second clause requires ("**Inference must be inspectable.** … the runtime should be
// able to expose the resolved result through diagnostics or equivalent tooling"), restated by 004
// §Inference and 005 Phase 4.
//
// Workspace-admin only (registered in router.go's ar group), the same gate and the same reason
// /install-application carries: it describes how this Workspace is put together.
//
// It issues **no query of its own**: composition.Inference is pure over the Workspace already resolved
// onto ctx. That is the answer to 007 §33's admission questions for this capability -- no logical
// dependency added, no DAG node generated, no physical fan-out -- and it is why the route satisfies the
// GET sweep's two invariants without needing care.
func showInference(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		_, switchHref := viewerWorkspaceContext(ctx, store, currentActor(req, store, cfg).ID)
		render(ctx, w, rendering.InferencePage(
			composition.Inference(rendering.CurrentWorkspace(ctx)),
			chrome.WorkspaceName, chrome.Viewer(), switchHref))
	}
}
