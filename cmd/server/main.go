// Command server is the Menata App entrypoint: a single binary that will grow to realize
// Runtime Metadata into a running application, per 002-architecture.md's Runtime Realization
// pipeline.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"menata.app/internal/action"
	"menata.app/internal/authorization"
	"menata.app/internal/behavior"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/db"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
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

	files, err := storage.NewStore(cfg.UploadsDir)
	if err != nil {
		log.Fatalf("failed to set up uploads directory: %v", err)
	}

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
		pr.Get("/my-tasks", showMyTasks(store, app.Application.Name, cfg))
		pr.Get("/board-settings", showBoardSettings(store, app.Application.Name))
		pr.Get("/activity", showActivity(store, app.Application.Name))
		pr.Get("/team-capacity", showTeamCapacity(store, app.Application.Name))
		pr.Get("/automation", showAutomation(app.Machines, app.Application.Name))
		pr.Get("/calendar", showCalendar(store, app.Application.Name))
		pr.Get("/sprint", showSprintDashboard(store, app.Application.Name))
		pr.Get("/approval-inbox", showApprovalInbox(store, app.Application.Name, cfg))
		pr.Get("/machines/{machineID}", showMachinePage(machines, app.Application.Name, store))
		pr.Post("/machines/{machineID}/records", createRecordForm(machines, store, files, cfg))
		pr.Get("/machines/{machineID}/records/{id}", showRecordRow(machines, store, app.Application.Name, cfg))
		pr.Get("/machines/{machineID}/records/{id}/edit", editRecordRow(machines, store))
		pr.Put("/machines/{machineID}/records/{id}", updateRecordForm(machines, store, files, cfg))
		pr.Delete("/machines/{machineID}/records/{id}", deleteRecord(machines, store))
		pr.Post("/machines/{machineID}/records/{id}/decide", decideStep(machines, store, cfg))

		pr.Get("/uploads/*", serveUpload(files))

		// Static design references (ROADMAP.md's own case-portfolio.md mockups) -- ui-sample/ is
		// a design reference, never current-code intent (per this repo's own convention), served
		// as-is with no rendering logic of its own so it's easy to compare against the real pages
		// above.
		pr.Handle("/ui-sample/*", http.StripPrefix("/ui-sample/", http.FileServer(http.Dir("ui-sample"))))
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

		documents, err := store.ListRecords(ctx, "mch_document")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var docs rendering.DocumentSummary
		var pending []*data.Record
		for _, d := range documents {
			switch toDisplayString(d.Values["fld_status"]) {
			case "draft":
				docs.Draft++
			case "in_review":
				docs.InReview++
				pending = append(pending, d)
			case "approved":
				docs.Approved++
			case "rejected":
				docs.Rejected++
			}
		}

		activity, err := recentActivity(ctx, store, 10)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rendering.DashboardPage(summaries, docs, pending, activity, appName).Render(ctx, w)
	}
}

