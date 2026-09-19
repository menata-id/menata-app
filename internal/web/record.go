package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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
		switch machine.ID {
		case action.DocumentMachineID:
			actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
			logActivity(req.Context(), store, machine.ID, record.ID, actor, fmt.Sprintf("%q submitted", toDisplayString(record.Values["fld_title"])))
		case "mch_task", "mch_project":
			actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
			logActivity(req.Context(), store, machine.ID, record.ID, actor, fmt.Sprintf("%q created", recordLabel(machine, record)))
		}

		renderMachineBody(w, req, machines, machine, store)
	}
}

// showRecordRow serves three different renderings of the same Record from one route, depending
// on who's asking (ROADMAP.md Phase 8): a direct browser navigation gets the full detail page;
// an HTMX request targeting the detail page's own container gets just that container's view
// fragment (used by the detail page's own Cancel-from-edit); any other HTMX request (a table row
// or board card's Cancel) gets the original RecordRow fragment, unchanged from Phase 1.
func showRecordRow(machines map[string]*domain.Machine, store *data.Store, appName string, cfg config.Config) http.HandlerFunc {
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
			render(req.Context(), w, rendering.RecordDetailPage(machine, record, appName, relations, children, actor))
			return
		}
		if isDetailContext(req) {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			render(req.Context(), w, rendering.RecordDetailView(machine, record, relations, children, actor))
			return
		}
		render(req.Context(), w, rendering.RecordRow(machine, record, relations))
	}
}

func editRecordRow(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
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
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		logTaskStatusMove(req, store, machine, record, actor, oldTaskStatus)

		renderRecord(w, req, machines, store, machine, record, actor)
	}
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
func renderRecord(w http.ResponseWriter, req *http.Request, machines map[string]*domain.Machine, store *data.Store, machine *domain.Machine, record *data.Record, actor string) {
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
		render(req.Context(), w, rendering.RecordDetailView(machine, record, relations, children, actor))
		return
	}
	render(req.Context(), w, rendering.RecordRow(machine, record, relations))
}

func deleteRecord(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if err := store.DeleteRecord(req.Context(), machine.ID, chi.URLParam(req, "id")); err != nil {
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
		key, saveErr := files.Save(machine.ID, f.ID, header.Filename, file)
		_ = file.Close() // read handle on the uploaded part; nothing to act on if this fails
		if saveErr != nil {
			return nil, saveErr
		}
		uploaded[f.ID] = key
	}
	return uploaded, nil
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
