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
	"menata.app/internal/behavior"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/db"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
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
		pr.Get("/dashboard", showDashboard(store, app.Application.Name))
		pr.Get("/machines/{machineID}", showMachinePage(machines, app.Application.Name, store))
		pr.Post("/machines/{machineID}/records", createRecordForm(machines, store))
		pr.Get("/machines/{machineID}/records/{id}", showRecordRow(machines, store, app.Application.Name))
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
		authorization.SetSessionCookie(w, cfg.SessionSecret, cfg.AdminUserID, cfg.SecureCookies)
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

// showDashboard combines Project and Task data on one page -- ROADMAP.md Phase 6's own forcing
// case, exercised here for real. It fetches each Machine's full record set once (two queries
// total, not one per Project) and joins them in Go, so it does not, on its own, demonstrate the
// naive-fetch problem Phase 6's planner exists to fix.
func showDashboard(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		projects, err := store.ListRecords(ctx, "mch_project")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tasks, err := store.ListRecords(ctx, "mch_task")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		open := make(map[string]int, len(projects))
		total := make(map[string]int, len(projects))
		for _, t := range tasks {
			projectID, _ := t.Values["fld_project"].(string)
			total[projectID]++
			if toDisplayString(t.Values["fld_status"]) != "done" {
				open[projectID]++
			}
		}

		summaries := make([]rendering.ProjectSummary, 0, len(projects))
		for _, p := range projects {
			summaries = append(summaries, rendering.ProjectSummary{
				Project:    p,
				OpenTasks:  open[p.ID],
				TotalTasks: total[p.ID],
			})
		}

		rendering.DashboardPage(summaries, appName).Render(ctx, w)
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
		boardColumns, err := loadBoardColumns(req.Context(), store, machines, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rendering.MachinePage(machine, records, appName, relations, boardColumns).Render(req.Context(), w)
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

// showRecordRow serves three different renderings of the same Record from one route, depending
// on who's asking (ROADMAP.md Phase 8): a direct browser navigation gets the full detail page;
// an HTMX request targeting the detail page's own container gets just that container's view
// fragment (used by the detail page's own Cancel-from-edit); any other HTMX request (a table row
// or board card's Cancel) gets the original RecordRow fragment, unchanged from Phase 1.
func showRecordRow(machines map[string]*domain.Machine, store *data.Store, appName string) http.HandlerFunc {
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

		if req.Header.Get("HX-Request") != "true" {
			children, err := loadChildSections(req.Context(), store, machines, machine, record.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rendering.RecordDetailPage(machine, record, appName, relations, children).Render(req.Context(), w)
			return
		}
		if isDetailContext(req) {
			children, err := loadChildSections(req.Context(), store, machines, machine, record.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rendering.RecordDetailView(machine, record, relations, children).Render(req.Context(), w)
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
		if isDetailContext(req) {
			rendering.RecordDetailEdit(machine, record, relations).Render(req.Context(), w)
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
		relatedRecords, err := loadConstraintRelatedRecords(req.Context(), store, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := behavior.CheckConstraints(machine, id, values, relatedRecords); err != nil {
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
		if isDetailContext(req) {
			children, err := loadChildSections(req.Context(), store, machines, machine, record.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rendering.RecordDetailView(machine, record, relations, children).Render(req.Context(), w)
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

// isDetailContext reports whether an HTMX request targets the record-detail page's own
// container, as opposed to a table row or board card -- the same fragments serve both contexts
// (ROADMAP.md Phase 8).
func isDetailContext(req *http.Request) bool {
	return req.Header.Get("HX-Target") == "record-detail"
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
	boardColumns, err := loadBoardColumns(req.Context(), store, machines, machine)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rendering.MachineBody(machine, records, relations, boardColumns).Render(req.Context(), w)
}

// loadConstraintRelatedRecords fetches every record of each Constraint's related Machine, keyed
// by that Machine's ID, for behavior.CheckConstraints to evaluate against.
func loadConstraintRelatedRecords(ctx context.Context, store *data.Store, m *domain.Machine) (map[string][]*data.Record, error) {
	related := map[string][]*data.Record{}
	for _, c := range m.Constraints {
		if _, loaded := related[c.BlockIf.RelatedMachine]; loaded {
			continue
		}
		records, err := store.ListRecords(ctx, c.BlockIf.RelatedMachine)
		if err != nil {
			return nil, err
		}
		related[c.BlockIf.RelatedMachine] = records
	}
	return related, nil
}

// loadChildSections resolves every child collection pointing at (m, recordID) -- every record of
// another Machine whose reference field names this one (ROADMAP.md Phase 9) -- fetching each
// collection's records and the relation options its own rows need to render.
func loadChildSections(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, m *domain.Machine, recordID string) ([]rendering.ChildSection, error) {
	var sections []rendering.ChildSection
	for _, cc := range domain.FindChildCollections(machineSlice(machines), m.ID) {
		records, err := store.ListRecordsBy(ctx, cc.Machine.ID, cc.Field.ID, recordID)
		if err != nil {
			return nil, err
		}
		relations, err := loadRelationOptions(ctx, store, machines, cc.Machine)
		if err != nil {
			return nil, err
		}
		sections = append(sections, rendering.ChildSection{Machine: cc.Machine, Records: records, Relations: relations})
	}
	return sections, nil
}

func machineSlice(machines map[string]*domain.Machine) []*domain.Machine {
	list := make([]*domain.Machine, 0, len(machines))
	for _, m := range machines {
		list = append(list, m)
	}
	return list
}

// loadBoardColumns resolves board columns for m when its board Layout groups by a reference
// field (ROADMAP.md Phase 10's ordered Lists, e.g. mch_list) -- fetching those real records is
// I/O experience.GroupRecords doesn't perform itself. Returns nil (not an error) when m isn't a
// board, or groups by an ordinary status field instead: GroupRecords computes its own columns
// from that Field's Options in that case, unchanged since Phase 5.
func loadBoardColumns(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, m *domain.Machine) ([]experience.Column, error) {
	if m.View.EffectiveLayout() != domain.LayoutBoard {
		return nil, nil
	}
	groupField, ok := m.FieldByID(m.View.GroupBy)
	if !ok || !groupField.IsReference() {
		return nil, nil
	}
	listMachine, ok := machines[groupField.RelatedMachine]
	if !ok || len(listMachine.Fields) == 0 {
		return nil, nil
	}

	records, err := store.ListRecords(ctx, listMachine.ID)
	if err != nil {
		return nil, err
	}
	labelFieldID := listMachine.Fields[0].ID
	columns := make([]experience.Column, 0, len(records))
	for _, r := range records {
		columns = append(columns, experience.Column{ID: r.ID, Label: toDisplayString(r.Values[labelFieldID])})
	}
	return columns, nil
}

// loadRelationOptions fetches every option a reference field on m could select (Relation or
// Person, per domain.Field.IsReference), keyed by target Machine ID. The target's first Field is
// used as the display label -- a minimal convention until a real Projection/semantic "title"
// role exists (007 SS7.6), which isn't forced yet by a case that needs more than one reasonable
// label field.
func loadRelationOptions(ctx context.Context, store *data.Store, machines map[string]*domain.Machine, m *domain.Machine) (rendering.RelationOptions, error) {
	options := rendering.RelationOptions{}
	for _, f := range m.Fields {
		if !f.IsReference() {
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
