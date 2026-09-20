package web

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/action"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/pdf"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// showReviewDocument serves board 10 (ui-sample/case-03-flow1/10-review-document.html, Fase 6b):
// one Approval Step, seen by whoever has to decide it.
//
// Registered under /machines/{machineID}/records/{id}/review alongside the two sibling routes that
// already use this shape (signature-placement, pdf-preview) and 404s for any other Machine,
// exactly as those two do. That prefix is the one internal/conformance's own route gate exempts by
// documented rule ("a generic Machine's own page is always reachable at /machines/{id} even with
// no navigation entry at all"), so this screen needs no navigation item and adds nothing to
// undeclaredScreenRatchet -- which has been empty since Fase 6a and should stay that way. A
// per-record screen is not a menu destination: there is no fixed route string to declare.
//
// The handler is thin on purpose. Every derivation lives in composition.ReviewDocument, because
// the .templ it renders may contain no field reads at all -- see rendering.ReviewView's own doc
// comment for why that constraint exists and what it bought.
func showReviewDocument(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if machine.ID != action.StepMachineID {
			http.NotFound(w, req)
			return
		}
		step, err := store.GetRecord(ctx, machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)

		// hasSignatureForGate is storage I/O the Composition plane must not perform (007 §20), so
		// it is answered here and passed in -- the same reason pdfPages below is.
		hasSignature, err := hasSignatureForGate(ctx, store, machine, actor.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		view, err := composition.ReviewDocument(ctx, composition.NewLoader(store, machines),
			machine, machines[action.DocumentMachineID], step, actor,
			documentPageCount(ctx, store, files, step), hasSignature, time.Now())
		if err != nil {
			serverError(w, err)
			return
		}

		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.ReviewDocumentPage(view, chrome.WorkspaceName, chrome.UserInitials, workspaceRoleOf(ctx, store, actor.ID)))
	}
}

// documentPageCount is how many pages this step's own Document has, or 0 when that cannot be
// answered.
//
// Fails open, the same posture documentSignaturePlacementView already takes for the same file: a
// Document with no PDF attached, or one whose bytes are unreadable, is a page that renders without
// a page count -- never a 500. Reviewing a document must not depend on rasterizing it.
func documentPageCount(ctx context.Context, store *data.Store, files *storage.Store, step *data.Record) int {
	documentID := composition.DisplayString(step.Values[action.FieldStepDocument])
	if documentID == "" {
		return 0
	}
	_, fileBytes, err := loadDocumentPDF(ctx, store, files, documentID)
	if err != nil {
		log.Printf("review page count for document %s: %v", documentID, err)
		return 0
	}
	pages, err := pdf.PageCount(fileBytes)
	if err != nil {
		log.Printf("review page count for document %s: %v", documentID, err)
		return 0
	}
	return pages
}

// workspaceRoleOf is the viewer's own Workspace role, or "" when they hold no membership row --
// the degradation every other caller of this question already makes. It feeds appShell's launcher
// filtering through rendering.membersHiddenFor, so a plain member is not offered the two
// admin-gated destinations.
func workspaceRoleOf(ctx context.Context, store *data.Store, userID string) string {
	if userID == "" {
		return ""
	}
	workspaceID, _ := data.WorkspaceScope(ctx)
	m, err := store.GetMembership(ctx, workspaceID, userID)
	if err != nil {
		return ""
	}
	return m.WorkspaceRole
}
