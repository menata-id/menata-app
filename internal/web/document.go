package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/pdf"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// showDocumentSubmit is Case 3's submission wizard (ROADMAP.md Phase 15 Step 6) -- Document
// details + Approval mode + a dynamic, flat approver picker, all on one screen.
func showDocumentSubmit(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		users, err := store.ListRecords(req.Context(), "mch_user")
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.DocumentSubmitPage(users, appName))
	}
}

// newApproverRow serves the wizard's own "+ Add approver" HTMX fragment -- a fresh
// rendering.ApproverRow, populated with the same real mch_user options as the wizard's initial
// row, never fabricated data.
func newApproverRow(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		users, err := store.ListRecords(req.Context(), "mch_user")
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.ApproverRow(users))
	}
}

// submitDocumentWizard is the wizard's own single POST: creates one mch_document and its own
// mch_approval_step records together. Hardcoded to mch_document/mch_approval_step, same posture
// as internal/action and every other Case 3-specific screen -- not a generic multi-Machine
// composite-create mechanism, since no second case needs one yet.
func submitDocumentWizard(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		docMachine := machines[action.DocumentMachineID]

		values, _, ok := submittedValues(w, req, docMachine, files)
		if !ok {
			return
		}
		data.ApplyDefaults(docMachine, values)
		// The wizard's own explicit rule outranks any declared default here, the same way an
		// INSERT's explicit column value outranks a SQL DEFAULT.
		values[action.FieldDocumentStatus] = action.DocumentStatusInReview
		if !validRecord(w, req, store, docMachine, values) {
			return
		}

		document, err := store.CreateRecord(req.Context(), docMachine.ID, values)
		if err != nil {
			serverError(w, err)
			return
		}
		if !createApprovalSteps(w, req, store, machines[action.StepMachineID], document.ID, req.Form["fld_assignee"]) {
			return
		}

		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		logActivity(req.Context(), store, docMachine.ID, document.ID, actor,
			fmt.Sprintf("%q submitted", toDisplayString(document.Values["fld_title"])))

		redirectTo(w, req, fmt.Sprintf("/machines/%s/records/%s/signature-placement", docMachine.ID, document.ID))
	}
}

// createApprovalSteps writes one pending Approval Step per named approver.
//
// Sequence comes from the submitted order rather than a hidden input: a browser submits repeated
// field names in DOM order, which is exactly the order the wizard's own reordering leaves the
// <select>s in.
//
// It is the position in the submitted list, so an empty slot leaves a gap -- ["", "usr_a"] makes
// usr_a step 2, not step 1. That is the existing behaviour and it is harmless, because
// action.CanDecide compares sequences relatively rather than expecting 1..n. Left as it was:
// Phase 19 moves code, it does not quietly renumber approvals.
func createApprovalSteps(w http.ResponseWriter, req *http.Request, store *data.Store, stepMachine *domain.Machine, documentID string, assignees []string) bool {
	for i, assignee := range assignees {
		if assignee == "" {
			continue
		}
		values := map[string]any{
			action.FieldStepDocument: documentID,
			action.FieldStepSequence: float64(i + 1),
			action.FieldStepAssignee: assignee,
			action.FieldStepDecision: action.DecisionPending,
		}
		data.ApplyDefaults(stepMachine, values)
		if !validRecord(w, req, store, stepMachine, values) {
			return false
		}
		if _, err := store.CreateRecord(req.Context(), stepMachine.ID, values); err != nil {
			serverError(w, err)
			return false
		}
	}
	return true
}

// showSignaturePlacement is Case 3's signature-coordinate placement screen (ROADMAP.md Phase 15
// Step 4) -- a real rendered page of the Document's own PDF (Step 3's internal/pdf), one
// draggable marker per Approval Step. Hardcoded to mch_document, same posture as decideStep: this
// is Case 3's own screen, not a generic per-Machine feature.
func showSignaturePlacement(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if machine.ID != action.DocumentMachineID {
			http.Error(w, "signature placement only applies to a Document", http.StatusNotFound)
			return
		}
		ctx := req.Context()
		documentID := chi.URLParam(req, "id")
		document, fileData, err := loadDocumentPDF(ctx, store, files, documentID)
		if err != nil {
			recordError(w, err)
			return
		}
		totalPages, err := pdf.PageCount(fileData)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to read document PDF: %v", err), http.StatusUnprocessableEntity)
			return
		}
		page := pageFromQuery(req, totalPages)

		steps, err := store.ListRecordsBy(ctx, action.StepMachineID, action.FieldStepDocument, documentID)
		if err != nil {
			serverError(w, err)
			return
		}
		ld := composition.NewLoader(store, machines)
		relations, err := ld.RelationOptions(ctx, machines[action.StepMachineID])
		if err != nil {
			serverError(w, err)
			return
		}

		render(ctx, w, rendering.SignaturePlacementPage(document, steps, relations, page, totalPages, appName))
	}
}

// servePDFPreview rasterizes one page of a Document's own PDF to PNG (Step 3's internal/pdf),
// for showSignaturePlacement's own <img> -- not exposed for any other Machine or file field, same
// hardcoded scope as the rest of Case 3's Action/screen code.
func servePDFPreview(machines map[string]*domain.Machine, store *data.Store, files *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if machine.ID != action.DocumentMachineID {
			http.Error(w, "PDF preview only applies to a Document", http.StatusNotFound)
			return
		}
		_, fileData, err := loadDocumentPDF(req.Context(), store, files, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		totalPages, err := pdf.PageCount(fileData)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to read document PDF: %v", err), http.StatusUnprocessableEntity)
			return
		}
		page := pageFromQuery(req, totalPages)

		png, err := pdf.RenderPagePNG(fileData, page-1, 900, 1200)
		if err != nil {
			http.Error(w, fmt.Sprintf("unable to render page: %v", err), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(png); err != nil {
			log.Printf("write PNG preview failed: %v", err)
		}
	}
}

// loadDocumentPDF fetches document's own fld_file upload and reads it off local disk, for both
// showSignaturePlacement and servePDFPreview -- the one place either handler needs the raw PDF
// bytes.
func loadDocumentPDF(ctx context.Context, store *data.Store, files *storage.Store, documentID string) (*data.Record, []byte, error) {
	document, err := store.GetRecord(ctx, action.DocumentMachineID, documentID)
	if err != nil {
		return nil, nil, err
	}
	key := toDisplayString(document.Values[action.FieldDocumentFile])
	if key == "" {
		return document, nil, fmt.Errorf("document has no file uploaded")
	}
	path, err := files.Path(key)
	if err != nil {
		return document, nil, err
	}
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return document, nil, err
	}
	return document, fileBytes, nil
}

// serveUpload streams a previously uploaded file back. Gated by requireAuth like every other
// route in its group -- there is no per-record ownership check yet (ROADMAP.md Phase 2's
// Machine+Action permission granularity doesn't extend to individual files), matching the rest
// of the app's current authorization boundary.
func serveUpload(files *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key := chi.URLParam(req, "*")
		path, err := files.Path(key)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename=%q`, storage.DisplayName(key)))
		http.ServeFile(w, req, path)
	}
}