// recentActivity loads mch_activity records, resolves each fld_actor id to the actor's display
// name (reusing the same label-field convention as loadRelationOptions), and returns the most
// recent limit entries newest-first.
func recentActivity(ctx context.Context, store *data.Store, limit int) ([]rendering.ActivityEntry, error) {
	events, names, err := loadRecentEvents(ctx, store, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]rendering.ActivityEntry, 0, len(events))
	for _, e := range events {
		entries = append(entries, rendering.ActivityEntry{
			Summary: toDisplayString(e.Values["fld_summary"]),
			Actor:   names[toDisplayString(e.Values["fld_actor"])],
			When:    e.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	return entries, nil
}

// loadRecentEvents fetches mch_activity records newest-first (capped at limit, or all when limit
// is 0), plus an actor-id -> display-name map -- the shared I/O behind both the Dashboard's
// flat Recent Activity list and the dedicated /activity page's day-grouped feed (ROADMAP.md
// Phase 14), which need the same data shaped two different ways.
func loadRecentEvents(ctx context.Context, store *data.Store, limit int) ([]*data.Record, map[string]string, error) {
	events, err := store.ListRecords(ctx, "mch_activity")
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	users, err := store.ListRecords(ctx, "mch_user")
	if err != nil {
		return nil, nil, err
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID] = toDisplayString(u.Values["fld_name"])
	}
	return events, names, nil
}

// showActivity is Case 19's cross-project event feed (ROADMAP.md Phase 14,
// project-activity.html): the same mch_activity data as the Dashboard's Recent Activity section,
// grouped by day (Today/Yesterday/Earlier) instead of a flat top-10 list.
func showActivity(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		events, names, err := loadRecentEvents(ctx, store, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		now := time.Now()
		yesterday := now.AddDate(0, 0, -1)
		var today, yest, older []rendering.ActivityEntry
		for _, e := range events {
			entry := rendering.ActivityEntry{
				Summary: toDisplayString(e.Values["fld_summary"]),
				Actor:   names[toDisplayString(e.Values["fld_actor"])],
			}
			switch {
			case sameDay(e.CreatedAt, now):
				entry.When = e.CreatedAt.Format("15:04")
				today = append(today, entry)
			case sameDay(e.CreatedAt, yesterday):
				entry.When = e.CreatedAt.Format("15:04")
				yest = append(yest, entry)
			default:
				entry.When = e.CreatedAt.Format("2006-01-02 15:04")
				older = append(older, entry)
			}
		}

		rendering.ActivityPage(today, yest, older, appName).Render(ctx, w)
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// recordLabel is a Record's display label -- its Machine's first Field's value, the same
// label-field convention used throughout (loadRelationOptions, rendering.recordTitle).
func recordLabel(m *domain.Machine, r *data.Record) string {
	if len(m.Fields) == 0 {
		return r.ID
	}
	return toDisplayString(r.Values[m.Fields[0].ID])
}

// showMyTasks is Case 19's personal work queue (ROADMAP.md Phase 14): every mch_task assigned to
// the current identity, bucketed into Today/Upcoming/Completed. "Assigned to me" resolves to
// authorization.CurrentUserID -- the same shared-admin-credential-to-real-mch_user resolution
// Phase 8 already built, not a new per-user login mechanism. Bucketing reuses
// experience.EvaluateSLA (Phase 13) rather than re-deriving day-truncation logic.
func showMyTasks(store *data.Store, appName string, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		tasks, err := store.ListRecords(ctx, "mch_task")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projects, err := store.ListRecords(ctx, "mch_project")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projectNames := make(map[string]string, len(projects))
		for _, p := range projects {
			projectNames[p.ID] = toDisplayString(p.Values["fld_name"])
		}

		now := time.Now()
		var summary rendering.MyTasksSummary
		var today, upcoming, completed []rendering.TaskRow
		for _, t := range tasks {
			if toDisplayString(t.Values["fld_assignee"]) != userID {
				continue
			}
			row := rendering.TaskRow{Task: t, ProjectName: projectNames[toDisplayString(t.Values["fld_project"])]}

			if toDisplayString(t.Values["fld_status"]) == "done" {
				completed = append(completed, row)
				continue
			}
			summary.Open++

			due, err := time.Parse("2006-01-02", toDisplayString(t.Values["fld_due_date"]))
			if err != nil {
				upcoming = append(upcoming, row)
				continue
			}
			status, label := experience.EvaluateSLA(due, now)
			switch {
			case status == experience.SLAOverdue:
				summary.Overdue++
				today = append(today, row)
			case label == "Due today":
				summary.DueToday++
				today = append(today, row)
			default:
				upcoming = append(upcoming, row)
			}
		}

		rendering.MyTasksPage(summary, today, upcoming, completed, appName).Render(ctx, w)
	}
}

// showTeamCapacity is Case 19's Team Capacity screen (ROADMAP.md Phase 14, project-team.html):
// every mch_user with their declared weekly capacity (a new Number field on an existing Machine,
// not a new mechanism) and how many mch_task are currently assigned to them, still open.
// showAutomation is Case 19's Workflow Automation screen (ROADMAP.md Phase 14,
// project-automation.html): a read-only Trigger/Condition/Action description of this
// Application's real Constraint metadata and Action behavior -- not a generic automation engine
// (no forcing case has built one) and not fictional example workflows.
// showCalendar is Case 19's week-grid Layout (ROADMAP.md Phase 14, project-calendar.html): every
// mch_task whose fld_due_date falls in the current Monday-Sunday week, one column per day. Pure
// composition over the same Task/Project data My Tasks already loads -- fld_due_date already
// exists, no new Field or Layout mechanism needed.
// showSprintDashboard is Case 19's analytics view (ROADMAP.md Phase 14, project-dashboard.html):
// a real Task-status summary, a workload preview (reusing MemberCapacity from Team Capacity), and
// an Attention Needed list of overdue/due-today Tasks (reusing My Tasks' own SLA bucketing). The
// mockup's points/burndown/blocked content is deliberately not reproduced -- see
// rendering.SprintSummary's own doc comment for why.
// showApprovalInbox is Case 3's Approval Inbox (ROADMAP.md Phase 15 Step 1,
// document-approval.html): every Approval Step assigned to the current identity, still pending,
// and actually actionable right now (action.CanDecide -- a locked sequential step doesn't belong
// in "pending my approval" even though its own fld_decision is "pending"), filtered by an SLA
// bucket; plus every Document the current identity has submitted. Both rendered as
// rendering.SummaryCard (Phase 15 Step 1's new shared component). "Submitted by" is derived from
// the existing mch_activity log (Phase 13's own "submitted" event) rather than a new Document
// Field -- Document already has no user-editable slot for this, and the data already exists.
func showApprovalInbox(store *data.Store, appName string, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		steps, err := store.ListRecords(ctx, action.StepMachineID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		documents, err := store.ListRecords(ctx, action.DocumentMachineID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		activities, err := store.ListRecords(ctx, "mch_activity")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users, err := store.ListRecords(ctx, "mch_user")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		docByID := make(map[string]*data.Record, len(documents))
		for _, d := range documents {
			docByID[d.ID] = d
		}
		stepsByDoc := make(map[string][]*data.Record, len(documents))
		for _, s := range steps {
			docID := toDisplayString(s.Values[action.FieldStepDocument])
			stepsByDoc[docID] = append(stepsByDoc[docID], s)
		}
		names := make(map[string]string, len(users))
		for _, u := range users {
			names[u.ID] = toDisplayString(u.Values["fld_name"])
		}

		sort.Slice(activities, func(i, j int) bool {
			return activities[i].CreatedAt.Before(activities[j].CreatedAt)
		})
		submitterByDoc := make(map[string]string, len(documents))
		for _, a := range activities {
			docID := toDisplayString(a.Values["fld_record_id"])
			if _, ok := submitterByDoc[docID]; ok {
				continue
			}
			if actor := toDisplayString(a.Values["fld_actor"]); actor != "" {
				submitterByDoc[docID] = actor
			}
		}

		now := time.Now()
		var allPending []rendering.SummaryCard
		var allBuckets []string
		var overdueCount, todayCount int
		for _, s := range steps {
			if toDisplayString(s.Values["fld_assignee"]) != userID {
				continue
			}
			if toDisplayString(s.Values[action.FieldStepDecision]) != action.DecisionPending {
				continue
			}
			docID := toDisplayString(s.Values[action.FieldStepDocument])
			doc := docByID[docID]
			if doc == nil {
				continue
			}
			mode := toDisplayString(doc.Values[action.FieldDocumentMode])
			if !action.CanDecide(mode, s, stepsByDoc[docID]) {
				continue
			}

			approved := 0
			for _, sib := range stepsByDoc[docID] {
				if toDisplayString(sib.Values[action.FieldStepDecision]) == action.DecisionApproved {
					approved++
				}
			}
			title := toDisplayString(doc.Values["fld_title"])
			submitter := names[submitterByDoc[docID]]
			if submitter == "" {
				submitter = "someone"
			}

			bucket := "upcoming"
			if due, err := time.Parse("2006-01-02", toDisplayString(doc.Values["fld_due_date"])); err == nil {
				if status, label := experience.EvaluateSLA(due, now); status == experience.SLAOverdue {
					bucket = "overdue"
					overdueCount++
				} else if label == "Due today" {
					bucket = "today"
					todayCount++
				}
			}
			allPending = append(allPending, rendering.SummaryCard{
				AvatarInitials: initials(submitter),
				Title:          title,
				Subtitle:       fmt.Sprintf("%s · %d/%d approved · Submitted by %s", mode, approved, len(stepsByDoc[docID]), submitter),
				StatusLabel:    toDisplayString(doc.Values["fld_status"]),
				SLADue:         doc.Values["fld_due_date"],
				Href:           fmt.Sprintf("/machines/%s/records/%s", action.StepMachineID, s.ID),
			})
			allBuckets = append(allBuckets, bucket)
		}

		filterKey := req.URL.Query().Get("filter")
		filters := []rendering.SLAFilter{
			{Key: "all", Label: "All", Count: len(allPending), Active: filterKey == "" || filterKey == "all"},
			{Key: "overdue", Label: "Overdue", Count: overdueCount, Active: filterKey == "overdue"},
			{Key: "today", Label: "Due today", Count: todayCount, Active: filterKey == "today"},
		}

		pending := allPending
		if filterKey == "overdue" || filterKey == "today" {
			pending = nil
			for i, c := range allPending {
				if allBuckets[i] == filterKey {
					pending = append(pending, c)
				}
			}
		}

		var mine []rendering.SummaryCard
		for _, d := range documents {
			if submitterByDoc[d.ID] != userID {
				continue
			}
			title := toDisplayString(d.Values["fld_title"])
			mine = append(mine, rendering.SummaryCard{
				AvatarInitials: initials(names[userID]),
				Title:          title,
				Subtitle:       "Submitted by you",
				StatusLabel:    toDisplayString(d.Values["fld_status"]),
				Href:           fmt.Sprintf("/machines/%s/records/%s", action.DocumentMachineID, d.ID),
			})
		}

		rendering.ApprovalInboxPage(filters, pending, mine, appName).Render(ctx, w)
	}
}

// initials is a person's display initials for a SummaryCard's avatar (Study 38's Avatar cluster)
// -- the first letter of up to the first two words of name.
func initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "?"
	}
	out := strings.ToUpper(fields[0][:1])
	if len(fields) > 1 {
		out += strings.ToUpper(fields[1][:1])
	}
	return out
}

func showSprintDashboard(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		tasks, err := store.ListRecords(ctx, "mch_task")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users, err := store.ListRecords(ctx, "mch_user")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projects, err := store.ListRecords(ctx, "mch_project")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projectNames := make(map[string]string, len(projects))
		for _, p := range projects {
			projectNames[p.ID] = toDisplayString(p.Values["fld_name"])
		}

		var summary rendering.SprintSummary
		active := make(map[string]int, len(users))
		now := time.Now()
		var attention []rendering.TaskRow
		for _, t := range tasks {
			summary.Total++
			status := toDisplayString(t.Values["fld_status"])
			switch status {
			case "todo":
				summary.Open++
			case "in_progress":
				summary.InProgress++
			case "done":
				summary.Done++
			}
			if status != "done" {
				active[toDisplayString(t.Values["fld_assignee"])]++
				if due, err := time.Parse("2006-01-02", toDisplayString(t.Values["fld_due_date"])); err == nil {
					if slaStatus, label := experience.EvaluateSLA(due, now); slaStatus == experience.SLAOverdue || label == "Due today" {
						attention = append(attention, rendering.TaskRow{Task: t, ProjectName: projectNames[toDisplayString(t.Values["fld_project"])]})
					}
				}
			}
		}

		workload := make([]rendering.MemberCapacity, 0, len(users))
		for _, u := range users {
			workload = append(workload, rendering.MemberCapacity{User: u, ActiveCards: active[u.ID]})
		}

		rendering.SprintDashboardPage(summary, workload, attention, appName).Render(ctx, w)
	}
}

