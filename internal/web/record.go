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

		if err := data.ValidateRecord(machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := data.ValidateRelations(req.Context(), store, machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		record, err := store.CreateRecord(req.Context(), machine.ID, values)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		if req.Header.Get("HX-Request") != "true" {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rendering.RecordDetailPage(machine, record, appName, relations, children, actor).Render(req.Context(), w)
			return
		}
		if isDetailContext(req) {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rendering.RecordDetailView(machine, record, relations, children, actor).Render(req.Context(), w)
			return
		}
		rendering.RecordRow(machine, record, relations).Render(req.Context(), w)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if isDetailContext(req) {
			rendering.RecordDetailEdit(machine, record, relations).Render(req.Context(), w)
			return
		}
		rendering.RecordEditRow(machine, record, relations).Render(req.Context(), w)
	}
}

func updateRecordForm(machines map[string]*domain.Machine, store *data.Store, files *storage.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if err := parseRecordForm(req); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}

		id := chi.URLParam(req, "id")
		values := data.ValuesFromForm(machine, req.Form)
		uploaded, err := handleFileUploads(req, machine, files)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for k, v := range uploaded {
			values[k] = v
		}
		// A browser can't pre-fill <input type="file">, so "no new upload" must not be read as
		// "clear the file" the way an empty text input would be -- carry the existing value
		// forward for any file field a fresh upload didn't touch (ROADMAP.md Phase 11).
		if err := carryForwardExistingFiles(req.Context(), store, machine, id, uploaded, values); err != nil {
			recordError(w, err)
			return
		}

		// An Approval Step's fld_decision only ever changes through POST .../decide, which
		// enforces action.CanDecide's sequencing rule -- the generic edit route must not become
		// a bypass for it (ROADMAP.md Phase 12).
		if machine.ID == action.StepMachineID {
			existing, err := store.GetRecord(req.Context(), machine.ID, id)
			if err != nil {
				recordError(w, err)
				return
			}
			if newDecision, ok := values[action.FieldStepDecision]; ok && fmt.Sprint(newDecision) != fmt.Sprint(existing.Values[action.FieldStepDecision]) {
				http.Error(w, "use Approve/Reject to change a decision, not a direct edit", http.StatusUnprocessableEntity)
				return
			}
		}

		if err := data.ValidateRecord(machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if err := data.ValidateRelations(req.Context(), store, machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		relatedRecords, err := composition.NewLoader(store, machines).ConstraintRelatedRecords(req.Context(), machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := behavior.CheckConstraints(machine, id, values, relatedRecords); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		// Case 19's Project Activity feed wants Task status moves as their own event (ROADMAP.md
		// Phase 14) -- the old status has to be read before the write replaces it.
		var oldTaskStatus string
		if machine.ID == "mch_task" {
			if existing, err := store.GetRecord(req.Context(), machine.ID, id); err == nil {
				oldTaskStatus = toDisplayString(existing.Values["fld_status"])
			}
		}

		record, err := store.UpdateRecord(req.Context(), machine.ID, id, values)
		if err != nil {
			recordError(w, err)
			return
		}
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		if machine.ID == "mch_task" {
			if newStatus := toDisplayString(record.Values["fld_status"]); newStatus != "" && newStatus != oldTaskStatus {
				title := recordLabel(machine, record)
				summary := fmt.Sprintf("%q moved from %s to %s", title, oldTaskStatus, newStatus)
				if newStatus == "done" {
					summary = fmt.Sprintf("%q completed", title)
				}
				logActivity(req.Context(), store, machine.ID, record.ID, actor, summary)
			}
		}
		ld := composition.NewLoader(store, machines)
		relations, err := ld.RelationOptions(req.Context(), machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if isDetailContext(req) {
			children, err := ld.ChildSections(req.Context(), machine, record.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rendering.RecordDetailView(machine, record, relations, children, actor).Render(req.Context(), w)
			return
		}
		rendering.RecordRow(machine, record, relations).Render(req.Context(), w)
	}
}

func deleteRecord(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if err := store.DeleteRecord(req.Context(), machine.ID, chi.URLParam(req, "id")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
// untouched on an edit form submits nothing.
func handleFileUploads(req *http.Request, machine *domain.Machine, files *storage.Store) (map[string]any, error) {
	uploaded := map[string]any{}
	for _, f := range machine.Fields {
		if f.Type != domain.FieldTypeFile {
			continue
		}
		file, header, err := req.FormFile(f.ID)
		if err != nil {
			if errors.Is(err, http.ErrMissingFile) {
				continue
			}
			return nil, fmt.Errorf("read upload for %s: %w", f.ID, err)
		}
		key, saveErr := files.Save(machine.ID, f.ID, header.Filename, file)
		file.Close()
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
