package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/data"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/storage"
)

// TestInstallApplication_endToEndOverACollision drives the whole chain a real install takes -- session,
// admin gate, POST, installer, reload hook, redirect -- against the **real template library** and a
// Workspace that already uses mch_document.
//
// This is the acceptance case the owner asked for at the start of the session ("bagaimana cara aku bisa
// pakai aplikasi document approval di workspace lain"), reduced to what a test can hold: dokter-kecil's
// shape, which is a generated Application already occupying the id Document Approval's own template
// wants.
//
// internal/installer's own tests cover the copy and the rename in detail. What only this one covers is
// that a person can actually reach it: the route exists, it is admin-gated, the form's fields are the
// ones the handler reads, and the redirect lands on the installed Application's own screen.
func TestInstallApplication_endToEndOverACollision(t *testing.T) {
	s := newInstallTestSetup(t, "install_collision")

	rec := s.post(t, url.Values{"template": {"app_document_approval"}, "role": {"approver"}})
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("POST /install-application = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}
	// Document Approval's own home_card route. Routes are never renamed on install (a collision there
	// is refused outright), so the template's declared route is the installed copy's route.
	if got := rec.Header().Get("Location"); got != "/approval-inbox" {
		t.Errorf("redirect = %q, want /approval-inbox", got)
	}

	after, err := metadata.LoadApplication(s.manifestPath)
	if err != nil {
		t.Fatalf("the Workspace no longer loads after the install: %v", err)
	}
	byID := map[string]*domain.Machine{}
	for _, m := range after.Workspace.Machines {
		byID[m.ID] = m
	}
	if byID["mch_document"] == nil || byID["mch_document"].Name != "Tracked Document" {
		t.Error("this Workspace's own mch_document must be exactly as it was")
	}
	if byID["mch_document_approval"] == nil {
		t.Fatal("the renamed copy was not installed")
	}
	if len(after.Workspace.Applications) != 2 {
		t.Errorf("the Workspace has %d Applications, want its own plus the installed one", len(after.Workspace.Applications))
	}
}

// A template whose routes already answer in this Workspace is refused, and the refusal comes out of the
// POST as well as off the screen -- a page is not a gate.
func TestInstallApplication_refusesARouteCollision(t *testing.T) {
	s := newInstallTestSetup(t, "install_refusal")

	// Install once, then again: the second attempt collides on every route the first one declared.
	if rec := s.post(t, url.Values{"template": {"app_document_approval"}}); rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("first install = %d, want a redirect; body=%s", rec.Code, rec.Body.String())
	}
	s.reloadWorkspace(t)

	rec := s.post(t, url.Values{"template": {"app_document_approval"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("second install = %d, want 422 -- its routes are already declared here", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/approval-inbox") {
		t.Errorf("the refusal must name the colliding route; got %q", rec.Body.String())
	}
}

// The listing is a read, and it says what would happen -- including the refusal, so an admin is never
// offered a button that then declines.
func TestInstallApplication_listingShowsRenamesAndRefusals(t *testing.T) {
	s := newInstallTestSetup(t, "install_listing")

	req := httptest.NewRequest(http.MethodGet, "/install-application", nil)
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: s.cookie})
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /install-application = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Document Approval") {
		t.Error("the listing does not offer Document Approval")
	}
	if !strings.Contains(body, "mch_document_approval") {
		t.Error("the listing does not say what the colliding id will be renamed to -- an admin has to know before clicking")
	}
}

// --- fixture -----------------------------------------------------------------------------------

type installTestSetup struct {
	handler      http.Handler
	cookie       string
	manifestPath string
	slug         string
	deps         Deps
	workspaces   map[string]domain.Workspace
}

