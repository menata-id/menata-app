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
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"menata.app/internal/authorization"
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

	if cfg.AdminUsername == "" || cfg.AdminPassword == "" || cfg.SessionSecret == "" {
		log.Fatal("ADMIN_USERNAME, ADMIN_PASSWORD, and SESSION_SECRET must all be set -- no safe default exists for a credential")
	}

	// Metadata is loaded and validated once at startup: invalid metadata must not enter
	// execution (005-runtime-lifecycle.md Phase 3-4).
	app, err := metadata.LoadApplication(cfg.MetadataPath)
	if err != nil {
		log.Fatalf("failed to load metadata: %v", err)
	}
	machines := make(map[string]*domain.Machine, len(app.Machines))
	for _, m := range app.Machines {
		machines[m.ID] = m
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
	r.Get("/login", showLogin)
	r.Post("/login", submitLogin(cfg))

	r.Group(func(pr chi.Router) {
		pr.Use(requireAuth(cfg))

		pr.Post("/logout", logout(cfg))

		pr.Get("/api/machines", listMachines(app.Machines))
		pr.Get("/api/machines/{machineID}/records", listRecords(store))
		pr.Post("/api/machines/{machineID}/records", createRecord(machines, store))

		pr.Get("/", showMachineList(app.Machines, app.Application.Name))
		pr.Get("/machines/{machineID}", showMachinePage(machines, app.Application.Name, store))
		pr.Post("/machines/{machineID}/records", createRecordForm(machines, store))
		pr.Get("/machines/{machineID}/records/{id}", showRecordRow(machines, store))
		pr.Get("/machines/{machineID}/records/{id}/edit", editRecordRow(machines, store))
		pr.Put("/machines/{machineID}/records/{id}", updateRecordForm(machines, store))
		pr.Delete("/machines/{machineID}/records/{id}", deleteRecord(machines, store))
	})

	log.Printf("menata-app listening on :%s (metadata: %s)", cfg.Port, cfg.MetadataPath)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

// requireAuth gates every route in its group behind a valid session cookie
// (internal/authorization, ROADMAP.md Phase 2). An HTMX/API request gets a plain 401 so the
// client can react; a full-page navigation is redirected to /login.
func requireAuth(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !authorization.IsAuthenticated(req, cfg.SessionSecret) {
				if req.Header.Get("HX-Request") == "true" || strings.HasPrefix(req.URL.Path, "/api/") {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				http.Redirect(w, req, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func showLogin(w http.ResponseWriter, req *http.Request) {
	rendering.LoginPage("").Render(req.Context(), w)
}

func submitLogin(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		if !authorization.CheckCredentials(req.FormValue("username"), req.FormValue("password"), cfg.AdminUsername, cfg.AdminPassword) {
			w.WriteHeader(http.StatusUnauthorized)
			rendering.LoginPage("Invalid username or password").Render(req.Context(), w)
			return
		}
		authorization.SetSessionCookie(w, cfg.SessionSecret, cfg.SecureCookies)
		http.Redirect(w, req, "/", http.StatusSeeOther)
	}
}

func logout(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		authorization.ClearSessionCookie(w, cfg.SecureCookies)
		http.Redirect(w, req, "/login", http.StatusSeeOther)
	}
}

func listMachines(machines []*domain.Machine) http.HandlerFunc {
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

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(record)
	}
}

func showMachineList(machines []*domain.Machine, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		rendering.MachineList(machines, appName).Render(req.Context(), w)
	}
}

func showMachinePage(machines map[string]*domain.Machine, appName string, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}

		records, err := store.ListRecords(req.Context(), machine.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		relations, err := loadRelationOptions(req.Context(), store, machines, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rendering.MachinePage(machine, records, appName, relations).Render(req.Context(), w)
	}
}

func createRecordForm(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
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
		if err := data.ValidateRelations(req.Context(), store, machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		if _, err := store.CreateRecord(req.Context(), machine.ID, values); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		renderMachineBody(w, req, machines, machine, store)
	}
}

func showRecordRow(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
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
		relations, err := loadRelationOptions(req.Context(), store, machines, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		relations, err := loadRelationOptions(req.Context(), store, machines, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rendering.RecordEditRow(machine, record, relations).Render(req.Context(), w)
	}
}

func updateRecordForm(machines map[string]*domain.Machine, store *data.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
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
		if err := data.ValidateRelations(req.Context(), store, machine, values); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		record, err := store.UpdateRecord(req.Context(), machine.ID, id, values)
		if err != nil {
			recordError(w, err)
			return
		}
		relations, err := loadRelationOptions(req.Context(), store, machines, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		// Empty response: HTMX swaps the row's outerHTML with nothing, removing it.
	}
}

func renderMachineBody(w http.ResponseWriter, req *http.Request, machines map[string]*domain.Machine, machine *domain.Machine, store *data.Store) {
	records, err := store.ListRecords(req.Context(), machine.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	relations, err := loadRelationOptions(req.Context(), store, machines, machine)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rendering.MachineBody(machine, records, relations).Render(req.Context(), w)
}

// loadRelationOptions fetches every option a relation field on m could select, keyed by target
// Machine ID. The target's first Field is used as the display label -- a minimal convention
// until a real Projection/semantic "title" role exists (007 SS7.6), which isn't forced yet by a
// case that needs more than one reasonable label field.
func loadRelationOptions(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, m *domain.Machine) (rendering.RelationOptions, error) {
	options := rendering.RelationOptions{}
	for _, f := range m.Fields {
		if f.Type != domain.FieldTypeRelation {
			continue
		}
		if _, loaded := options[f.RelatedMachine]; loaded {
			continue
		}
		target, ok := machines[f.RelatedMachine]
		if !ok || len(target.Fields) == 0 {
			continue
		}
		labelFieldID := target.Fields[0].ID

		records, err := store.ListRecords(ctx, target.ID)
		if err != nil {
			return nil, err
		}
		list := make([]rendering.RelationOption, 0, len(records))
		for _, r := range records {
			list = append(list, rendering.RelationOption{ID: r.ID, Label: toDisplayString(r.Values[labelFieldID])})
		}
		options[f.RelatedMachine] = list
	}
	return options, nil
}

func toDisplayString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func resolveMachine(w http.ResponseWriter, machines map[string]*domain.Machine, req *http.Request) (*domain.Machine, bool) {
	m, ok := machines[chi.URLParam(req, "machineID")]
	if !ok {
		http.Error(w, "unknown machine", http.StatusNotFound)
		return nil, false
	}
	return m, true
}

func recordError(w http.ResponseWriter, err error) {
	if errors.Is(err, data.ErrRecordNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
