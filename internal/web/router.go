package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
	"menata.app/internal/rendering"
	"menata.app/internal/storage"
)

// Deps is everything the handlers need, built once by the composition root and never rebuilt per
// request. Passing one struct rather than five parameters is not only brevity: it means adding a
// dependency does not touch the signature of every handler that does not use it, which is how a
// route table this size stays reviewable.
type Deps struct {
	// Machines is the id-keyed lookup; MachineList preserves the declaration order metadata was
	// written in, which the Machine list page and the automation page both render in that order.
	Machines    map[string]*domain.Machine
	MachineList []*domain.Machine

	Store   *data.Store
	Files   *storage.Store
	Mailer  mail.Mailer
	Cfg     config.Config
	AppName string

	// Navigation is the Application's own declared menu (004 §Navigation Metadata, 006
	// §Navigation) -- Routes hands it to internal/rendering once, at startup, rather than
	// threading it through every handler and Page function the way AppName would otherwise need
	// to be (ROADMAP.md Phase 21 round 2 Step J already chose the equivalent trade-off for the
	// pending-approval badge).
	Navigation []domain.NavigationItem
	// PrimaryNavGroup is domain.Application.PrimaryNavGroup -- see its own doc comment for why
	// this travels alongside Navigation instead of being derived from it at render time.
	PrimaryNavGroup string
	// HomeRoute is domain.Application.HomeRoute, showWorkspaceHome's own source for its
	// Application card's link -- decided from the full declared navigation before
	// hidden_nav_groups runs, for the same reason PrimaryNavGroup is (see its own doc comment on
	// domain.Application): deriving it from Navigation here instead would silently blank it out
	// whenever the HomeCard item's own group is hidden.
	HomeRoute string
	// AllNavigation is domain.Application.AllNavigation -- routeByID's own source
	// (internal/rendering), for the same reason HomeRoute needs the pre-filter list: a
	// contextual link to a hidden group's item must still resolve.
	AllNavigation []domain.NavigationItem

	// DefaultWorkspaceID is this manifest's own declared Workspace (metadata/app.yaml) --
	// requireAuth's fallback Workspace for a session whose subject isn't a real mch_user record id
	// (ROADMAP.md Phase 21 Step 4).
	DefaultWorkspaceID string
}

// Routes builds the application's complete route table.
//
// /health and /login sit outside the authenticated group deliberately: a liveness probe carries
// no session, and requiring one to reach the login form would be a redirect loop.
// loginAttemptLimit and loginAttemptWindow bound POST /login (ROADMAP.md Operational backlog:
// "unlimited password guesses are possible today"), now genuinely worth enforcing since real
// per-user passwords exist to guess (Phase 21). Generous enough that a person mistyping their own
// password a few times in a row is never affected.
const (
	loginAttemptLimit  = 10
	loginAttemptWindow = 5 * time.Minute

	// registrationAttemptLimit/Window bound POST /register (address-only key -- see
	// rateLimitByAddress) -- a real Workspace-creation flow is now worth defending against
	// scripted spam the same way login already is.
	registrationAttemptLimit  = 10
	registrationAttemptWindow = 5 * time.Minute

	// forgotPasswordAttemptLimit/Window bound POST /forgot-password (address-only key, same
	// reasoning as registration) -- prevents both spamming reset emails and probing which
	// addresses have accounts via response timing.
	forgotPasswordAttemptLimit  = 10
	forgotPasswordAttemptWindow = 5 * time.Minute
)

