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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	if app.Workspace.ID != "ws_default" {
		t.Errorf("Workspace.ID = %q, want ws_default", app.Workspace.ID)
	}
	if app.Workspace.Applications[0].WorkspaceID != "ws_default" {
		t.Errorf("Application.WorkspaceID = %q, want ws_default", app.Workspace.Applications[0].WorkspaceID)
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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
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
  machines:
    - project.yaml
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_project
  - mch_task
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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
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
  machines:
    - user.yaml
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_user
  - mch_task
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
  machines: []
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
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
  machines:
    - project.yaml
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_project
  - mch_task
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
  machines:
    - project.yaml
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_project
  - mch_task
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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
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
	if len(app.Workspace.Applications[0].Navigation) != 2 {
		t.Fatalf("Navigation = %+v, want 2 items", app.Workspace.Applications[0].Navigation)
	}
	inbox := app.Workspace.Applications[0].Navigation[1]
	if inbox.Group != "Document Approval" || inbox.Badge != "approval_inbox_pending" {
		t.Errorf("Navigation[1] = %+v, want Group=Document Approval Badge=approval_inbox_pending", inbox)
	}
}

func TestLoadApplication_homeCardRoute(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
navigation:
  - id: nav_home
    label: Home
    route: /home
  - id: nav_inbox
    label: Approval Inbox
    route: /approval-inbox
    group: Document Approval
    priority: 1
    home_card: true
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	if app.Workspace.Applications[0].HomeRoute != "/approval-inbox" {
		t.Errorf("HomeRoute = %q, want /approval-inbox", app.Workspace.Applications[0].HomeRoute)
	}
}

func TestLoadApplication_homeCardRouteSurvivesShowNavFalse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
navigation:
  - id: nav_home
    label: Home
    route: /home
  - id: nav_inbox
    label: Approval Inbox
    route: /approval-inbox
    group: Document Approval
    priority: 1
    home_card: true
show_nav: false
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	// Navigation is empty (show_nav: false), but HomeRoute was frozen from the full declared list
	// first -- rendering.WorkspaceHomePage's card for this Application must keep working even
	// though the Application renders no menu at all.
	if len(app.Workspace.Applications[0].Navigation) != 0 {
		t.Errorf("Navigation = %+v, want empty under show_nav: false", app.Workspace.Applications[0].Navigation)
	}
	if app.Workspace.Applications[0].HomeRoute != "/approval-inbox" {
		t.Errorf("HomeRoute = %q, want /approval-inbox (frozen before show_nav emptied Navigation)", app.Workspace.Applications[0].HomeRoute)
	}
	// AllNavigation still has nav_inbox even though Navigation doesn't -- internal/rendering's
	// routeByID needs the full list to resolve a contextual link into a menu-less Application
	// (approvalinbox.templ's own "+ New Approval" -> nav_new_approval is the real case this
	// protects).
	found := false
	for _, n := range app.Workspace.Applications[0].AllNavigation {
		if n.ID == "nav_inbox" {
			found = true
		}
	}
	if !found {
		t.Errorf("AllNavigation = %+v, want it to still contain nav_inbox despite show_nav: false", app.Workspace.Applications[0].AllNavigation)
	}
	if len(app.Workspace.Applications[0].AllNavigation) != 2 {
		t.Errorf("AllNavigation = %+v, want 2 items (unfiltered)", app.Workspace.Applications[0].AllNavigation)
	}
}

func TestLoadApplication_duplicateHomeCardRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
navigation:
  - id: nav_inbox
    label: Approval Inbox
    route: /approval-inbox
    home_card: true
  - id: nav_board
    label: Board
    route: /board
    home_card: true
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: at most one navigation item may set home_card")
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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
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
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_task_tracker
name: Task Tracker
machines:
  - mch_task
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

func TestLoadApplication_showNavFalseHidesMenuButKeepsEverythingResolvable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_document_approval
name: Document Approval
show_nav: false
machines:
  - mch_task
navigation:
  - id: nav_inbox
    label: Approval Inbox
    route: /approval-inbox
    home_card: true
  - id: nav_new
    label: New Approval
    route: /documents/new
    priority: 1
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	got := app.Workspace.Applications[0]

	if got.ShowNav {
		t.Error("ShowNav = true, want false")
	}
	if len(got.Navigation) != 0 {
		t.Errorf("Navigation = %+v, want empty -- show_nav: false renders no menu chrome at all", got.Navigation)
	}

	// Everything below is the freeze-then-filter property: suppressing the menu must not make the
	// Application's own screens unreachable or unresolvable. routeByID/labelByID resolve against
	// AllNavigation, and both metadata-hardcoding conformance gates depend on it.
	if len(got.AllNavigation) != 2 {
		t.Errorf("AllNavigation = %+v, want both declared items (unfiltered)", got.AllNavigation)
	}
	if got.HomeRoute != "/approval-inbox" {
		t.Errorf("HomeRoute = %q, want /approval-inbox -- decided before show_nav emptied Navigation", got.HomeRoute)
	}
}

// TestLoadApplication_showNavDefaultsToTrue guards the one place a bool default would silently do
// the wrong thing: an Application that says nothing about its menu keeps it. That is why
// applicationDoc.ShowNav is a *bool -- a plain bool defaults to false and would suppress every
// Application's menu the moment the key was omitted.
func TestLoadApplication_showNavDefaultsToTrue(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_project_management
name: Project Management
machines:
  - mch_task
navigation:
  - id: nav_board
    label: Board
    route: /board
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	got := app.Workspace.Applications[0]
	if !got.ShowNav {
		t.Error("ShowNav = false with no show_nav declared, want true -- an Application that says nothing keeps its menu")
	}
	if len(got.Navigation) != 1 {
		t.Errorf("Navigation = %+v, want the declared item", got.Navigation)
	}
}

