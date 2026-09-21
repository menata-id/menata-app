package web

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func listMachines(machines []*domain.Machine) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, machines)
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

func createRecord(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}

		var values map[string]any
		if err := json.NewDecoder(req.Body).Decode(&values); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
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
		// Parity with createRecordForm's own logging -- found while adding update/delete parity
		// below: the JSON path had silently never logged Activity for a Document/Task/Project
		// created through it, unlike its form-based sibling.
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		runCreateEvents(req.Context(), store, machine, record, actor)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, record)
	}
}

// updateRecord is /api's PUT counterpart to updateRecordForm -- same guards (write order matters:
// carry-forward files, then decision-change guard, then shape/relation/constraint checks), a JSON
// body decoded straight into map[string]any needing no ValuesFromForm equivalent (createRecord
// above already established this), and a JSON response instead of an HTML fragment.
func updateRecord(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
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
		if !carryForwardFiles(w, req, store, machine, id, nil, values) {
			return
		}
		if !allowsTransition(w, req, store, machine, id, values) {
			return
		}
		if !passesWriteGuards(w, req, store, machines, machine, id, values) {
			return
		}

		oldValues, oldValuesOK := eventOldValues(req, store, machine, id)
		record, err := store.UpdateRecord(req.Context(), machine.ID, id, values)
		if err != nil {
			recordError(w, err)
			return
		}
		runEvents(req.Context(), store, machine, record, actor.ID, oldValues, oldValuesOK)

		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, record)
	}
}

func deleteRecordAPI(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		id := chi.URLParam(req, "id")
		actor := currentActor(req, store, cfg)
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
		w.WriteHeader(http.StatusNoContent)
	}
}
