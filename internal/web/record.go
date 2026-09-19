package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/behavior"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

func createRecordForm(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if err := parseRecordForm(req); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}

		values := data.ValuesFromForm(machine, req.Form)
		uploaded, err := handleFileUploads(req, machine, files)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for k, v := range uploaded {
			values[k] = v
		}
		data.ApplyDefaults(machine, values)

		if !validRecord(w, req, store, machine, values) {
			return
		}
		record, err := store.CreateRecord(req.Context(), machine.ID, values)
		if err != nil {
			serverError(w, err)
			return
		}
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		logRecordCreated(req.Context(), store, machine, record, actor)

		renderMachineBody(w, req, machines, machine, store, actor)
	}
}

// showRecordRow serves three different renderings of the same Record from one route, depending
// on who's asking (ROADMAP.md Phase 8): a direct browser navigation gets the full detail page;
// an HTMX request targeting the detail page's own container gets just that container's view
// fragment (used by the detail page's own Cancel-from-edit); any other HTMX request (a table row
// or board card's Cancel) gets the original RecordRow fragment, unchanged from Phase 1.
func showRecordRow(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, appName string, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		record, err := store.GetRecord(req.Context(), machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		ld := composition.NewLoader(store, machines)
		relations, err := ld.RelationOptions(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		if req.Header.Get("HX-Request") != "true" {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			hasSignature, err := hasSignatureForGate(req.Context(), store, machine, actor)
			if err != nil {
				serverError(w, err)
				return
			}
			sigPlacement := documentSignaturePlacementView(req.Context(), store, files, machines, machine, record.ID)
			render(req.Context(), w, rendering.RecordDetailPage(machine, record, appName, relations, children, actor, hasSignature, sigPlacement))
			return
		}
		if isDetailContext(req) {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			hasSignature, err := hasSignatureForGate(req.Context(), store, machine, actor)
			if err != nil {
				serverError(w, err)
				return
			}
			sigPlacement := documentSignaturePlacementView(req.Context(), store, files, machines, machine, record.ID)
			render(req.Context(), w, rendering.RecordDetailView(machine, record, relations, children, actor, hasSignature, sigPlacement))
			return
		}
		render(req.Context(), w, rendering.RecordRow(machine, record, relations, actor))
	}
}

func editRecordRow(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		id := chi.URLParam(req, "id")
		record, err := store.GetRecord(req.Context(), machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		if !authorization.AllowsAction(machine, domain.ActionEdit, record.Values, actor) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		ld := composition.NewLoader(store, machines)
		relations, err := ld.RelationOptions(req.Context(), machine)
		if err != nil {
			serverError(w, err)
			return
		}
		if isDetailContext(req) {
			render(req.Context(), w, rendering.RecordDetailEdit(machine, record, relations))
			return
		}
		render(req.Context(), w, rendering.RecordEditRow(machine, record, relations))
	}
}

// updateRecordForm applies a form edit to one record: collect the submitted values, run every
// guard the write has to pass, write, log, and re-render.
//
// The guards are named helpers below rather than inline blocks because each is a rule in its own
// right -- one about file inputs, one about who may change a decision, one about Constraints --
// and reading this handler should show the order they run in, which is itself the contract
// (005-runtime-lifecycle.md "Security Ordering").
func updateRecordForm(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		id := chi.URLParam(req, "id")
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		if !allowsRecordEdit(w, req, store, machine, id, actor) {
			return
		}

		values, uploaded, ok := submittedValues(w, req, machine, files)
		if !ok {
			return
		}
		if !carryForwardFiles(w, req, store, machine, id, uploaded, values) {
			return
		}
		if !allowsDecisionChange(w, req, store, machine, id, values) {
			return
		}
		if !passesWriteGuards(w, req, store, machines, machine, id, values) {
			return
		}

		// Case 19's Project Activity feed wants Task status moves as their own event (ROADMAP.md
		// Phase 14) -- the old status has to be read before the write replaces it.
		oldTaskStatus := currentTaskStatus(req, store, machine, id)

		record, err := store.UpdateRecord(req.Context(), machine.ID, id, values)
		if err != nil {
			recordError(w, err)
			return
		}
		logTaskStatusMove(req, store, machine, record, actor, oldTaskStatus)

		renderRecord(w, req, machines, store, files, machine, record, actor)
	}
}

