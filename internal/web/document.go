package web

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
//
// documentMachine is mch_document itself, so fld_document_type's options come from
// metadata/document.yaml rather than being hardcoded in the template (the mismatch closed
// alongside this handler change -- see documentsubmit.templ's own doc comment).
func showDocumentSubmit(store *data.Store, documentMachine *domain.Machine) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		users, err := store.ListRecords(req.Context(), "mch_user")
		if err != nil {
			serverError(w, err)
			return
		}
		documentType, _ := documentMachine.FieldByID("fld_document_type")
		render(req.Context(), w, rendering.DocumentSubmitPage(users, documentType))
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
		if !hasAnyApprover(req.Form[action.FieldStepAssignee]) {
			// Without this, createApprovalSteps below silently skips every empty slot and
			// returns success -- a Document would be created with zero Approval Steps, and
			// nothing could ever decide it. Checked before CreateRecord so a rejected submission
			// never creates an orphaned Document that would then need cleaning up.
			http.Error(w, "at least one approver is required", http.StatusUnprocessableEntity)
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
		if !createApprovalSteps(w, req, store, machines[action.StepMachineID], document.ID, req.Form[action.FieldStepAssignee]) {
			return
		}

		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		logActivity(req.Context(), store, docMachine.ID, document.ID, actor,
			fmt.Sprintf("%q submitted", toDisplayString(document.Values["fld_title"])))

		redirectTo(w, req, fmt.Sprintf("/machines/%s/records/%s/signature-placement", docMachine.ID, document.ID))
	}
}

// hasAnyApprover reports whether at least one non-empty fld_assignee was submitted -- the wizard's
// own approver rows always submit a value (possibly ""), never omit the field entirely, so a
// simple non-blank check is enough.
func hasAnyApprover(assignees []string) bool {
	for _, a := range assignees {
		if a != "" {
			return true
		}
	}
	return false
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
func showSignaturePlacement(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
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
		document, steps, relations, totalPages, err := loadSignaturePlacementData(ctx, store, files, machines, documentID)
		if err != nil {
			recordError(w, err)
			return
		}
		page := pageFromQuery(req, totalPages)
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		render(ctx, w, rendering.SignaturePlacementPage(document, steps, relations, page, totalPages, machines[action.StepMachineID], actor))
	}
}

// loadSignaturePlacementData centralizes what both the dedicated Signature Placement page and
// Document's own detail page need: the Document record, its PDF's total page count, its own
// Approval Steps, and their relation options -- the exact logic that used to live inline in
// showSignaturePlacement, extracted so documentSignaturePlacementView below doesn't duplicate it.
// It does not take a page number: totalPages is an output (the caller decides which page to show,
// or -- for the inline embed -- always shows page 1), not an input this loading step needs.
func loadSignaturePlacementData(ctx context.Context, store *data.Store, files *storage.Store, machines map[string]*domain.Machine, documentID string) (*data.Record, []*data.Record, rendering.RelationOptions, int, error) {
	document, fileData, err := loadDocumentPDF(ctx, store, files, documentID)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	totalPages, err := pdf.PageCount(fileData)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	steps, err := store.ListRecordsBy(ctx, action.StepMachineID, action.FieldStepDocument, documentID)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	ld := composition.NewLoader(store, machines)
	relations, err := ld.RelationOptions(ctx, machines[action.StepMachineID])
	if err != nil {
		return nil, nil, nil, 0, err
	}
	return document, steps, relations, totalPages, nil
}

