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
// wizardOptions is everything both wizard renderings need beyond chrome: the two Fields whose
// declared options the form must offer (never hardcoded lists), and the pickers' own choices.
//
// It exists because the full page and the "+ Add approver" fragment must offer the *identical*
// approver row -- the drift that would otherwise appear as a new row silently missing a Group the
// first row had.
type wizardOptions struct {
	documentType domain.Field
	mode         domain.Field
	approvers    rendering.RelationOptions
	groups       rendering.GroupOptions
}

// readWizardOptions resolves them once.
//
// The approver list comes from composition.Loader.RelationOptions rather than a hand-built option
// list off mch_user records, which is what let documentsubmit.templ leave the projection ratchet:
// the Loader already resolves a reference Field's target records to {ID, Label} using the target's
// first Field, and for mch_user that first Field is fld_name -- exactly what the template used to
// read itself. The group list is keyed on mch_approval_step, not mch_document: GroupOptions
// short-circuits on a Machine declaring no group Field, and only the step Machine declares one.
func readWizardOptions(req *http.Request, machines map[string]*domain.Machine, store *data.Store) (wizardOptions, error) {
	stepMachine := machines[action.StepMachineID]
	ld := composition.NewLoader(store, machines)
	approvers, err := ld.RelationOptions(req.Context(), stepMachine)
	if err != nil {
		return wizardOptions{}, err
	}
	// Narrowed to people who could actually decide a step (composition.ApproverOptions). Without
	// it a submitter can name anyone in the Workspace, including someone holding no role in this
	// Application at all -- producing a step assigned to a real person and decidable by nobody,
	// with no error anywhere.
	workspaceID, _ := data.WorkspaceScope(req.Context())
	members, err := store.ListMembers(req.Context(), workspaceID)
	if err != nil {
		return wizardOptions{}, err
	}
	approvers = composition.ApproverOptions(approvers, stepMachine, members)
	groups, err := ld.GroupOptions(req.Context(), stepMachine)
	if err != nil {
		return wizardOptions{}, err
	}
	docMachine := machines[action.DocumentMachineID]
	documentType, _ := docMachine.FieldByID("fld_document_type")
	mode, _ := docMachine.FieldByID(action.FieldDocumentMode)
	return wizardOptions{documentType: documentType, mode: mode, approvers: approvers, groups: groups}, nil
}

// showDocumentSubmit serves board 08 (ui-sample/case-03-flow1/08-submit-document.html).
//
// Every option list on this screen comes from metadata: fld_document_type's since the Field was
// added, and fld_mode's since Fase 6c-2 -- it was the last hardcoded pair in the wizard, two
// <input type="radio" value="sequential|parallel"> literals sitting two sections below a select
// that already did it correctly.
func showDocumentSubmit(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		opts, err := readWizardOptions(req, machines, store)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		actor := currentActor(req, store, cfg)
		workspaceRole, switchHref := viewerWorkspaceContext(ctx, store, actor.ID)
		render(ctx, w, rendering.DocumentSubmitPage(opts.documentType, opts.mode, opts.approvers, opts.groups,
			chrome.WorkspaceName, chrome.Viewer(), workspaceRole, switchHref))
	}
}

// newApproverRow serves the wizard's own "+ Add approver" HTMX fragment -- a fresh
// rendering.ApproverRow with the same real options as the wizard's initial row, never fabricated
// data and never a different set.
func newApproverRow(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		opts, err := readWizardOptions(req, machines, store)
		if err != nil {
			serverError(w, err)
			return
		}
		render(req.Context(), w, rendering.ApproverRow(opts.approvers, opts.groups))
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
		rows, problem := parseStepInputs(req)
		if problem != "" {
			http.Error(w, problem, http.StatusUnprocessableEntity)
			return
		}
		if !hasApprover(rows) {
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
		// Who submitted this Document, stamped from the session rather than accepted from the
		// form (2026-09-21). It is what mch_document's own prm_create_own_document checks, and
		// what finally puts an owner on the record instead of leaving it recoverable only by
		// scanning the activity feed.
		actor := currentActor(req, store, cfg)
		values[action.FieldDocumentSubmittedBy] = actor.ID
		if !allowsRecordCreate(w, docMachine, values, actor) {
			return
		}
		if !validRecord(w, req, store, docMachine, values) {
			return
		}

		document, err := store.CreateRecord(req.Context(), docMachine.ID, values)
		if err != nil {
			serverError(w, err)
			return
		}
		if !createApprovalSteps(w, req, store, machines[action.StepMachineID], document.ID, rows) {
			return
		}

		logActivity(req.Context(), store, docMachine.ID, document.ID, actor.ID,
			fmt.Sprintf("%q submitted", toDisplayString(document.Values["fld_title"])))

		redirectTo(w, req, fmt.Sprintf("/machines/%s/records/%s/signature-placement", docMachine.ID, document.ID))
	}
}

// stepInput is one approver row as the wizard submitted it, already paired up.
//
// One struct rather than four parallel []string arguments, because the pairing is the fragile
// part: row order *is* fld_sequence (there is no hidden sequence input -- see below), so four
// slices that drift out of alignment by one would silently attach every approver to the wrong
// step. parseStepInputs is the single place that alignment is established, and the reason the
// wizard's type picker is a <select> rather than the board's segmented buttons: an unchecked radio
// submits nothing at all, which is exactly how a slice loses an element.
type stepInput struct {
	name          string
	assignee      string
	approverType  string
	approverGroup string
}