// allowsRecordEdit enforces any declared domain.ActionEdit Permission before the generic update
// route writes anything, generalizing decideStep's own authorization.AllowsAction check
// (internal/web/approval.go) from Approve/Reject to every Machine -- the same "generalize on a
// second real case, never the first" discipline deleteAllowed below already follows. A Machine
// declaring no edit Permission stays unrestricted (001-design-principles.md Principle #6:
// metadata describes exceptions, not defaults) and skips the extra fetch entirely.
func allowsRecordEdit(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id, actor string) bool {
	if len(machine.PermissionsFor(domain.ActionEdit)) == 0 {
		return true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return false
	}
	if !authorization.AllowsAction(machine, domain.ActionEdit, existing.Values, actor) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return false
	}
	return true
}

// submittedValues reads the form body and merges in any freshly uploaded files. It returns the
// uploaded set separately as well, because "which file fields did this request actually set" is
// what carryForwardFiles needs and cannot recover from values alone.
func submittedValues(w http.ResponseWriter, req *http.Request, machine *domain.Machine, files *storage.Store) (values, uploaded map[string]any, ok bool) {
	if err := parseRecordForm(req); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return nil, nil, false
	}
	values = data.ValuesFromForm(machine, req.Form)
	uploaded, err := handleFileUploads(req, machine, files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, nil, false
	}
	for k, v := range uploaded {
		values[k] = v
	}
	return values, uploaded, true
}

// carryForwardFiles keeps a file field that this request did not re-upload. A browser can't
// pre-fill <input type="file">, so "no new upload" must not be read as "clear the file" the way
// an empty text input would be (ROADMAP.md Phase 11).
func carryForwardFiles(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, uploaded, values map[string]any) bool {
	if err := carryForwardExistingFiles(req.Context(), store, machine, id, uploaded, values); err != nil {
		recordError(w, err)
		return false
	}
	return true
}

