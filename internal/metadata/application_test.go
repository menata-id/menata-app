package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestLoadApplication_valid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
fields:
  - id: fld_title
    name: Title
    type: text
    required: true
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
application:
  id: app_task_tracker
  name: Task Tracker
  machines:
    - task.yaml
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	if app.Workspace.ID != "ws_default" {
		t.Errorf("Workspace.ID = %q, want ws_default", app.Workspace.ID)
	}
	if app.Application.WorkspaceID != "ws_default" {
		t.Errorf("Application.WorkspaceID = %q, want ws_default", app.Application.WorkspaceID)
	}
	if len(app.Machines) != 1 || app.Machines[0].ID != "mch_task" {
		t.Fatalf("Machines = %+v, want one machine mch_task", app.Machines)
	}
}

func TestLoadApplication_badWorkspaceID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: default
  name: Default Workspace
application:
  id: app_task_tracker
  name: Task Tracker
  machines:
    - task.yaml
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error for malformed workspace id")
	}
}

func TestLoadApplication_noMachines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
application:
  id: app_task_tracker
  name: Task Tracker
  machines: []
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error for zero machines")
	}
}
