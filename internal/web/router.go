package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
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
	Cfg     config.Config
	AppName string
}

// Routes builds the application's complete route table.
//
// /health and /login sit outside the authenticated group deliberately: a liveness probe carries
// no session, and requiring one to reach the login form would be a redirect loop.
func Routes(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/login", showLogin)
	r.Post("/login", submitLogin(d.Cfg))

	r.Group(func(pr chi.Router) {
		pr.Use(requireAuth(d.Cfg))
		pr.Use(queryDiagnostics)

		pr.Post("/logout", logout(d.Cfg))

		pr.Get("/api/machines", listMachines(d.MachineList))
		pr.Get("/api/machines/{machineID}/records", listRecords(d.Store))
		pr.Post("/api/machines/{machineID}/records", createRecord(d.Machines, d.Store))

		pr.Get("/", showMachineList(d.MachineList, d.AppName))
		pr.Get("/dashboard", showDashboard(d.Machines, d.Store, d.AppName))
		pr.Get("/my-tasks", showMyTasks(d.Machines, d.Store, d.AppName, d.Cfg))
		pr.Get("/board-settings", showBoardSettings(d.Store, d.AppName))
		pr.Get("/activity", showActivity(d.Machines, d.Store, d.AppName))
		pr.Get("/team-capacity", showTeamCapacity(d.Machines, d.Store, d.AppName))
		pr.Get("/automation", showAutomation(d.MachineList, d.AppName))
		pr.Get("/calendar", showCalendar(d.Machines, d.Store, d.AppName))
		pr.Get("/sprint", showSprintDashboard(d.Machines, d.Store, d.AppName))
		pr.Get("/approval-inbox", showApprovalInbox(d.Machines, d.Store, d.AppName, d.Cfg))
		pr.Get("/documents/new", showDocumentSubmit(d.Store, d.AppName))
		pr.Get("/documents/new/approver-row", newApproverRow(d.Store))
		pr.Post("/documents", submitDocumentWizard(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}", showMachinePage(d.Machines, d.AppName, d.Store))
		pr.Post("/machines/{machineID}/records", createRecordForm(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}", showRecordRow(d.Machines, d.Store, d.AppName, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/edit", editRecordRow(d.Machines, d.Store))
		pr.Put("/machines/{machineID}/records/{id}", updateRecordForm(d.Machines, d.Store, d.Files, d.Cfg))
		pr.Delete("/machines/{machineID}/records/{id}", deleteRecord(d.Machines, d.Store))
		pr.Post("/machines/{machineID}/records/{id}/decide", decideStep(d.Machines, d.Store, d.Cfg))
		pr.Get("/machines/{machineID}/records/{id}/signature-placement", showSignaturePlacement(d.Machines, d.Store, d.Files, d.AppName))
		pr.Get("/machines/{machineID}/records/{id}/pdf-preview", servePDFPreview(d.Machines, d.Store, d.Files))

		pr.Get("/uploads/*", serveUpload(d.Files))

		// Static design references (ROADMAP.md's own case-portfolio.md mockups) -- ui-sample/ is
		// a design reference, never current-code intent (per this repo's own convention), served
		// as-is with no rendering logic of its own so it's easy to compare against the real pages
		// above.
		pr.Handle("/ui-sample/*", http.StripPrefix("/ui-sample/", http.FileServer(http.Dir("ui-sample"))))
	})

	return r
}