// allowsDecisionChange refuses an attempt to move an Approval Step's fld_decision through the
// generic edit route. That transition only ever happens through POST .../decide, which enforces
// action.CanDecide's sequencing rule; the edit route must not become a bypass for it
// (ROADMAP.md Phase 12).
func allowsDecisionChange(w http.ResponseWriter, req *http.Request, store *data.Store, machine *domain.Machine, id string, values map[string]any) bool {
	if machine.ID != action.StepMachineID {
		return true
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		recordError(w, err)
		return false
	}
	if newDecision, ok := values[action.FieldStepDecision]; ok && fmt.Sprint(newDecision) != fmt.Sprint(existing.Values[action.FieldStepDecision]) {
		http.Error(w, "use Approve/Reject to change a decision, not a direct edit", http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// passesWriteGuards runs the three checks every write has to clear: the record's own shape, the
// existence of anything it points at, and the Machine's declared Constraints.
func passesWriteGuards(w http.ResponseWriter, req *http.Request, store *data.Store, machines map[string]*domain.Machine, machine *domain.Machine, id string, values map[string]any) bool {
	if !validRecord(w, req, store, machine, values) {
		return false
	}
	relatedRecords, err := composition.NewLoader(store, machines).ConstraintRelatedRecords(req.Context(), machine)
	if err != nil {
		serverError(w, err)
		return false
	}
	if err := behavior.CheckConstraints(machine, id, values, relatedRecords); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// currentTaskStatus reads a Task's status before the write. A read failure yields "", which
// simply means the status-move event is not logged -- never a reason to fail the edit itself.
func currentTaskStatus(req *http.Request, store *data.Store, machine *domain.Machine, id string) string {
	if machine.ID != taskMachineID {
		return ""
	}
	existing, err := store.GetRecord(req.Context(), machine.ID, id)
	if err != nil {
		return ""
	}
	return toDisplayString(existing.Values["fld_status"])
}

// logTaskStatusMove appends the Project Activity event for a Task that changed status.
func logTaskStatusMove(req *http.Request, store *data.Store, machine *domain.Machine, record *data.Record, actor, oldStatus string) {
	if machine.ID != taskMachineID {
		return
	}
	newStatus := toDisplayString(record.Values["fld_status"])
	if newStatus == "" || newStatus == oldStatus {
		return
	}
	title := recordLabel(machine, record)
	summary := fmt.Sprintf("%q moved from %s to %s", title, oldStatus, newStatus)
	if newStatus == "done" {
		summary = fmt.Sprintf("%q completed", title)
	}
	logActivity(req.Context(), store, machine.ID, record.ID, actor, summary)
}

// renderRecord re-renders one record after a write, as the detail view or as a table/board row
// depending on what the HTMX request targeted.
func renderRecord(w http.ResponseWriter, req *http.Request, machines map[string]*domain.Machine, store *data.Store, files *storage.Store, machine *domain.Machine, record *data.Record, actor string) {
	ld := composition.NewLoader(store, machines)
	relations, err := ld.RelationOptions(req.Context(), machine)
	if err != nil {
		serverError(w, err)
		return
	}
	if isDetailContext(req) {
		children, err := ld.ChildSections(req.Context(), machine, record.ID)
		if err != nil {
			serverError(w, err)
			return
		}
		hasSignature, err := hasSignatureForGate(req.Context(), store, machine, actor)
		if err != nil {
			serverError(w, err)
			return
		}
		sigPlacement := documentSignaturePlacementView(req.Context(), store, files, machines, machine, record.ID)
		render(req.Context(), w, rendering.RecordDetailView(machine, record, relations, children, actor, hasSignature, sigPlacement))
		return
	}
	render(req.Context(), w, rendering.RecordRow(machine, record, relations, actor))
}

func deleteRecord(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		id := chi.URLParam(req, "id")
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		if allowed, status, reason, err := deleteAllowed(req.Context(), store, machine, id, actor); err != nil {
			serverError(w, err)
			return
		} else if !allowed {
			http.Error(w, reason, status)
			return
		}
		if err := store.DeleteRecord(req.Context(), machine.ID, id); err != nil {
			serverError(w, err)
			return
		}
		if isDetailContext(req) {
			// The record is gone; nothing left on this page to show. Send the visitor back to
			// the Machine's own list/board.
			w.Header().Set("HX-Redirect", "/machines/"+machine.ID)
			return
		}
		// Empty response: HTMX swaps the row's outerHTML with nothing, removing it.
	}
}

// deleteAllowed guards the generic delete route two ways, business-state and identity, evaluated
// independently: a hardcoded, Case-3-scoped business-state check for the two Machines whose
// records are an audit trail Phase 16/17 exist to protect
// (action.CanDeleteApprovalStep/CanDeleteDocument, same narrow posture as
// action.CanDecide/CompositeSignatures, not a generic Machine-level permission engine on its own),
// ANDed with any declared domain.ActionDelete Permission (authorization.AllowsAction,
// generalizing decideStep's identity check the same way allowsRecordEdit above does for edits).
// A Machine with neither -- the common case -- stays fully unrestricted, same as the generic
// route always was (ROADMAP.md's own Method: generalize on a second real case, never the first).
func deleteAllowed(ctx context.Context, store *data.Store, machine *domain.Machine, id, actor string) (ok bool, status int, reason string, err error) {
	var existing *data.Record
	switch machine.ID {
	case action.StepMachineID:
		step, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			return false, 0, "", err
		}
		existing = step
		if ok, reason := action.CanDeleteApprovalStep(step.Values); !ok {
			return false, http.StatusUnprocessableEntity, reason, nil
		}
	case action.DocumentMachineID:
		doc, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			return false, 0, "", err
		}
		existing = doc
		steps, err := store.ListRecordsBy(ctx, action.StepMachineID, action.FieldStepDocument, id)
		if err != nil {
			return false, 0, "", err
		}
		status, _ := doc.Values[action.FieldDocumentStatus].(string)
		if ok, reason := action.CanDeleteDocument(status, steps); !ok {
			return false, http.StatusUnprocessableEntity, reason, nil
		}
	}
	if len(machine.PermissionsFor(domain.ActionDelete)) == 0 {
		return true, 0, "", nil
	}
	if existing == nil {
		existing, err = store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			return false, 0, "", err
		}
	}
	if !authorization.AllowsAction(machine, domain.ActionDelete, existing.Values, actor) {
		return false, http.StatusForbidden, "not allowed to delete this record", nil
	}
	return true, 0, "", nil
}

// parseRecordForm parses a create/update request body that may be multipart/form-data (needed
// for a FieldTypeFile upload -- every form sets hx-encoding for this, ROADMAP.md Phase 11) or a
// plain url-encoded body (any other client, e.g. a direct API caller). ParseMultipartForm always
// runs ParseForm first regardless of content type, so http.ErrNotMultipart here just means "no
// file part was present, req.Form is already populated correctly" -- not a real failure.
func parseRecordForm(req *http.Request) error {
	err := req.ParseMultipartForm(maxUploadBytes)
	if err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return err
	}
	return nil
}

