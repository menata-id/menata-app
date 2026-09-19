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

func TestLoadApplication_relationTargetExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.yaml", `
id: mch_project
name: Project
fields:
  - id: fld_name
    name: Name
    type: text
    required: true
`)
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
fields:
  - id: fld_project
    name: Project
    type: relation
    machine: mch_project
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
application:
  id: app_task_tracker
  name: Task Tracker
  machines:
    - project.yaml
    - task.yaml
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	if len(app.Machines) != 2 {
		t.Fatalf("Machines = %+v, want 2", app.Machines)
	}
}

func TestLoadApplication_relationTargetMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
fields:
  - id: fld_project
    name: Project
    type: relation
    machine: mch_project
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

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: fld_project targets mch_project, which isn't in this application")
	}
}

func TestLoadApplication_personFieldRequiresUserMachine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
fields:
  - id: fld_assignee
    name: Assignee
    type: person
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

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: fld_assignee implicitly targets mch_user, which isn't in this application")
	}
}

func TestLoadApplication_personFieldWithUserMachine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "user.yaml", `
id: mch_user
name: User
fields:
  - id: fld_name
    name: Name
    type: text
`)
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
fields:
  - id: fld_assignee
    name: Assignee
    type: person
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
application:
  id: app_task_tracker
  name: Task Tracker
  machines:
    - user.yaml
    - task.yaml
`)

	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
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

func TestLoadApplication_constraintValid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
fields:
  - id: fld_project
    name: Project
    type: relation
    machine: mch_project
  - id: fld_status
    name: Status
    type: status
    options: [todo, done]
`)
	writeFile(t, dir, "project.yaml", `
id: mch_project
name: Project
fields:
  - id: fld_status
    name: Status
    type: status
    options: [planning, done]
constraints:
  - id: cst_project_done_no_open_tasks
    on: fld_status
    when_equals: done
    block_if:
      related_machine: mch_task
      related_field: fld_project
      condition:
        field: fld_status
        op: not_equals
        value: done
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
application:
  id: app_task_tracker
  name: Task Tracker
  machines:
    - project.yaml
    - task.yaml
`)

	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
}

func TestLoadApplication_constraintRelatedFieldNotARelationBack(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
fields:
  - id: fld_status
    name: Status
    type: status
    options: [todo, done]
`)
	writeFile(t, dir, "project.yaml", `
id: mch_project
name: Project
fields:
  - id: fld_status
    name: Status
    type: status
    options: [planning, done]
constraints:
  - id: cst_project_done_no_open_tasks
    on: fld_status
    when_equals: done
    block_if:
      related_machine: mch_task
      related_field: fld_status
      condition:
        field: fld_status
        op: not_equals
        value: done
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
application:
  id: app_task_tracker
  name: Task Tracker
  machines:
    - project.yaml
    - task.yaml
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: fld_status on mch_task is not a relation field pointing back to mch_project")
	}
}

func TestLoadApplication_navigation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
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
  navigation:
    - id: nav_home
      label: Home
      route: /home
    - id: nav_inbox
      label: Approval Inbox
      route: /approval-inbox
      group: Document Approval
      priority: 1
      badge: approval_inbox_pending
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	if len(app.Application.Navigation) != 2 {
		t.Fatalf("Navigation = %+v, want 2 items", app.Application.Navigation)
	}
	inbox := app.Application.Navigation[1]
	if inbox.Group != "Document Approval" || inbox.Badge != "approval_inbox_pending" {
		t.Errorf("Navigation[1] = %+v, want Group=Document Approval Badge=approval_inbox_pending", inbox)
	}
}

func TestLoadApplication_navigationBadRouteRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
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
  navigation:
    - id: nav_home
      label: Home
      route: home
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: route must start with \"/\"")
	}
}

func TestLoadApplication_navigationUnknownBadgeRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
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
  navigation:
    - id: nav_home
      label: Home
      route: /home
      badge: made_up_badge
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: unknown badge")
	}
}

func TestLoadApplication_navigationDuplicateIDRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
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
  navigation:
    - id: nav_home
      label: Home
      route: /home
    - id: nav_home
      label: Home Again
      route: /home2
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: duplicate navigation item id")
	}
}

func TestLoadApplication_hiddenNavGroupDropsItsItems(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
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
  navigation:
    - id: nav_home
      label: Home
      route: /home
    - id: nav_inbox
      label: Approval Inbox
      route: /approval-inbox
      group: Document Approval
      priority: 1
    - id: nav_board
      label: Board
      route: /board
      group: Project Management
      priority: 1
  hidden_nav_groups:
    - Document Approval
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	if len(app.Application.Navigation) != 2 {
		t.Fatalf("Navigation = %+v, want 2 items (Document Approval's item dropped)", app.Application.Navigation)
	}
	for _, n := range app.Application.Navigation {
		if n.Group == "Document Approval" {
			t.Errorf("Navigation still contains a Document Approval item: %+v", n)
		}
	}
}

func TestLoadApplication_hiddenNavGroupUnknownRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
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
  navigation:
    - id: nav_home
      label: Home
      route: /home
  hidden_nav_groups:
    - Typo Group
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: hidden_nav_groups entry does not match any navigation item's group")
	}
}
