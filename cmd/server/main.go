// Command server is the Menata App entrypoint: a single binary that will grow to realize
// Runtime Metadata into a running application, per 002-architecture.md's Runtime Realization
// pipeline.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/db"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	// Metadata is loaded and validated once at startup: invalid metadata must not enter
	// execution (005-runtime-lifecycle.md Phase 3-4).
	machine, err := metadata.Load(cfg.MetadataPath)
	if err != nil {
		log.Fatalf("failed to load metadata: %v", err)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()
	store := data.NewStore(pool)

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/api/machines", listMachines(machine))
	r.Get("/api/machines/{machineID}/records", listRecords(store))
	r.Post("/api/machines/{machineID}/records", createRecord(machine, store))

	r.Get("/", showMachinePage(machine, store))
	r.Post("/machines/{machineID}/records", createRecordForm(machine, store))
	r.Get("/machines/{machineID}/records/{id}", showRecordRow(machine, store))
	r.Get("/machines/{machineID}/records/{id}/edit", editRecordRow(machine, store))
	r.Put("/machines/{machineID}/records/{id}", updateRecordForm(machine, store))
	r.Delete("/machines/{machineID}/records/{id}", deleteRecord(machine, store))

	log.Printf("menata-app listening on :%s (metadata: %s)", cfg.Port, cfg.MetadataPath)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

func listMachines(machines ...*domain.Machine) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(machines)
	}
}

func listRecords(store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machineID := chi.URLParam(req, "machineID")
		records, err := store.ListRecords(req.Context(), machineID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(records)
	}
}

func createRecord(machine *domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machineID := chi.URLParam(req, "machineID")
		if machineID != machine.ID {
			http.Error(w, "unknown machine", http.StatusNotFound)
			return
		}

		var values map[string]any
		if err := json.NewDecoder(req.Body).Decode(&values); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		if err := data.ValidateRecord(machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		record, err := store.CreateRecord(req.Context(), machineID, values)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(record)
	}
}

func showMachinePage(machine *domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		records, err := store.ListRecords(req.Context(), machine.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rendering.MachinePage(machine, records).Render(req.Context(), w)
	}
}

// createRecordForm, showRecordRow, editRecordRow, updateRecordForm, and deleteRecord serve the
// browser-facing HTMX flow: only one Machine exists yet, so machineID is checked but not yet
// used to look up among several (ROADMAP.md Phase 3 generalizes this once a second Machine
// exists to force it).

func createRecordForm(machine *domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !isMachine(w, machine, req) {
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}

		values := data.ValuesFromForm(machine, req.Form)
		if err := data.ValidateRecord(machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if _, err := store.CreateRecord(req.Context(), machine.ID, values); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		renderMachineBody(w, req, machine, store)
	}
}

func showRecordRow(machine *domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !isMachine(w, machine, req) {
			return
		}
		record, err := store.GetRecord(req.Context(), machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		rendering.RecordRow(machine, record).Render(req.Context(), w)
	}
}

func editRecordRow(machine *domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !isMachine(w, machine, req) {
			return
		}
		record, err := store.GetRecord(req.Context(), machine.ID, chi.URLParam(req, "id"))
		if err != nil {
			recordError(w, err)
			return
		}
		rendering.RecordEditRow(machine, record).Render(req.Context(), w)
	}
}

func updateRecordForm(machine *domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !isMachine(w, machine, req) {
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}

		id := chi.URLParam(req, "id")
		values := data.ValuesFromForm(machine, req.Form)
		if err := data.ValidateRecord(machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		record, err := store.UpdateRecord(req.Context(), machine.ID, id, values)
		if err != nil {
			recordError(w, err)
			return
		}
		rendering.RecordRow(machine, record).Render(req.Context(), w)
	}
}

func deleteRecord(machine *domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !isMachine(w, machine, req) {
			return
		}
		if err := store.DeleteRecord(req.Context(), machine.ID, chi.URLParam(req, "id")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Empty response: HTMX swaps the row's outerHTML with nothing, removing it.
	}
}

func renderMachineBody(w http.ResponseWriter, req *http.Request, machine *domain.Machine, store *data.Store) {
	records, err := store.ListRecords(req.Context(), machine.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rendering.MachineBody(machine, records).Render(req.Context(), w)
}

func isMachine(w http.ResponseWriter, machine *domain.Machine, req *http.Request) bool {
	if chi.URLParam(req, "machineID") != machine.ID {
		http.Error(w, "unknown machine", http.StatusNotFound)
		return false
	}
	return true
}

func recordError(w http.ResponseWriter, err error) {
	if errors.Is(err, data.ErrRecordNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