// TestLoadApplication_machineClaimedByTwoApplicationsRejected guards what makes
// domain.Workspace.ApplicationForMachine unambiguous -- the primary half of resolving which
// Application a request is in, for the many routes no navigation item names. Two claimants would
// make that answer depend on declaration order.
func TestLoadApplication_machineClaimedByTwoApplicationsRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-one.yaml
  - app-two.yaml
`)
	writeFile(t, dir, "app-one.yaml", `
id: app_one
name: One
machines:
  - mch_task
`)
	writeFile(t, dir, "app-two.yaml", `
id: app_two
name: Two
machines:
  - mch_task
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: mch_task claimed by two applications")
	}
}

// TestLoadApplication_unknownMachineClaimRejected: an Application selects Machines by id from the
// Workspace's own set, so naming one the Workspace never declares is a typo worth failing on
// rather than an empty selection.
func TestLoadApplication_unknownMachineClaimRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_main
name: Main
machines:
  - mch_nope
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: mch_nope is not a machine this workspace declares")
	}
}

// TestLoadApplication_duplicateNavigationIDAcrossApplicationsRejected: routeByID/labelByID resolve
// an id against every declared list workspace-wide, so a duplicate across two Applications would
// silently resolve to whichever loaded first -- pointing a page at another Application's screen.
func TestLoadApplication_duplicateNavigationIDAcrossApplicationsRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "project.yaml", `
id: mch_project
name: Project
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
    - project.yaml
applications:
  - app-one.yaml
  - app-two.yaml
`)
	writeFile(t, dir, "app-one.yaml", `
id: app_one
name: One
machines:
  - mch_task
navigation:
  - id: nav_shared
    label: One
    route: /one
`)
	writeFile(t, dir, "app-two.yaml", `
id: app_two
name: Two
machines:
  - mch_project
navigation:
  - id: nav_shared
    label: Two
    route: /two
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want error: nav_shared declared by two applications")
	}
}

// TestLoadApplication_rolesOptional covers Project Management's real case: an Application that
// declares no role vocabulary loads fine and yields an empty list, so the Members screens simply
// offer it no role rather than borrowing another Application's words or rendering an empty
// dropdown. Asserted rather than assumed, since "declares none" is the default state and a
// default that silently broke would be easy to miss.
func TestLoadApplication_rolesOptional(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "project.yaml", `
id: mch_project
name: Project
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
    - project.yaml
applications:
  - with-roles.yaml
  - without-roles.yaml
`)
	writeFile(t, dir, "with-roles.yaml", `
id: app_with_roles
name: With Roles
machines:
  - mch_task
roles:
  - approver
  - submitter
`)
	writeFile(t, dir, "without-roles.yaml", `
id: app_without_roles
name: Without Roles
machines:
  - mch_project
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	with, _ := app.Workspace.ApplicationByID("app_with_roles")
	if len(with.Roles) != 2 || with.Roles[0] != "approver" {
		t.Errorf("Roles = %+v, want [approver submitter]", with.Roles)
	}
	without, _ := app.Workspace.ApplicationByID("app_without_roles")
	if len(without.Roles) != 0 {
		t.Errorf("Roles = %+v, want none declared", without.Roles)
	}
}

func TestLoadApplication_duplicateRoleRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_main
name: Main
machines:
  - mch_task
roles:
  - approver
  - approver
`)

	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err == nil {
		t.Fatal("LoadApplication() error = nil, want error: role declared twice")
	}
}

func TestLoadApplication_blankRoleRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_main
name: Main
machines:
  - mch_task
roles:
  - approver
  - ""
`)

	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err == nil {
		t.Fatal("LoadApplication() error = nil, want error: blank role -- 'no role' is already the absence of one")
	}
}

func TestLoadApplication_unknownColorRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", "\nid: mch_task\nname: Task\n")
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_main
name: Main
machines:
  - mch_task
color: chartreuse
`)

	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err == nil {
		t.Fatal("LoadApplication() error = nil, want error: color outside the closed set would render nothing")
	}
}

// TestLoadApplication_summaryMachineMustBeOwn is the check that keeps a card's number honest: an
// Application reporting a Machine it does not claim would put another Application's count on its
// own card.
func TestLoadApplication_summaryMachineMustBeOwn(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", "\nid: mch_task\nname: Task\n")
	writeFile(t, dir, "project.yaml", "\nid: mch_project\nname: Project\n")
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
    - project.yaml
applications:
  - app-one.yaml
  - app-two.yaml
`)
	writeFile(t, dir, "app-one.yaml", `
id: app_one
name: One
machines:
  - mch_task
summary_machine: mch_project
`)
	writeFile(t, dir, "app-two.yaml", `
id: app_two
name: Two
machines:
  - mch_project
`)

	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err == nil {
		t.Fatal("LoadApplication() error = nil, want error: app_one reports mch_project, which app_two owns")
	}
}

// TestLoadApplication_cardFaceOptional: an Application declaring none of the card-face keys loads
// fine and renders plainly, so the default state is covered rather than assumed.
func TestLoadApplication_cardFaceOptional(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", "\nid: mch_task\nname: Task\n")
	writeFile(t, dir, "app.yaml", `
workspace:
  id: ws_default
  name: Default Workspace
  machines:
    - task.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_main
name: Main
machines:
  - mch_task
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	got := app.Workspace.Applications[0]
	if got.Description != "" || got.Icon != "" || got.Color != "" || got.SummaryMachine != "" {
		t.Errorf("card face = %+v, want all empty when nothing is declared", got)
	}
}