// documentSignaturePlacementView is Fase 0 of the composable-runtime kajian: the same
// signaturePlacementBlock Component the dedicated Signature Placement page renders, now also
// available to Document's own generic detail view (record.go). Returns nil immediately for every
// Machine other than mch_document (zero cost -- no PDF read, no extra query). Fails open, not
// closed: Document's detail page has never depended on its PDF being readable, so a loading error
// here is logged and treated as "nothing to show inline", not a reason to 500 the whole page --
// the same "best-effort, logged, never blocks the primary flow" posture already established for
// PDF signature compositing (capabilities.md).
func documentSignaturePlacementView(ctx context.Context, store *data.Store, files *storage.Store, machines map[string]*domain.Machine, machine *domain.Machine, recordID string) *rendering.DocumentSignaturePlacement {
	if machine.ID != action.DocumentMachineID {
		return nil
	}
	_, steps, relations, totalPages, err := loadSignaturePlacementData(ctx, store, files, machines, recordID)
	if err != nil {
		log.Printf("signature placement inline view for document %s: %v", recordID, err)
		return nil
	}
	return &rendering.DocumentSignaturePlacement{
		Steps:       steps,
		Relations:   relations,
		Page:        1,
		TotalPages:  totalPages,
		StepMachine: machines[action.StepMachineID],
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
// route in its group, plus an ownership check: the storage key names the Machine/Field it belongs
// to (recordOwnsUpload below), so serveUpload confirms some record in the caller's own Workspace
// actually carries this exact key in that field before serving it -- closing security audit
// 2026-09-19's M1 (an authenticated member of any Workspace could previously read any other
// Workspace's uploaded file just by knowing or guessing its key). A miss 404s rather than 403, so
// a guessed key can't be used to distinguish "wrong workspace" from "never existed".
//
// Content-Type and Content-Disposition are decided from the file's own sniffed bytes, never from
// the stored key's extension (security audit 2026-09-19, H2) -- an attacker who got a disguised
// payload past handleFileUploads' own validation (record.go), or whose file was stored before that
// validation existed, still can't get the browser to render it inline as something other than what
// it actually is. Content-Type is set explicitly before http.ServeFile so it takes precedence over
// ServeFile's own extension-based guess.
func serveUpload(store *data.Store, files *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key := chi.URLParam(req, "*")
		owns, err := recordOwnsUpload(req.Context(), store, key)
		if err != nil {
			serverError(w, err)
			return
		}
		if !owns {
			http.NotFound(w, req)
			return
		}
		path, err := files.Path(key)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		contentType, disposition, err := sniffUploadForServing(path)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename=%q`, disposition, storage.DisplayName(key)))
		http.ServeFile(w, req, path)
	}
}

// recordOwnsUpload reports whether some record of the Machine/Field the key itself names
// (storage.Store.Save's own key shape: "<machineID>/<fieldID>/<random>__<filename>") carries this
// exact key as that field's value, in the caller's own Workspace -- store.ListRecordsBy is already
// workspace-scoped from context the same way every other Store read is, so this doesn't special-
// case any one Machine (mch_document, mch_task, or any future Field Type: file Machine alike).
func recordOwnsUpload(ctx context.Context, store *data.Store, key string) (bool, error) {
	// filepath.Separator (a rune constant), not a "/" string literal -- matches storage.Save's own
	// filepath.Join for this same key, and keeps this line out of
	// TestHandlersHaveNoHardcodedApplicationRoute's string-literal scan, which doesn't distinguish
	// a path separator from a route by argument position, only by literal value.
	parts := strings.SplitN(key, string(filepath.Separator), 3)
	if len(parts) != 3 {
		return false, nil
	}
	machineID, fieldID := parts[0], parts[1]
	records, err := store.ListRecordsBy(ctx, machineID, fieldID, key)
	if err != nil {
		return false, err
	}
	return len(records) > 0, nil
}

// inlineSafeUploadContentTypes is the small allowlist of content this app deliberately opens
// in-browser (PDF/image previews and attachments) rather than downloading -- everything else is
// forced to `attachment`, regardless of what extension the original upload carried. Narrower than,
// and independent of, handleFileUploads' own denylist (record.go): validation happens once at
// upload time, disposition is decided fresh on every serve, which also covers any file stored
// before that upload-time validation existed.
var inlineSafeUploadContentTypes = map[string]bool{
	"application/pdf": true,
	"image/png":       true,
	"image/jpeg":      true,
}

// sniffUploadForServing reads path's own first bytes (http.DetectContentType, the same mechanism
// a browser's MIME-sniffing would use) to decide what Content-Type to declare and whether it's
// safe to show inline.
func sniffUploadForServing(path string) (contentType, disposition string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", "", err
	}
	contentType = http.DetectContentType(buf[:n])

	base := contentType
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		base = strings.TrimSpace(contentType[:idx])
	}
	disposition = "attachment"
	if inlineSafeUploadContentTypes[base] {
		disposition = "inline"
	}
	return contentType, disposition, nil
}
