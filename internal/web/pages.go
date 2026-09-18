package web

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/experience"
	"menata.app/internal/rendering"
)

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

// showSprintDashboard is Case 19's analytics view (ROADMAP.md Phase 14, project-dashboard.html):
// a real Task-status summary, a workload preview (reusing MemberCapacity from Team Capacity), and
// an Attention Needed list of overdue/due-today Tasks (reusing My Tasks' own SLA bucketing). The
// mockup's points/burndown/blocked content is deliberately not reproduced -- see
// rendering.SprintSummary's own doc comment for why.
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

// showCalendar is Case 19's week-grid Layout (ROADMAP.md Phase 14, project-calendar.html): every
// mch_task whose fld_due_date falls in the current Monday-Sunday week, one column per day. Pure
// composition over the same Task/Project data My Tasks already loads -- fld_due_date already
// exists, no new Field or Layout mechanism needed.
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

// showTeamCapacity is Case 19's Team Capacity screen (ROADMAP.md Phase 14, project-team.html):
// every mch_user with their declared weekly capacity (a new Number field on an existing Machine,
// not a new mechanism) and how many mch_task are currently assigned to them, still open.
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

// showAutomation is Case 19's Workflow Automation screen (ROADMAP.md Phase 14,
// project-automation.html): a read-only Trigger/Condition/Action description of this
// Application's real Constraint metadata and Action behavior -- not a generic automation engine
// (no forcing case has built one) and not fictional example workflows.
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

func findMachine(machines []*domain.Machine, id string) *domain.Machine {
	for _, m := range machines {
		if m.ID == id {
			return m
		}
	}
	return nil
}