// handleFileUploads saves any file actually submitted for one of machine's FieldTypeFile fields,
// returning fieldID -> storage key for just those fields. A field with no file in this request
// (http.ErrMissingFile) is simply absent from the result -- not an error, since a file input left
// untouched on an edit form submits nothing. Same for http.ErrNotMultipart: a genuinely
// non-multipart body (a direct API caller posting url-encoded, per parseRecordForm's own promise
// above) by definition submitted no file for *any* field, not just the one FormFile call happened
// to hit first -- previously only ErrMissingFile was tolerated here, so this exact case still 400'd
// despite parseRecordForm's comment claiming it worked (ROADMAP.md Operational backlog, pre-existing
// since Phase 19).
func handleFileUploads(req *http.Request, machine *domain.Machine, files *storage.Store) (map[string]any, error) {
	uploaded := map[string]any{}
	for _, f := range machine.Fields {
		if f.Type != domain.FieldTypeFile {
			continue
		}
		file, header, err := req.FormFile(f.ID)
		if err != nil {
			if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
				continue
			}
			return nil, fmt.Errorf("read upload for %s: %w", f.ID, err)
		}
		content, sniffErr := rejectDangerousUpload(file)
		if sniffErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("upload for %s: %w", f.ID, sniffErr)
		}
		key, saveErr := files.Save(machine.ID, f.ID, header.Filename, content)
		_ = file.Close() // read handle on the uploaded part; nothing to act on if this fails
		if saveErr != nil {
			return nil, saveErr
		}
		uploaded[f.ID] = key
	}
	return uploaded, nil
}

// dangerousUploadContentTypes denylists content a browser will execute as active content if it's
// ever served back, rather than allowlisting one "correct" type -- FieldTypeFile is a generic,
// composable Field Type already used for genuinely different content (mch_document.fld_file's
// PDFs, mch_task.fld_attachment's free-form attachments, capabilities.md's own Field Types table),
// so an allowlist would need updating every time a new attachment use case appears; a denylist of
// "things a browser executes" doesn't (security audit 2026-09-19, H2).
var dangerousUploadContentTypes = []string{
	"text/html",
	"image/svg+xml",
	"text/xml",
	"application/xml",
	"text/javascript",
	"application/javascript",
	"application/ecmascript",
}

// rejectDangerousUpload sniffs the real content of an upload (http.DetectContentType, the same
// mechanism a browser's own MIME-sniffing would use) rather than trusting the client-declared
// filename/extension, and returns an error for anything on the execute-as-active-content
// denylist -- this is what actually stops an "evil.svg"/"evil.html" payload at upload time,
// independent of the nosniff header (secureheaders.go) and serveUpload's own disposition fix,
// which are this bug's other two layers. It returns a reader that still yields the upload's full
// bytes (the sniffed prefix plus the rest of the stream), since the up-to-512-byte read this needs
// would otherwise be lost before files.Save gets to see it.
//
// http.DetectContentType alone isn't enough: its signature table (WHATWG MIME sniffing, a fixed
// list of HTML tag names plus "<?xml") has no entry for "<svg", so an SVG file that doesn't happen
// to open with one of those exact HTML tags sniffs as plain text/xml rather than
// "image/svg+xml" -- an SVG's own <script> would still be denylisted via the "<script" HTML
// signature if it's near the very start, but a real-world SVG (xmlns declaration, etc. before the
// payload) would sail past DetectContentType alone. A direct substring check for "<svg" in the
// sniffed prefix closes that gap regardless of where DetectContentType lands.
func rejectDangerousUpload(file multipart.File) (io.Reader, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	buf = buf[:n]

	if bytes.Contains(bytes.ToLower(buf), []byte("<svg")) {
		return nil, fmt.Errorf("SVG uploads are not allowed")
	}
	contentType := http.DetectContentType(buf)
	for _, bad := range dangerousUploadContentTypes {
		if strings.HasPrefix(contentType, bad) {
			return nil, fmt.Errorf("uploads of type %q are not allowed", contentType)
		}
	}
	return io.MultiReader(bytes.NewReader(buf), file), nil
}

// carryForwardExistingFiles fills values with each FieldTypeFile field's current stored value,
// for every such field uploaded didn't just set -- see handleFileUploads' caller for why. A
// no-op (and no fetch) when machine has no file fields at all.
func carryForwardExistingFiles(ctx context.Context, store *data.Store, machine *domain.Machine, recordID string, uploaded map[string]any, values map[string]any) error {
	hasFileField := false
	for _, f := range machine.Fields {
		if f.Type == domain.FieldTypeFile {
			hasFileField = true
			break
		}
	}
	if !hasFileField {
		return nil
	}

	existing, err := store.GetRecord(ctx, machine.ID, recordID)
	if err != nil {
		return err
	}
	for _, f := range machine.Fields {
		if f.Type != domain.FieldTypeFile {
			continue
		}
		if _, justUploaded := uploaded[f.ID]; justUploaded {
			continue
		}
		if v, ok := existing.Values[f.ID]; ok {
			values[f.ID] = v
		}
	}
	return nil
}
