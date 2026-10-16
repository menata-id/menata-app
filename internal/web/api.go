package web

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/mail"
	"menata.app/internal/storage"
)

func listMachines() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The JSON twin of All Machines, narrowed the same way: this Workspace's own Machines,
		// not every Machine the process happens to have loaded.
		writeJSON(w, installedMachines(req.Context()))
	}
}

func listRecords(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machineID := chi.URLParam(req, "machineID")
		records, err := store.ListRecords(req.Context(), machineID)
		if err != nil {
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, records)
	}
}

func createRecord(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}

		var values map[string]any
		if err := json.NewDecoder(req.Body).Decode(&values); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		// The same pipeline the form route runs (defaults, stamps, the create Permission, the guards, the write inside its
		// locks, then the create Events), so the two cannot drift: the JSON path had silently never logged Activity once.
		record, _, ok := createFromValues(w, req, store, files, mailer, cfg, machine, values)
		if !ok {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, record)
	}
}

// updateRecord is /api's PUT counterpart to updateRecordForm -- same guards (write order matters:
// carry-forward files, then decision-change guard, then shape/relation/constraint checks), a JSON
// body decoded straight into map[string]any needing no ValuesFromForm equivalent (createRecord
// above already established this), and a JSON response instead of an HTML fragment.
func updateRecord(store *data.Store, files *storage.Store, mailer mail.Mailer, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machines := machinesFor(req.Context())
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if refusesAppendOnlyWrite(w, machine) {
			return
		}
		id := chi.URLParam(req, "id")
		actor := currentActor(req, store, cfg)
		if !allowsRecordEdit(w, req, store, machine, id, actor) {
			return
		}

		var values map[string]any
		if err := json.NewDecoder(req.Body).Decode(&values); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if !carryForwardFixedFields(w, req, store, machine, id, nil, values) {
			return
		}
		record, ok := commitRecordUpdate(w, req, store, files, mailer, machines, machine, id, values, actor)
		if !ok {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, record)
	}
}

func deleteRecordAPI(store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, req)
		if !ok {
			return
		}
		if refusesAppendOnlyWrite(w, machine) {
			return
		}
		id := chi.URLParam(req, "id")
		actor := currentActor(req, store, cfg)
		if !deleteLocked(w, req, store, machine, id, actor) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