func Routes(d Deps) http.Handler {
	rendering.ConfigureNavigation(d.Navigation, d.PrimaryNavGroup, d.AllNavigation)

	r := chi.NewRouter()
	r.Use(secureHeaders)
	loginLimiter := newLoginRateLimiter(loginAttemptLimit, loginAttemptWindow)
	registrationLimiter := newLoginRateLimiter(registrationAttemptLimit, registrationAttemptWindow)
	forgotPasswordLimiter := newLoginRateLimiter(forgotPasswordAttemptLimit, forgotPasswordAttemptWindow)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok")) // liveness probe body; a failed write here isn't actionable
	})
	r.Get("/manifest.json", serveManifest)
	r.Get("/sw.js", serveServiceWorker)
	r.Handle("/icons/*", http.StripPrefix("/icons/", http.FileServer(http.Dir("static/icons"))))
	r.Get("/login", showLogin)
	r.Post("/login", rateLimitLogin(loginLimiter, submitLogin(d.Store, d.Cfg)))
	r.Get("/register", showRegistration)
	r.Post("/register", rateLimitByAddress(registrationLimiter, "too many registration attempts -- try again later", submitRegistration(d.Machines, d.Store, d.Mailer, d.Cfg)))
	r.Get("/verify-email", showVerifyEmail(d.Store, d.Cfg))
	r.Get("/resend-verification", showResendVerification)
	r.Post("/resend-verification", submitResendVerification(d.Store, d.Mailer, d.Cfg))
	r.Get("/forgot-password", showForgotPassword)
	r.Post("/forgot-password", rateLimitByAddress(forgotPasswordLimiter, "too many attempts -- try again later", submitForgotPassword(d.Store, d.Mailer, d.Cfg)))
	r.Get("/reset-password", showResetPassword)
	r.Post("/reset-password", submitResetPassword(d.Store, d.Cfg))
	r.Get("/accept-invite", showAcceptInvite)
	r.Post("/accept-invite", submitAcceptInvite(d.Store, d.Cfg))
	r.Get("/choose-workspace", showChooseWorkspace(d.Store, d.Cfg))
	r.Post("/choose-workspace", submitChooseWorkspace(d.Store, d.Cfg))

	r.Group(func(pr chi.Router) {
		pr.Use(requireAuth(d.Store, d.DefaultWorkspaceID, d.Cfg))
		pr.Use(queryDiagnostics)

		pr.Post("/logout", logout(d.Store, d.Cfg))

		pr.Get("/api/machines", listMachines(d.MachineList))
		pr.Get("/api/machines/{machineID}/records", listRecords(d.Store))
		pr.Post("/api/machines/{machineID}/records", createRecord(d.Machines, d.Store, d.Cfg))
		pr.Put("/api/machines/{machineID}/records/{id}", updateRecord(d.Machines, d.Store, d.Cfg))
		pr.Delete("/api/machines/{machineID}/records/{id}", deleteRecordAPI(d.Machines, d.Store))

		pr.Get("/", showMachineList(d.MachineList, d.AppName))
		pr.Get("/home", showWorkspaceHome(d.Machines, d.Store, d.AppName, d.HomeRoute, d.Cfg))
		pr.Get("/switch-workspace", showSwitchWorkspace(d.Store, d.Cfg))
		pr.Post("/switch-workspace", submitSwitchWorkspace(d.Store, d.Cfg))
		pr.Get("/dashboard", showDashboard(d.Machines, d.Store, d.AppName))
		pr.Get("/my-tasks", showMyTasks(d.Machines, d.Store, d.AppName, d.Cfg))
		pr.Get("/board-settings", showBoardSettings(d.Store, d.AppName))
		pr.Get("/activity", showActivity(d.Machines, d.Store, d.AppName))
		pr.Get("/team-capacity", showTeamCapacity(d.Machines, d.Store, d.AppName))
		pr.Get("/automation", showAutomation(d.MachineList, d.AppName))
		pr.Get("/calendar", showCalendar(d.Machines, d.Store, d.AppName))
		pr.Get("/sprint", showSprintDashboard(d.Machines, d.Store, d.AppName))
		pr.Get("/approval-inbox", showApprovalInbox(d.Machines, d.Store, d.AppName, d.Cfg))
		pr.Get("/api/approval-inbox/pending-count", showPendingCount(d.Machines, d.Store, d.Cfg))
		pr.Get("/documents/new", showDocumentSubmit(d.Store, d.Machines["mch_document"], d.AppName))
		pr.Get("/documents/new/approver-row", newApproverRow(d.Store))
		pr.Post("/documents", submitDocumentWizard(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}", showMachinePage(d.Machines, d.AppName, d.Store))
		pr.Post("/machines/{machineID}/records", createRecordForm(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}", showRecordRow(d.Machines, d.Store, d.AppName, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/edit", editRecordRow(d.Machines, d.Store))
		pr.Put("/machines/{machineID}/records/{id}", updateRecordForm(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Delete("/machines/{machineID}/records/{id}", deleteRecord(d.Machines, d.Store))
		pr.Post("/machines/{machineID}/records/{id}/decide", decideStep(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/signature-placement", showSignaturePlacement(d.Machines, d.Store, d.Files, d.AppName))
		pr.Get("/machines/{machineID}/records/{id}/pdf-preview", servePDFPreview(d.Machines, d.Store, d.Files))

		pr.Group(func(ar chi.Router) {
			ar.Use(requireWorkspaceAdmin(d.Store, d.Cfg))
			ar.Get("/workspace-members", showWorkspaceMembers(d.Store, d.AppName))
			ar.Post("/workspace-members/invite", submitInviteMember(d.Machines, d.Store, d.Mailer, d.Cfg))
			ar.Get("/workspace-members/{userRecordID}/edit", showEditMember(d.Store, d.AppName))
			ar.Post("/workspace-members/{userRecordID}/edit", submitEditMember(d.Store))
		})

		pr.Get("/uploads/*", serveUpload(d.Store, d.Files))

		// Static design references (ROADMAP.md's own case-portfolio.md mockups) -- ui-sample/ is
		// a design reference, never current-code intent (per this repo's own convention), served
		// as-is with no rendering logic of its own so it's easy to compare against the real pages
		// above.
		pr.Handle("/ui-sample/*", http.StripPrefix("/ui-sample/", http.FileServer(http.Dir("ui-sample"))))
	})

	return r
}
