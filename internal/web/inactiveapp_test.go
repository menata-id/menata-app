package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"menata.app/internal/config"
	"menata.app/internal/domain"
	"menata.app/internal/metadata"
	"menata.app/internal/rendering"
)

// An Application switched off (`inactive: true` in its file) is read as inactive by the loader, answers 404 on every route it
// owns -- a Machine route and a navigation route alike -- and drops out of the menu, while the Application beside it is
// untouched and its Machines stay in the Workspace.
func TestInactiveApplicationIsGoneFromEveryRouteAndTheMenuButItsDataStays(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "workspaces")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "user.yaml"), "id: mch_user\nname: User\nfields:\n  - id: fld_name\n    name: Name\n    type: text\n")
	for _, m := range []string{"a", "b"} {
		write(filepath.Join(dir, "w", m+".yaml"), "id: mch_"+m+"\nname: M"+m+"\nfields:\n  - id: fld_t\n    name: T\n    type: text\n")
	}
	app := func(id, machine, extra string) string {
		return "id: app_" + id + "\nname: App " + id + "\nmachines: [" + machine + "]\n" + extra + "navigation:\n  - id: nav_" + id +
			"\n    label: " + id + "\n    route: /machines/" + machine + "\n"
	}
	write(filepath.Join(dir, "w", "applications", "a.yaml"), app("a", "mch_a", "inactive: true\n"))
	write(filepath.Join(dir, "w", "applications", "b.yaml"), app("b", "mch_b", ""))
	write(filepath.Join(dir, "w.yaml"), "workspace: w\nmachines:\n  - ../user.yaml\n  - w/a.yaml\n  - w/b.yaml\napplications:\n  - w/applications/a.yaml\n  - w/applications/b.yaml\n")

	loaded, err := metadata.LoadApplication(filepath.Join(dir, "w.yaml"))
	if err != nil {
		t.Fatalf("does not load: %v", err)
	}
	ws := loaded.Workspace
	byID := map[string]domain.Application{}
	for _, a := range ws.Applications {
		byID[a.ID] = a
	}
	if !byID["app_a"].Inactive || byID["app_b"].Inactive {
		t.Fatalf("inactive flags: a=%v b=%v, want true and false", byID["app_a"].Inactive, byID["app_b"].Inactive)
	}

	serve := func(path string) int {
		h := currentApplication()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(rendering.WithCurrentWorkspace(req.Context(), ws, "W", false))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for path, want := range map[string]int{
		"/machines/mch_a":              http.StatusNotFound, // the switched-off Application's Machine
		"/machines/mch_a/records/x":    http.StatusNotFound,
		"/api/machines/mch_a/records":  http.StatusNotFound,
		"/machines/mch_b":              http.StatusOK, // its neighbour is untouched
		"/machines/mch_user/records/x": http.StatusOK, // a shared Machine belongs to no Application
	} {
		if got := serve(path); got != want {
			t.Errorf("GET %s = %d, want %d", path, got, want)
		}
	}
	admin := domain.Actor{WorkspaceRole: domain.WorkspaceRoleAdmin}
	for _, a := range openableApplications(ws, admin, config.Config{}) {
		if a.ID == "app_a" {
			t.Error("the switched-off Application is still offered in the menu")
		}
	}
	if len(ws.Machines) < 3 {
		t.Errorf("the Workspace holds %d Machines; switching an Application off must not remove its Machines", len(ws.Machines))
	}
}
