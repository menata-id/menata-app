package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/mail"
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

	Store  *data.Store
	Files  *storage.Store
	Mailer mail.Mailer
	Cfg    config.Config

	// Workspace is the whole loaded Workspace -- its own navigation and every Application
	// declared inside it (004 §Navigation Metadata, 006 §Navigation). Routes hands it to
	// internal/rendering once, at startup, rather than threading it through every handler and
	// Page function (ROADMAP.md Phase 21 round 2 Step J already chose the equivalent trade-off
	// for the pending-approval badge).
	//
	// It replaces the four single-Application fields that used to live here (AppName, Navigation,
	// PrimaryNavGroup, HomeRoute, AllNavigation): with several Applications none of them has one
	// value for the process any more. Which Application a given *request* is in is resolved per
	// request by the currentApplication middleware and read back through
	// rendering.CurrentApplication.
	Workspaces map[string]domain.Workspace

	// DefaultWorkspaceID is this manifest's own declared Workspace (the default Workspace row) --
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

	// inviteAcceptAttemptLimit/Window bound POST /accept-invite (address-only key -- see
	// rateLimitByAddress; unlike /login, a valid invite token is a prerequisite to reach the
	// existing-credential password check at all, so the exposure this defends is smaller, but the
	// check itself is still a password guess worth slowing down, security audit 2026-09-19's H1
	// follow-up).
	inviteAcceptAttemptLimit  = 10
	inviteAcceptAttemptWindow = 5 * time.Minute
)