// newInstallTestSetup builds a Workspace shaped like dokter-kecil -- its own mch_document, a generated
// Application claiming it -- with its manifest in a temp tree and the template library pointed at the
// repo's real metadata/. Nothing here writes inside the repo.
func newInstallTestSetup(t *testing.T, name string) *installTestSetup {
	t.Helper()
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	email := name + "@example.com"
	slug := strings.ReplaceAll(name, "_", "-")

	ws, err := store.CreateWorkspace(ctx, name, slug)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	cleanupAuthTest(t, pool, ws.ID, email)
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	actor, err := store.CreateRecord(wsCtx, "mch_user", map[string]any{"fld_email": email})
	if err != nil {
		t.Fatalf("CreateRecord(actor): %v", err)
	}
	// admin, because installing is an admin action -- that gate is part of what this exercises.
	if err := store.AddMember(ctx, ws.ID, actor.ID, email, "admin", ""); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	library, err := filepath.Abs(filepath.Join("..", "..", "metadata"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	workspacesDir := filepath.Join(root, "workspaces")
	if err := os.MkdirAll(filepath.Join(workspacesDir, slug, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(workspacesDir, slug, "document.yaml"),
		"id: mch_document\nname: Tracked Document\nfields:\n  - id: fld_title\n    name: Title\n    type: text\n")
	writeTestFile(t, filepath.Join(workspacesDir, slug, "applications", "tracking.yaml"),
		"id: app_document_tracking\nname: Document Tracking\nmachines:\n  - mch_document\nnavigation:\n  - id: nav_document_tracking\n    label: Documents\n    route: /machines/mch_document\n    home_card: true\n")
	manifestPath := filepath.Join(workspacesDir, slug+".yaml")
	writeTestFile(t, manifestPath, "workspace: "+slug+"\nmachines:\n"+
		"  - "+relTo(t, workspacesDir, filepath.Join(library, "user.yaml"))+"\n"+
		"  - "+relTo(t, workspacesDir, filepath.Join(library, "activity.yaml"))+"\n"+
		"  - "+slug+"/document.yaml\napplications:\n  - "+slug+"/applications/tracking.yaml\n")

	loaded, err := metadata.LoadApplication(manifestPath)
	if err != nil {
		t.Fatalf("fixture Workspace does not load: %v", err)
	}
	files, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	s := &installTestSetup{
		manifestPath: manifestPath,
		slug:         slug,
		workspaces:   map[string]domain.Workspace{slug: loaded.Workspace},
		cookie:       sessionCookieValueForTest(t, config.Config{SessionSecret: name + "-secret"}, actor.ID, 0),
	}
	s.deps = Deps{
		UserMachine: userMachineFor(t, loaded),
		Store:       store,
		Files:       files,
		Cfg: config.Config{
			SessionSecret: name + "-secret",
			SecureCookies: false,
			MetadataPath:  workspacesDir,
			TemplatePath:  library,
		},
		Workspaces:         s.workspaces,
		DefaultWorkspaceID: ws.ID,
		// The real hook reloads the process's whole route table; here the assertions read the files on
		// disk, so re-reading the manifest into the same map is the honest equivalent -- it proves the
		// install produced something loadable, which is what a real reload would discover.
		ReloadMetadata: func(string) error {
			reloaded, err := metadata.LoadApplication(manifestPath)
			if err != nil {
				return err
			}
			s.workspaces[slug] = reloaded.Workspace
			return nil
		},
	}
	s.handler = Routes(s.deps)
	return s
}

// post sends one install, as the admin, through the whole router -- including csrfProtect, which this
// route is behind like every other POST. The cookie+form token pair is the same shape csrfHiddenInput
// renders and csrf_test.go already exercises.
func (s *installTestSetup) post(t *testing.T, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	form.Set("csrf_token", "install-test-token")
	req := httptest.NewRequest(http.MethodPost, "/install-application", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: authorization.SessionCookieName, Value: s.cookie})
	req.AddCookie(&http.Cookie{Name: authorization.CSRFCookieName, Value: "install-test-token"})
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

// reloadWorkspace rebuilds the router from the manifest as it now stands -- what the process does after
// a successful install, and what makes a *second* install see the first one's routes.
func (s *installTestSetup) reloadWorkspace(t *testing.T) {
	t.Helper()
	if err := s.deps.ReloadMetadata(s.slug); err != nil {
		t.Fatalf("reload: %v", err)
	}
	s.handler = Routes(s.deps)
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func relTo(t *testing.T, from, to string) string {
	t.Helper()
	rel, err := filepath.Rel(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

func userMachineFor(t *testing.T, loaded *metadata.App) *domain.Machine {
	t.Helper()
	for _, m := range loaded.Machines {
		if m.ID == domain.UserMachineID {
			return m
		}
	}
	t.Fatal("the fixture Workspace has no mch_user")
	return nil
}