func showCalendar(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		tasks, err := store.ListRecords(ctx, "mch_task")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projects, err := store.ListRecords(ctx, "mch_project")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		projectNames := make(map[string]string, len(projects))
		for _, p := range projects {
			projectNames[p.ID] = toDisplayString(p.Values["fld_name"])
		}

		byDate := make(map[string][]rendering.TaskRow)
		for _, t := range tasks {
			due := toDisplayString(t.Values["fld_due_date"])
			if due == "" {
				continue
			}
			byDate[due] = append(byDate[due], rendering.TaskRow{
				Task:        t,
				ProjectName: projectNames[toDisplayString(t.Values["fld_project"])],
			})
		}

		now := time.Now()
		offset := int(now.Weekday()) - int(time.Monday)
		if offset < 0 {
			offset += 7
		}
		monday := now.AddDate(0, 0, -offset)

		days := make([]rendering.CalendarDay, 0, 7)
		for i := 0; i < 7; i++ {
			day := monday.AddDate(0, 0, i)
			days = append(days, rendering.CalendarDay{
				Label:   day.Format("Mon Jan 2"),
				IsToday: sameDay(day, now),
				Tasks:   byDate[day.Format("2006-01-02")],
			})
		}

		rendering.CalendarPage(days, appName).Render(ctx, w)
	}
}