func Routes(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(secureHeaders)
	r.Use(csrfProtect(d.Cfg))
	loginLimiter := newLoginRateLimiter(loginAttemptLimit, loginAttemptWindow)
	registrationLimiter := newLoginRateLimiter(registrationAttemptLimit, registrationAttemptWindow)
	forgotPasswordLimiter := newLoginRateLimiter(forgotPasswordAttemptLimit, forgotPasswordAttemptWindow)
	inviteAcceptLimiter := newLoginRateLimiter(inviteAcceptAttemptLimit, inviteAcceptAttemptWindow)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok")) // liveness probe body; a failed write here isn't actionable
	})
	r.Get("/manifest.json", serveManifest)
	r.Get("/sw.js", serveServiceWorker)
	// STATIC FILES, AND THE ONE PLACE THIS APP COMPRESSES.
	//
	// Measured 2026-09-22: hyperscript.min.js 369kB -> 67kB, htmx.min.js 51kB -> 16kB,
	// app.css 30kB -> 6.5kB. About 359kB off a cold load, which was the largest single
	// perceived-speed item in the performance study and cost nothing architectural.
	//
	// **HTML is deliberately not compressed**, and the omission is the decision rather than an
	// oversight (owner, 2026-09-22). Every page carries the CSRF token in its own markup
	// (`hx-headers` on <body>, rendering's csrfHeadersAttr, from the 2026-09-19 security audit's
	// L1), and a secret inside a compressed response alongside content an attacker can influence
	// -- a record title someone else wrote, rendered on the page -- is the BREACH precondition.
	// Compressing HTML is worth 2.6kB per page against that; the static assets above are ~99% of
	// the win and carry no secret at all, so the question does not have to be answered to collect
	// it. Revisit alongside per-response CSRF token masking, not before.
	//
	// chi's default compressible content types cover text/css and application/javascript and do
	// not cover font/woff2, so the Ubuntu faces under /vendor/fonts are not re-compressed for
	// nothing. No custom type list is needed.
	r.Group(func(sr chi.Router) {
		sr.Use(middleware.Compress(5))
		sr.Handle("/icons/*", http.StripPrefix("/icons/", http.FileServer(http.Dir("static/icons"))))
		// The Tailwind build (static/css/app.css, `make css`). Public rather than inside
		// requireAuth because the pre-auth pages -- sign in, register, password reset -- render
		// from it too, and a stylesheet behind an auth gate would leave the sign-in page unstyled
		// for exactly the people who cannot be authenticated yet.
		sr.Handle("/css/*", http.StripPrefix("/css/", http.FileServer(http.Dir("static/css"))))
		// Vendored htmx/hyperscript (pageHead's own doc comment) -- self-hosted rather than loaded
		// from unpkg.com so a CDN outage or block can't silently take down every hx-* interaction.
		// Also serves the Ubuntu woff2 faces app.css names (static/vendor/fonts/ubuntu), which is
		// why they live under vendor/ rather than needing a public route of their own.
		sr.Handle("/vendor/*", http.StripPrefix("/vendor/", http.FileServer(http.Dir("static/vendor"))))
	})
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
	r.Get("/accept-invite", showAcceptInvite(d.Store, d.Cfg))
	r.Post("/accept-invite", rateLimitByAddress(inviteAcceptLimiter, "too many attempts -- try again later", submitAcceptInvite(d.Machines, d.Store, d.Cfg)))
	r.Get("/choose-workspace", showChooseWorkspace(d.Store, d.Cfg))
	r.Post("/choose-workspace", submitChooseWorkspace(d.Store, d.Cfg))

	r.Group(func(pr chi.Router) {
		// First in the group, ahead of requireAuth: the diagnostic can only count what happens
		// after it installs the ReadLog on ctx, and requireAuth issues two queries of its own. It
		// sat *below* all four of these until 2026-09-22, which is why the log under-reported
		// every authenticated request by the whole cost of its own middleware.
		pr.Use(queryDiagnostics)
		pr.Use(requireAuth(d.Store, d.DefaultWorkspaceID, d.Cfg))
		// Immediately after requireAuth, which establishes the Workspace scope it reads, and
		// before the three below, all of which consume what it resolves rather than querying
		// again. It is the one place this request's Workspace row, membership and Actor are read.
		pr.Use(resolveIdentity(d.Store, d.Cfg))
		// Workspace first, then Application: currentApplication reads the Workspace this
		// resolves, and requireApplicationAccess reads the Application that resolves.
		pr.Use(currentWorkspace(d.Store, d.Workspaces))
		pr.Use(currentApplication())
		// Immediately after, and never before: it reads the Application currentApplication just
		// resolved. Every route inside an Application passes through both, which is what makes
		// "no role here means no access" complete rather than a list of gated handlers.
		pr.Use(requireApplicationAccess(d.Store, d.Cfg))

		pr.Post("/logout", logout(d.Store, d.Cfg))

		pr.Get("/api/machines", listMachines(d.MachineList))
		pr.Get("/api/machines/{machineID}/records", listRecords(d.Store))
		pr.Post("/api/machines/{machineID}/records", createRecord(d.Machines, d.Store, d.Cfg))
		pr.Put("/api/machines/{machineID}/records/{id}", updateRecord(d.Machines, d.Store, d.Cfg))
		pr.Delete("/api/machines/{machineID}/records/{id}", deleteRecordAPI(d.Machines, d.Store, d.Cfg))

		pr.Get("/", showMachineList(d.MachineList))
		pr.Get("/home", showWorkspaceHome(d.Machines, d.Store, d.Cfg))
		pr.Get("/switch-workspace", showSwitchWorkspace(d.Store, d.Cfg))
		pr.Post("/switch-workspace", submitSwitchWorkspace(d.Store, d.Cfg))
		pr.Get("/create-workspace", showCreateWorkspace(d.Store, d.Cfg))
		pr.Post("/create-workspace", submitCreateWorkspace(d.Machines, d.Store, d.Cfg))
		pr.Get("/account-profile", showProfile(d.Store, d.Cfg))
		pr.Post("/account-profile", submitProfile(d.Store, d.Cfg))
		pr.Get("/account-security", showSecurity(d.Store, d.Cfg))
		pr.Post("/account-security/change-password", submitChangePassword(d.Store, d.Cfg))
		pr.Post("/account-security/sign-out-other-devices", submitSignOutOtherDevices(d.Store, d.Cfg))
		pr.Get("/dashboard", showDashboard(d.Machines, d.Store, d.Cfg))
		pr.Get("/my-tasks", showMyTasks(d.Machines, d.Store, d.Cfg))
		pr.Get("/board-settings", showBoardSettings(d.Store, d.Cfg))
		pr.Get("/activity", showActivity(d.Machines, d.Store, d.Cfg))
		pr.Get("/team-capacity", showTeamCapacity(d.Machines, d.Store, d.Cfg))
		pr.Get("/automation", showAutomation(d.MachineList, d.Store, d.Cfg))
		pr.Get("/calendar", showCalendar(d.Machines, d.Store, d.Cfg))
		pr.Get("/sprint", showSprintDashboard(d.Machines, d.Store, d.Cfg))
		pr.Get("/approval-inbox", showApprovalInbox(d.Machines, d.Store, d.Cfg))
		pr.Get("/api/approval-inbox/pending-count", showPendingCount(d.Machines, d.Store, d.Cfg))
		pr.Get("/documents/new", showDocumentSubmit(d.Machines, d.Store, d.Cfg))
		pr.Get("/documents/new/approver-row", newApproverRow(d.Machines, d.Store))
		pr.Post("/documents", submitDocumentWizard(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}", showMachinePage(d.Machines, d.Store, d.Cfg))
		pr.Post("/machines/{machineID}/records", createRecordForm(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}", showRecordRow(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/edit", editRecordRow(d.Machines, d.Store, d.Cfg))
		pr.Put("/machines/{machineID}/records/{id}", updateRecordForm(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Delete("/machines/{machineID}/records/{id}", deleteRecord(d.Machines, d.Store, d.Cfg))
		pr.Post("/machines/{machineID}/records/{id}/decide", decideStep(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/review", showReviewDocument(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/signature-placement", showSignaturePlacement(d.Machines, d.Store, d.Files, d.Cfg))
		// The write half, on its own route rather than the generic record one: this screen asks
		// "may you place this signature", which is a different question from "may you edit this
		// step" -- see composition.MayPlaceSignature.
		pr.Put("/machines/{machineID}/records/{id}/signature-placement", updateSignaturePlacement(d.Machines, d.Store, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/pdf-preview", servePDFPreview(d.Machines, d.Store, d.Files))

		pr.Group(func(ar chi.Router) {
			ar.Use(requireWorkspaceAdmin(d.Store, d.Cfg))
			ar.Get("/workspace-members", showWorkspaceMembers(d.Store, d.Cfg))
			ar.Post("/workspace-members/invite", submitInviteMember(d.Store, d.Mailer, d.Cfg))
			ar.Post("/workspace-members/revoke-invite", submitRevokeInvite(d.Store))
			ar.Get("/workspace-members/{userRecordID}/edit", showEditMember(d.Store, d.Cfg))
			ar.Post("/workspace-members/{userRecordID}/edit", submitEditMember(d.Store))

			// Groups (Case 03 Fase 4) -- membership administration, so the same requireWorkspaceAdmin
			// gate as the member routes above.
			// Board 06 (Case 03 Fase 7) -- the Workspace's access model read role-first, so the
			// same administration gate as Members and Groups above.
			ar.Get("/authorization-matrix", showRoleMatrix(d.MachineList, d.Store, d.Cfg))

			ar.Get("/workspace-groups", showGroups(d.Store, d.Cfg))
			ar.Post("/workspace-groups", submitCreateGroup(d.Store))
			ar.Get("/workspace-groups/{groupID}", showGroupDetail(d.Store, d.Cfg))
			ar.Post("/workspace-groups/{groupID}/members", submitGroupMembers(d.Store))
			ar.Post("/workspace-groups/{groupID}/roles", submitGroupRoles(d.Store))
			ar.Post("/workspace-groups/{groupID}/delete", submitDeleteGroup(d.Store))
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
