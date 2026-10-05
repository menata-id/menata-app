package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"menata.app/internal/aiassist"
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
	// UserMachine is mch_user, and it is deliberately the *only* Machine on Deps.
	//
	// Deps carried the whole process-wide Machine set here until 2026-09-27 (an id-keyed map plus
	// a declaration-ordered list, unioned across every installed Workspace by cmd/server and
	// deduped by id). That made "which Machine does id X mean" a process-level answer, when it is
	// a per-Workspace one: two Workspaces could not hold different definitions under one id, the
	// second silently getting the first's. A request's Machines now come from the Workspace
	// already on its ctx -- machinesFor/installedMachines (machine.go) -- which is the same
	// per-request discipline routeByID/labelByID already follow.
	//
	// mch_user survives as an explicit exception because it genuinely is cross-Workspace (owner,
	// 2026-09-27: "hanya user aja yang bisa lintas workspace"): memberships point at user records,
	// so one identity has one of these in every Workspace it belongs to. The three handlers that
	// need it cannot ask a ctx Workspace for it anyway -- /register and /accept-invite run before
	// any session exists, and /create-workspace is making the Workspace whose record it is about
	// to write. Naming the one shared Machine is honest; holding all of them was not.
	UserMachine *domain.Machine

	Store  *data.Store
	Files  *storage.Store
	Mailer mail.Mailer
	Cfg    config.Config

	// AIClient is the AI Metadata Assistant's own Gemini conversation client (Flow 2 gap study
	// Tahap 8) -- aiassist.UnconfiguredClient when GEMINI_API_KEY is unset, matching Mailer's own
	// injected-interface shape.
	AIClient aiassist.Client

	// Workspace is the whole loaded Workspace -- its own navigation and every Application
	// declared inside it (004 §Navigation Metadata, 006 §Navigation). Routes hands it to
	// internal/rendering once, at startup, rather than threading it through every handler and
	// Page function (development-history.md Phase 21 round 2 Step J already chose the equivalent trade-off
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
	// (development-history.md Phase 21 Step 4).
	DefaultWorkspaceID string

	// ReloadMetadata rebuilds this whole route table from metadata on disk and swaps it in
	// atomically -- the AI Metadata Assistant's own publish handler (internal/web/newapplication.go,
	// Flow 2 gap study Tahap 8) calls it after writing a purely-additive change, so the new
	// Application is reachable with no process restart.
	//
	// A plain func() error, not a call into internal/metadata directly: this package is forbidden
	// from importing internal/metadata (internal/conformance.TestPlaneBoundaries -- "the transport
	// layer adapts HTTP to the planes; it holds no pool and loads no Runtime Metadata... the
	// composition root builds both once at startup"). cmd/server builds the real closure (it already
	// owns metadata.LoadWorkspaces and web.Routes) and hands it in here, the same injection shape
	// Mailer already uses for a capability this package must invoke but not implement. Nil in any
	// test/fixture Deps that never exercises the AI assistant's publish path.
	ReloadMetadata func() error
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
	ownedRoutes := ownedApplicationRoutes(d)
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
	// Content hashes for the assets a page names, computed once here so the URL a page emits and
	// the file served below can never disagree (assets.go).
	installAssetFingerprints()

	r.Group(func(sr chi.Router) {
		sr.Use(middleware.Compress(5))
		sr.Handle("/icons/*", staticAssets("icons", "static/icons"))
		// The Tailwind build (static/css/app.css, `make css`). Public rather than inside
		// requireAuth because the pre-auth pages -- sign in, register, password reset -- render
		// from it too, and a stylesheet behind an auth gate would leave the sign-in page unstyled
		// for exactly the people who cannot be authenticated yet.
		sr.Handle("/css/*", staticAssets("css", "static/css"))
		// Vendored htmx/hyperscript (pageHead's own doc comment) -- self-hosted rather than loaded
		// from unpkg.com so a CDN outage or block can't silently take down every hx-* interaction.
		// Also serves the Ubuntu woff2 faces app.css names (static/vendor/fonts/ubuntu), which is
		// why they live under vendor/ rather than needing a public route of their own.
		sr.Handle("/vendor/*", staticAssets("vendor", "static/vendor"))
	})
	r.Get("/login", showLogin)
	r.Post("/login", rateLimitLogin(loginLimiter, submitLogin(d.Store, d.Cfg)))
	r.Get("/register", showRegistration)
	r.Post("/register", rateLimitByAddress(registrationLimiter, "too many registration attempts -- try again later", submitRegistration(d.UserMachine, d.Store, d.Mailer, d.Cfg)))
	r.Get("/verify-email", showVerifyEmail(d.Store, d.Cfg))
	r.Get("/resend-verification", showResendVerification)
	r.Post("/resend-verification", submitResendVerification(d.Store, d.Mailer, d.Cfg))
	r.Get("/forgot-password", showForgotPassword)
	r.Post("/forgot-password", rateLimitByAddress(forgotPasswordLimiter, "too many attempts -- try again later", submitForgotPassword(d.Store, d.Mailer, d.Cfg)))
	r.Get("/reset-password", showResetPassword)
	r.Post("/reset-password", submitResetPassword(d.Store, d.Cfg))
	r.Get("/accept-invite", showAcceptInvite(d.Store, d.Cfg))
	r.Post("/accept-invite", rateLimitByAddress(inviteAcceptLimiter, "too many attempts -- try again later", submitAcceptInvite(d.UserMachine, d.Store, d.Cfg)))
	r.Get("/choose-workspace", showChooseWorkspace(d.Store, d.Cfg))
	r.Post("/choose-workspace", submitChooseWorkspace(d.Store, d.Cfg))
	// Restore (Flow 2 gap study Tahap 7) is reachable pre-session, the same way Choose Workspace
	// itself already is: the pending-email cookie names who is acting, not an authenticated
	// session, since a Restore admin may be signing in via the Choose Workspace path rather than
	// switching mid-session (submitSwitchWorkspace's /switch-workspace/restore counterpart below).
	r.Post("/choose-workspace/restore", submitRestoreWorkspaceFromChoose(d.Store, d.Cfg))

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
		// Member deactivation (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27) -- immediately
		// after resolveIdentity, for the same zero-extra-query reason blockWritesToArchivedWorkspace
		// sits where it does, and before every route below so a deactivated member reaches none of
		// them, reads included.
		pr.Use(requireActiveMembership(d.Store, d.Cfg))
		// Workspace first, then Application: currentApplication reads the Workspace this
		// resolves, and requireApplicationAccess reads the Application that resolves.
		pr.Use(currentWorkspace(d.Store, d.Workspaces))
		pr.Use(currentApplication())
		pr.Use(requireInstalledApplication(ownedRoutes))
		// Immediately after, and never before: it reads the Application currentApplication just
		// resolved. Every route inside an Application passes through both, which is what makes
		// "no role here means no access" complete rather than a list of gated handlers.
		pr.Use(requireApplicationAccess(d.Store, d.Cfg))
		// Tahap 7's own gate (Flow 2 gap study) -- one method-based rule above every write route in
		// this whole group, not a per-Machine Permission. Needs currentWorkspaceRow, which
		// resolveIdentity above already resolved, so this costs no extra query.
		pr.Use(blockWritesToArchivedWorkspace(d.Store))

		pr.Post("/logout", logout(d.Store, d.Cfg))

		pr.Get("/api/machines", listMachines())
		pr.Get("/api/machines/{machineID}/records", listRecords(d.Store))
		pr.Post("/api/machines/{machineID}/records", createRecord(d.Store, d.Files, d.Mailer, d.Cfg))
		pr.Put("/api/machines/{machineID}/records/{id}", updateRecord(d.Store, d.Files, d.Mailer, d.Cfg))
		pr.Delete("/api/machines/{machineID}/records/{id}", deleteRecordAPI(d.Store, d.Cfg))

		pr.Get("/", showMachineList(d.Store, d.Cfg))
		pr.Get("/home", showWorkspaceHome(d.Store, d.Cfg))
		pr.Get("/switch-workspace", showSwitchWorkspace(d.Store, d.Cfg))
		pr.Post("/switch-workspace", submitSwitchWorkspace(d.Store, d.Cfg))
		// Restore's mid-session counterpart -- allow-listed in blockWritesToArchivedWorkspace since
		// it is the one write an archived Workspace must accept (see that gate's own doc comment).
		pr.Post("/switch-workspace/restore", submitRestoreWorkspaceFromSwitch(d.Store, d.Cfg))
		pr.Get("/create-workspace", showCreateWorkspace(d.Store, d.Cfg))
		pr.Post("/create-workspace", submitCreateWorkspace(d.UserMachine, d.Store, d.Cfg))
		pr.Get("/account-profile", showProfile(d.Store, d.Cfg))
		pr.Post("/account-profile", submitProfile(d.Store, d.Cfg))
		pr.Get("/account-notifications", showAccountNotifications(d.Store, d.Cfg))
		pr.Post("/account-notifications", submitAccountNotifications(d.Store, d.Cfg))
		pr.Get("/account-security", showSecurity(d.Store, d.Cfg))
		pr.Post("/account-security/change-password", submitChangePassword(d.Store, d.Cfg))
		pr.Post("/account-security/sign-out-other-devices", submitSignOutOtherDevices(d.Store, d.Cfg))
		// Account menu's own "Workspaces" section (Flow 2 canvas re-audit -- ROADMAP.md,
		// 2026-09-27), fetched lazily by accountMenu's own hx-get, not on every appShell render.
		pr.Get("/api/account-menu/workspaces", showAccountMenuWorkspaces(d.Store, d.Cfg))
		pr.Get("/dashboard", showDashboard(d.Store, d.Cfg))
		pr.Get("/my-tasks", showMyTasks(d.Store, d.Cfg))
		pr.Get("/board-settings", showBoardSettings(d.Store, d.Cfg))
		// Notifications (Flow 2 gap study Tahap 6) -- Workspace-level runtime routes, same category
		// as /dashboard and /account-profile: reachable by any authenticated member regardless of
		// Application access, since a notification can concern any Application's own Machine.
		pr.Get("/notifications", showNotifications(d.Store, d.Cfg))
		pr.Post("/notifications/mark-all-read", submitMarkAllNotificationsRead(d.Store, d.Cfg))
		pr.Get("/api/notifications/unread-count", showUnreadNotificationCount(d.Store, d.Cfg))
		pr.Get("/calendar", showCalendar(d.Store, d.Cfg))
		pr.Get("/approval-inbox", showApprovalInbox(d.Store, d.Cfg))
		pr.Get("/api/approval-inbox/pending-count", showPendingCount(d.Store, d.Cfg))
		pr.Get("/documents/new", showDocumentSubmit(d.Store, d.Cfg))
		pr.Get("/documents/new/approver-row", newApproverRow(d.Store))
		// CAP-V28 (ROADMAP.md, 2026-09-27): the doc_type <select>'s own htmx fragment, loading that
		// Document Type's saved default approval flow, if one exists.
		pr.Get("/documents/new/approval-flow-template", showApprovalFlowTemplateRows(d.Store))
		pr.Post("/documents", submitDocumentWizard(d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/continue-submit", showDocumentContinue(d.Store, d.Cfg))
		pr.Post("/machines/{machineID}/records/{id}/continue-submit", continueDocumentWizard(d.Store, d.Files, d.Cfg))
		pr.Post("/machines/{machineID}/records/{id}/revise", reviseDocument(d.Store, d.Cfg))
		// nav_app_settings / nav_app_settings_permissions (metadata/applications/document-
		// approval.yaml) -- the Application Settings hub and its Permissions sub-page
		// (ROADMAP.md "Application Settings hub"), one handler factory for both. Gated by
		// requireApplicationAccess above like every other Document Approval route, not
		// requireWorkspaceAdmin: the page is read-only information for any member.
		pr.Get("/document-approval/settings", showApplicationSettings("", d.Store, d.Cfg))
		pr.Get("/document-approval/settings/permissions", showApplicationSettings("permissions", d.Store, d.Cfg))
		pr.Get("/machines/{machineID}", showMachinePage(d.Store, d.Cfg))
		pr.Post("/machines/{machineID}/records", createRecordForm(d.Store, d.Files, d.Mailer, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}", showRecordRow(d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/edit", editRecordRow(d.Store, d.Cfg))
		pr.Put("/machines/{machineID}/records/{id}", updateRecordForm(d.Store, d.Files, d.Mailer, d.Cfg))
		pr.Patch("/machines/{machineID}/records/{id}", patchRecordForm(d.Store, d.Files, d.Mailer, d.Cfg))
		pr.Delete("/machines/{machineID}/records/{id}", deleteRecord(d.Store, d.Cfg))
		pr.Post("/machines/{machineID}/records/{id}/decide", decideStep(d.Store, d.Files, d.Mailer, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/review", showReviewDocument(d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/signature-placement", showSignaturePlacement(d.Store, d.Files, d.Cfg))
		// The write half, on its own route rather than the generic record one: this screen asks
		// "may you place this signature", which is a different question from "may you edit this
		// step" -- see composition.MayPlaceSignature.
		pr.Put("/machines/{machineID}/records/{id}/signature-placement", updateSignaturePlacement(d.Store, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/pdf-preview", servePDFPreview(d.Store, d.Files))

		pr.Group(func(ar chi.Router) {
			ar.Use(requireWorkspaceAdmin(d.Store, d.Cfg))
			// nav_workspace_settings (Flow 2 gap study Tahap 5) -- the Workspace-level Settings
			// hub, same admin gate as everything else in this group.
			ar.Get("/workspace-settings", showWorkspaceSettings(d.Store, d.Cfg))
			// Danger zone's Archive action (Flow 2 gap study Tahap 7) -- taken from inside the
			// Workspace being archived, same admin gate as the hub itself.
			ar.Post("/workspace-settings/archive", submitArchiveWorkspace(d.Store, d.Cfg))
			ar.Get("/workspace-members", showWorkspaceMembers(d.Store, d.Cfg))
			ar.Post("/workspace-members/invite", submitInviteMember(d.Store, d.Mailer, d.Cfg))
			ar.Post("/workspace-members/revoke-invite", submitRevokeInvite(d.Store))
			ar.Get("/workspace-members/{userRecordID}/edit", showEditMember(d.Store, d.Cfg))
			ar.Post("/workspace-members/{userRecordID}/edit", submitEditMember(d.Store))
			// Member deactivation (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27).
			ar.Post("/workspace-members/{userRecordID}/deactivate", submitDeactivateMember(d.Store))
			ar.Post("/workspace-members/{userRecordID}/reactivate", submitReactivateMember(d.Store))

			// Groups (Case 03 Fase 4) -- membership administration, so the same requireWorkspaceAdmin
			// gate as the member routes above.
			ar.Get("/workspace-groups", showGroups(d.Store, d.Cfg))
			ar.Post("/workspace-groups", submitCreateGroup(d.Store))
			ar.Get("/workspace-groups/{groupID}", showGroupDetail(d.Store, d.Cfg))
			ar.Post("/workspace-groups/{groupID}/members", submitGroupMembers(d.Store))
			ar.Post("/workspace-groups/{groupID}/roles", submitGroupRoles(d.Store))
			ar.Post("/workspace-groups/{groupID}/delete", submitDeleteGroup(d.Store))

			// AI Metadata Assistant (Flow 2 gap study Tahap 8) -- "Only workspace admins can add
			// applications" (the mockup's own words, M03b-WorkspaceMenu.dc.html), same gate as
			// everything else in this group. Entry point itself is hidden from navigation when
			// d.Cfg.GeminiAPIKey is unset (internal/rendering's own showNewApplicationEntry).
			ar.Get("/new-application", showNewApplication(d.Store, d.AIClient, d.Cfg))
			ar.Post("/new-application/message", postNewApplicationMessage(d.Store, d.AIClient, d.Cfg))
			ar.Get("/new-application/{session}/review", showNewApplicationReview(d.Store, d.Cfg))
			ar.Post("/new-application/{session}/publish", publishNewApplication(d.Store, d.AIClient, d.Cfg, d.ReloadMetadata))
			ar.Post("/new-application/{session}/discard", discardNewApplication(d.Store))
			// Workspace Home's own "draft Application" row, fetched lazily (Flow 2 canvas
			// re-audit, ROADMAP.md, 2026-09-27) -- same admin gate as the section that triggers it.
			ar.Get("/api/home/draft-applications", showHomeDraftApplications(d.Store))

			// Installing a ready-made Application from the template library (2026-09-28) -- the
			// non-AI half of "add an application", and the first thing that can install
			// metadata/applications/*.yaml at all. Same admin gate for the same stated reason:
			// only workspace admins add applications. POST for the write, so the listing stays a
			// read (TestGetRoutesDoNotWrite).
			ar.Get("/install-application", showInstallApplication(d.Store, d.Cfg))
			ar.Post("/install-application", submitInstallApplication(d.Store, d.Cfg, d.ReloadMetadata))

			// What the runtime inferred (2026-09-29): 001 #6's second clause, "Inference must be
			// inspectable". Admin-gated for the same stated reason as the two lines above -- it
			// describes how this Workspace is assembled. A GET with no path parameter, so
			// TestNoGetRouteRepeatsAReadOrLeavesOneUnnamed sweeps it automatically; it issues no query
			// of its own, which is the whole of its 007 §33 fan-out answer.
			ar.Get("/inference", showInference(d.Store, d.Cfg))
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