func showAutomation(machines []*domain.Machine, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var rules []rendering.AutomationRule
		for _, m := range machines {
			for _, c := range m.Constraints {
				onField, _ := m.FieldByID(c.On)
				relatedFieldName := c.BlockIf.Condition.Field
				if related := findMachine(machines, c.BlockIf.RelatedMachine); related != nil {
					if f, ok := related.FieldByID(c.BlockIf.Condition.Field); ok {
						relatedFieldName = f.Name
					}
				}
				rules = append(rules, rendering.AutomationRule{
					Name:      c.ID,
					Trigger:   fmt.Sprintf("%s's %s becomes %q", m.Name, onField.Name, c.WhenEquals),
					Condition: fmt.Sprintf("a related %s (via its %s field) has %s %s %q", c.BlockIf.RelatedMachine, c.BlockIf.RelatedField, relatedFieldName, c.BlockIf.Condition.Op, c.BlockIf.Condition.Value),
					Action:    "Block the transition (422)",
				})
			}
		}

		rules = append(rules, rendering.AutomationRule{
			Name:      "Approval Step sequencing",
			Trigger:   "POST /machines/mch_approval_step/records/{id}/decide",
			Condition: "sequential mode: every earlier-sequence step on the same Document is already approved; parallel mode: always",
			Action:    "Record the decision; recompute the Document's aggregate status (approved once every step is approved, rejected if any step is)",
		})

		rendering.AutomationPage(rules, appName).Render(req.Context(), w)
	}
}