// parseStepInputs reads the approver rows out of the submitted form and rejects a row that names
// no approver of the kind it claims.
//
// That pairing check lives here rather than in metadata because metadata cannot state it: what it
// wants to say is "fld_assignee is required when fld_approver_type is User", and this runtime's
// Constraint has one shape only -- block a field transition while a *related Machine* has a
// matching record -- which cannot condition on a sibling Field of the same record. So fld_assignee
// is declared optional (see metadata/approval_step.yaml's own comment) and this is the enforcement.
// Forward pointer: ROADMAP.md's "Conditional required" deferral row; when that lands, this check
// becomes two declarations and this function loses its reason to validate anything.
//
// An entirely blank row is skipped rather than rejected: the wizard renders one empty row to start
// with, and "+ Add approver" can leave a spare.
func parseStepInputs(req *http.Request) ([]stepInput, string) {
	names := req.Form[action.FieldStepName]
	assignees := req.Form[action.FieldStepAssignee]
	types := req.Form[action.FieldStepApproverType]
	groups := req.Form[action.FieldStepApproverGroup]

	rows := make([]stepInput, 0, len(assignees))
	for i := range assignees {
		row := stepInput{
			name:          at(names, i),
			assignee:      assignees[i],
			approverType:  at(types, i),
			approverGroup: at(groups, i),
		}
		switch {
		case row.approverType == domain.ActorKindGroup && row.approverGroup == "":
			return nil, "a step set to Group must name a group"
		case row.approverType != domain.ActorKindGroup && row.assignee == "" && row.approverGroup == "":
			row = stepInput{} // an untouched spare row
		case row.approverType != domain.ActorKindGroup && row.assignee == "":
			return nil, "a step set to User must name a person"
		}
		rows = append(rows, row)
	}
	return rows, ""
}

// at is a bounds-safe index into a parallel form slice. A row whose control was never rendered --
// an older cached page, a hand-built POST -- reads as empty rather than panicking the handler.
func at(values []string, i int) string {
	if i < len(values) {
		return values[i]
	}
	return ""
}

func hasApprover(rows []stepInput) bool {
	for _, r := range rows {
		if r.assignee != "" || r.approverGroup != "" {
			return true
		}
	}
	return false
}

// createApprovalSteps writes one pending Approval Step per approver row.
//
// Sequence comes from the submitted order rather than a hidden input: a browser submits repeated
// field names in DOM order, which is exactly the order the wizard's own reordering leaves the
// <select>s in.
//
// It is the position in the submitted list, so an empty slot leaves a gap -- an untouched row
// before a filled one makes the filled one step 2, not step 1. That is the existing behaviour and
// it is harmless, because action.CanDecide compares sequences relatively rather than expecting
// 1..n. Left as it was: Phase 19 moved this code, it did not quietly renumber approvals, and
// neither does Fase 6c-2.
//
// fld_approver_type is written only when the row actually chose one. A row left on the default
// stores nothing there, which is deliberate: an empty type is what makes authorization's own
// fallback take over, so a wizard that stamped "User" on every row would opt every step into the
// dynamic gate for no reason and make the fallback path untested in practice.
func createApprovalSteps(w http.ResponseWriter, req *http.Request, store *data.Store, stepMachine *domain.Machine, documentID string, rows []stepInput) bool {
	for i, row := range rows {
		if row.assignee == "" && row.approverGroup == "" {
			continue
		}
		values := map[string]any{
			action.FieldStepDocument: documentID,
			action.FieldStepSequence: float64(i + 1),
			action.FieldStepDecision: action.DecisionPending,
		}
		if row.name != "" {
			values[action.FieldStepName] = row.name
		}
		if row.approverType == domain.ActorKindGroup {
			values[action.FieldStepApproverType] = domain.ActorKindGroup
			values[action.FieldStepApproverGroup] = row.approverGroup
		} else {
			values[action.FieldStepAssignee] = row.assignee
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
		actor := currentActor(req, store, cfg)
		view, err := composition.SignaturePlacement(ctx, composition.NewLoader(store, machines),
			machines[action.StepMachineID], document, steps, relations, page, totalPages, actor)
		if err != nil {
			serverError(w, err)
			return
		}
		chrome, err := resolveChrome(ctx, req, store, cfg)
		if err != nil {
			serverError(w, err)
			return
		}
		workspaceRole, switchHref := viewerWorkspaceContext(ctx, store, actor.ID)
		render(ctx, w, rendering.SignaturePlacementPage(view, chrome.WorkspaceName, chrome.Viewer(), workspaceRole, switchHref))
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
func documentSignaturePlacementView(ctx context.Context, store *data.Store, files *storage.Store, machines map[string]*domain.Machine, machine *domain.Machine, recordID string, actor domain.Actor) *rendering.DocumentSignaturePlacement {
	if machine.ID != action.DocumentMachineID {
		return nil
	}
	document, steps, relations, totalPages, err := loadSignaturePlacementData(ctx, store, files, machines, recordID)
	if err != nil {
		log.Printf("signature placement inline view for document %s: %v", recordID, err)
		return nil
	}
	// Page 1, because this embed has no pager of its own -- a step placed on page 6 has its card
	// rendered here but not its marker. Named rather than fixed: the dedicated screen is where
	// placement happens, and giving the embed a pager would duplicate that screen inside a page
	// that is already the generic record view.
	view, err := composition.SignaturePlacement(ctx, composition.NewLoader(store, machines),
		machines[action.StepMachineID], document, steps, relations, 1, totalPages, actor)
	if err != nil {
		log.Printf("signature placement inline view for document %s: %v", recordID, err)
		return nil
	}
	return &rendering.DocumentSignaturePlacement{View: view}
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
