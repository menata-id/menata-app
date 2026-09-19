package web

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func createRecord(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
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

		if !validRecord(w, req, store, machine, values) {
			return
		}

		record, err := store.CreateRecord(req.Context(), machine.ID, values)
		if err != nil {
			serverError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, record)
	}
}
