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
// one Approval Step, seen by whoever has to decide it -- or, since 2026-09-24, one Document, seen
// by whoever opened it from a list of Documents. See reviewStep below for why both.
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
		actor := currentActor(req, store, cfg)
		step, ok := reviewStep(ctx, w, store, machines, machine, chi.URLParam(req, "id"), actor.ID)
		if !ok {
			return
		}

		// hasSignatureForGate is storage I/O the Composition plane must not perform (007 §20), so
		// it is answered here and passed in -- the same reason pdfPages below is.
		hasSignature, err := hasSignatureForGate(ctx, store, machine, actor.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		view, err := composition.ReviewDocument(ctx, composition.NewLoader(store, machines),
			machines[action.StepMachineID], machines[action.DocumentMachineID], step, actor,
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
		_, switchHref := viewerWorkspaceContext(ctx, store, actor.ID)
		render(ctx, w, rendering.ReviewDocumentPage(view, chrome.WorkspaceName, chrome.Viewer(), switchHref))
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

// reviewStep resolves the Approval Step this screen opens on, from either kind of id the route
// accepts, and writes the response itself when it cannot.
//
// **Two id kinds, one screen, and the second one is the point.** The Inbox lists *steps* -- each
// card is a decision the viewer owes -- so it links the step it already knows. My Documents lists
// *Documents*, and the viewer there is the submitter, who has no step of their own; until
// 2026-09-24 those cards went to the Document's generic record page instead, which is the page
// this repo had already ruled out for these two Machines ("POC scaffolding no real approver should
// land on", owner request 2026-09-19, recorded on rendering.detailBackLink). The screen itself was
// always the right destination: it draws the document, its PDF, every step's state and the SLA,
// and its Approve/Reject bar is gated on authorization.AllowsAction, so a submitter simply does
// not see one. What was missing was a way to reach it holding a Document.
//
// Which step a Document opens on is a rule, not a guess, and it lives in
// composition.ReviewStepForDocument beside the other approval rules rather than here.
//
// A Document with no steps 404s: the screen's whole subject is a step's decision, and there is
// none to show. Nothing in the app links there -- the card falls back to the generic page in that
// one case -- so this is the hand-typed-URL path.
func reviewStep(ctx context.Context, w http.ResponseWriter, store *data.Store, machines map[string]*domain.Machine, machine *domain.Machine, id, viewerID string) (*data.Record, bool) {
	record, err := store.GetRecord(ctx, machine.ID, id)
	if err != nil {
		recordError(w, err)
		return nil, false
	}
	if machine.ID == action.StepMachineID {
		return record, true
	}
	if machine.ID != action.DocumentMachineID {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}
	step, err := composition.ReviewStepForDocument(ctx, composition.NewLoader(store, machines), machines[action.StepMachineID], record, viewerID)
	if err != nil {
		serverError(w, err)
		return nil, false
	}
	if step == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}
	return step, true
}