func findMachine(machines []*domain.Machine, id string) *domain.Machine {
	for _, m := range machines {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func showTeamCapacity(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		users, err := store.ListRecords(ctx, "mch_user")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tasks, err := store.ListRecords(ctx, "mch_task")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		active := make(map[string]int, len(users))
		total := make(map[string]int, len(users))
		for _, t := range tasks {
			assignee := toDisplayString(t.Values["fld_assignee"])
			total[assignee]++
			if toDisplayString(t.Values["fld_status"]) != "done" {
				active[assignee]++
			}
		}

		var totalCapacity, totalActive int
		members := make([]rendering.MemberCapacity, 0, len(users))
		for _, u := range users {
			if cap, ok := u.Values["fld_weekly_capacity"].(float64); ok {
				totalCapacity += int(cap)
			}
			totalActive += active[u.ID]
			members = append(members, rendering.MemberCapacity{
				User:        u,
				ActiveCards: active[u.ID],
				TotalCards:  total[u.ID],
			})
		}

		rendering.TeamCapacityPage(members, totalCapacity, totalActive, appName).Render(ctx, w)
	}
}

func showBoardSettings(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		lists, err := store.ListRecords(ctx, "mch_list")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		labels, err := store.ListRecords(ctx, "mch_label")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rendering.BoardSettingsPage(lists, labels, appName).Render(ctx, w)
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
		relations, err := loadRelationOptions(req.Context(), store, machines, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		if req.Header.Get("HX-Request") != "true" {
			children, err := loadChildSections(req.Context(), store, machines, machine, record.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			rendering.RecordDetailPage(machine, record, appName, relations, children, actor).Render(req.Context(), w)
			return
		}
		if isDetailContext(req) {
			children, err := loadChildSections(req.Context(), store, machines, machine, record.ID)
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
		relatedRecords, err := loadConstraintRelatedRecords(req.Context(), store, machine)
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

// decideStep is Case 3's core Action (ROADMAP.md Phase 12): Approve or Reject one Approval Step,
// enforcing action.CanDecide's sequencing rule, then recomputing and saving the parent
// Document's own aggregate status. Hardcoded to mch_approval_step/mch_document, matching
// internal/action's own scope -- not a generic action-dispatch route.
func decideStep(machines map[string]*domain.Machine, store *data.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		machine, ok := resolveMachine(w, machines, req)
		if !ok {
			return
		}
		if machine.ID != action.StepMachineID {
			http.Error(w, "this machine has no decide action", http.StatusNotFound)
			return
		}
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		decision := req.FormValue("decision")
		if decision != action.DecisionApproved && decision != action.DecisionRejected {
			http.Error(w, "decision must be approved or rejected", http.StatusUnprocessableEntity)
			return
		}

		ctx := req.Context()
		id := chi.URLParam(req, "id")
		step, err := store.GetRecord(ctx, machine.ID, id)
		if err != nil {
			recordError(w, err)
			return
		}

		// Authorization before any further work, per 005-runtime-lifecycle.md "Security Ordering"
		// and 007 §20: mch_approval_step declares prm_decide_own_step, so only the step's own
		// fld_assignee gets past here (ROADMAP.md Phase 16).
		actor, _ := authorization.CurrentUserID(req, cfg.SessionSecret)
		if !authorization.AllowsAction(machine, domain.ActionDecide, step.Values, actor) {
			http.Error(w, "this approval step is assigned to someone else", http.StatusForbidden)
			return
		}

		documentID, _ := step.Values[action.FieldStepDocument].(string)
		document, err := store.GetRecord(ctx, action.DocumentMachineID, documentID)
		if err != nil {
			recordError(w, err)
			return
		}
		siblings, err := store.ListRecordsBy(ctx, machine.ID, action.FieldStepDocument, documentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		mode, _ := document.Values[action.FieldDocumentMode].(string)
		if !action.CanDecide(mode, step, siblings) {
			http.Error(w, "an earlier step has not been decided yet", http.StatusUnprocessableEntity)
			return
		}

		step.Values[action.FieldStepDecision] = decision
		if _, err := store.UpdateRecord(ctx, machine.ID, id, step.Values); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		updatedSiblings, err := store.ListRecordsBy(ctx, machine.ID, action.FieldStepDocument, documentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		document.Values[action.FieldDocumentStatus] = action.DocumentStatus(updatedSiblings)
		if _, err := store.UpdateRecord(ctx, action.DocumentMachineID, documentID, document.Values); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		logActivity(ctx, store, action.DocumentMachineID, documentID, actor, fmt.Sprintf("Step %v %s", toDisplayString(step.Values[action.FieldStepSequence]), decision))

		documentURL := "/machines/" + action.DocumentMachineID + "/records/" + documentID
		if req.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", documentURL)
			return
		}
		http.Redirect(w, req, documentURL, http.StatusSeeOther)
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

// maxUploadBytes bounds one multipart request body (ROADMAP.md Phase 11) -- generous enough for
// a real PDF or a handful of images, small enough that a malicious upload can't exhaust disk.
const maxUploadBytes = 20 << 20 // 20MB

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

// logActivity appends one mch_activity record (ROADMAP.md Phase 13) -- an ordinary Machine, not
// a new system-data-source concept (007 SS4.1's admission question). Best-effort: a logging
// failure is not allowed to fail the real operation it's describing, only get logged itself.
func logActivity(ctx context.Context, store *data.Store, machineID, recordID, actorID, summary string) {
	values := map[string]any{
		"fld_machine_id": machineID,
		"fld_record_id":  recordID,
		"fld_summary":    summary,
	}
	if actorID != "" {
		values["fld_actor"] = actorID
	}
	if _, err := store.CreateRecord(ctx, "mch_activity", values); err != nil {
		log.Printf("failed to log activity (%s %s): %v", machineID, recordID, err)
	}
}

// serveUpload streams a previously uploaded file back. Gated by requireAuth like every other
// route in its group -- there is no per-record ownership check yet (ROADMAP.md Phase 2's
// Machine+Action permission granularity doesn't extend to individual files), matching the rest
// of the app's current authorization boundary.
func serveUpload(files *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key := chi.URLParam(req, "*")
		path, err := files.Path(key)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename=%q`, storage.DisplayName(key)))
		http.ServeFile(w, req, path)
	}
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
