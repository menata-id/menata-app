package web

import (
	"net/http"
	"time"

	"menata.app/internal/authorization"
	"menata.app/internal/composition"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// The screens in this file follow one shape: resolve whatever the request carries, ask
// internal/composition for the page's content, render it. The joins, rollups and SLA bucketing
// they used to perform inline now live beside their own tests (ROADMAP.md Phase 19 Step 3).

// dashboardActivityLimit is how many recent events the dashboard's tail shows; /activity has its
// own, larger limit because a dedicated feed can afford more than a summary card can.
const (
	dashboardActivityLimit = 10
	activityFeedLimit      = 50
)

// showDashboard combines Project and Task data on one page -- ROADMAP.md Phase 6's own forcing
// case, exercised here for real.
func showDashboard(machines map[string]*domain.Machine, store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		d, err := composition.DashboardData(ctx, composition.NewLoader(store, machines), dashboardActivityLimit)
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.DashboardPage(d.Projects, d.Documents, d.Pending, d.Activity, appName))
	}
}

// showMyTasks is Case 19's personal work queue (ROADMAP.md Phase 14). "Assigned to me" resolves
// to authorization.CurrentUserID -- the same shared-admin-credential-to-real-mch_user resolution
// Phase 8 already built, not a new per-user login mechanism.
func showMyTasks(machines map[string]*domain.Machine, store *data.Store, appName string, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		userID, _ := authorization.CurrentUserID(req, cfg.SessionSecret)

		t, err := composition.PersonalTasks(ctx, composition.NewLoader(store, machines), userID, time.Now())
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.MyTasksPage(t.Summary, t.Today, t.Upcoming, t.Completed, appName))
	}
}

// showActivity is Case 19's cross-project event feed (ROADMAP.md Phase 14,
// project-activity.html): the same mch_activity data as the Dashboard's Recent Activity section,
// grouped by day (Today/Yesterday/Earlier) instead of a flat top-10 list.
func showActivity(machines map[string]*domain.Machine, store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		feed, err := composition.GroupedActivity(ctx, composition.NewLoader(store, machines), activityFeedLimit, time.Now())
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.ActivityPage(feed.Today, feed.Yesterday, feed.Earlier, appName))
	}
}

// showSprintDashboard is Case 19's analytics view (ROADMAP.md Phase 14, project-dashboard.html):
// a real Task-status summary, a workload preview (reusing MemberCapacity from Team Capacity), and
// an Attention Needed list of overdue/due-today Tasks (reusing My Tasks' own SLA bucketing).
func showSprintDashboard(machines map[string]*domain.Machine, store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		s, err := composition.SprintDashboard(ctx, composition.NewLoader(store, machines), time.Now())
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.SprintDashboardPage(s.Summary, s.Workload, s.Attention, appName))
	}
}

// showCalendar is Case 19's week-grid Layout (ROADMAP.md Phase 14, project-calendar.html): every
// mch_task whose fld_due_date falls in the current Monday-Sunday week, one column per day.
func showCalendar(machines map[string]*domain.Machine, store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		days, err := composition.CalendarWeek(ctx, composition.NewLoader(store, machines), time.Now())
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.CalendarPage(days, appName))
	}
}

// showTeamCapacity is Case 19's Team Capacity screen (ROADMAP.md Phase 14, project-team.html):
// every mch_user with their declared weekly capacity (a new Number field on an existing Machine,
// not a new mechanism) and how many mch_task are currently assigned to them, still open.
func showTeamCapacity(machines map[string]*domain.Machine, store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		c, err := composition.TeamCapacity(ctx, composition.NewLoader(store, machines))
		if err != nil {
			serverError(w, err)
			return
		}
		render(ctx, w, rendering.TeamCapacityPage(c.Members, c.TotalCapacity, c.TotalActive, appName))
	}
}

// showBoardSettings is the Lists/Labels catalog hub. It renders two Machines' records as-is, with
// nothing to derive, which is why it reads them directly rather than through a composition step.
func showBoardSettings(store *data.Store, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		lists, err := store.ListRecords(ctx, "mch_list")
		if err != nil {
			serverError(w, err)
			return
		}
		labels, err := store.ListRecords(ctx, "mch_label")
		if err != nil {
			serverError(w, err)
			return
		}

		render(ctx, w, rendering.BoardSettingsPage(lists, labels, appName))
	}
}

// showAutomation is Case 19's Workflow Automation screen (ROADMAP.md Phase 14,
// project-automation.html): a read-only Trigger/Condition/Action description of this
// Application's real Constraint metadata and Action behavior.
func showAutomation(machines []*domain.Machine, appName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		render(req.Context(), w, rendering.AutomationPage(composition.AutomationRules(machines), appName))
	}
}
