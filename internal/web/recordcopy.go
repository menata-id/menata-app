package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/mail"
	"menata.app/internal/storage"
)

// copyRecord makes a new record from an existing one (Case 19 PM02b's "Copy…"), named by the form. It is a
// create like any other -- createFromValues -- so the Machine's create Permission, validation and on_create
// Events all apply to the copy; what is its own is only where the values come from. Records the Machine holds
// *about* the source (a Task's checklist items, comments, attachments, labels) are not copied: they are other
// Machines' records, and a copy of them is a decision about each child Machine this does not make.
func copyRecord(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		titleField := machine.CopyTitleField()
		if titleField == "" {
			http.Error(w, machine.Name+" records cannot be copied", http.StatusUnprocessableEntity)
			return
		}
		if err := parseRecordForm(req); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		source, err := store.GetRecord(req.Context(), machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		values := data.CopyableValues(machine, source.Values)
		if title := strings.TrimSpace(req.Form.Get(titleField)); title != "" {
			values[titleField] = title
		}
		record, _, ok := createFromValues(w, req, store, files, mailer, cfg, machine, values)
		if !ok {
			return
		}
		w.Header().Set("HX-Redirect", fmt.Sprintf("/machines/%s/records/%s", machine.ID, record.ID))
	}
}
