package metadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/domain"
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
workspace: default
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
	if app.Workspace.Slug != "default" {
		t.Errorf("Workspace.Slug = %q, want default", app.Workspace.Slug)
	}
	if app.Workspace.Applications[0].WorkspaceSlug != "default" {
		t.Errorf("Application.WorkspaceSlug = %q, want default", app.Workspace.Applications[0].WorkspaceSlug)
	}
	if len(app.Machines) != 1 || app.Machines[0].ID != "mch_task" {
		t.Fatalf("Machines = %+v, want one machine mch_task", app.Machines)
	}
}

// TestLoadApplication_suggestedApplications is Workspace Home's own "Add an application" chips
// (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27) -- declared per-Workspace, not a literal in
// internal/rendering, so this is a manifest-level concern like Applications itself.
func TestLoadApplication_suggestedApplications(t *testing.T) {
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
workspace: default
machines:
  - task.yaml
suggested_applications:
  - label: "Leave & permits"
    prompt: "Leave requests approved by a supervisor, then HR"
  - label: "Asset booking"
    prompt: "Booking shared equipment, approved by whoever owns them"
`)

	app, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	want := []domain.ApplicationSuggestion{
		{Label: "Leave & permits", Prompt: "Leave requests approved by a supervisor, then HR"},
		{Label: "Asset booking", Prompt: "Booking shared equipment, approved by whoever owns them"},
	}
	got := app.Workspace.SuggestedApplications
	if len(got) != len(want) {
		t.Fatalf("SuggestedApplications = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SuggestedApplications[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadApplication_suggestedApplicationMissingLabelRejected(t *testing.T) {
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
workspace: default
machines:
  - task.yaml
suggested_applications:
  - label: ""
    prompt: "Leave requests approved by a supervisor, then HR"
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want an error for a suggestion with no label")
	}
}

func TestLoadApplication_suggestedApplicationMissingPromptRejected(t *testing.T) {
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
workspace: default
machines:
  - task.yaml
suggested_applications:
  - label: "Leave & permits"
    prompt: ""
`)

	_, err := LoadApplication(filepath.Join(dir, "app.yaml"))
	if err == nil {
		t.Fatal("LoadApplication() error = nil, want an error for a suggestion with no prompt")
	}
}

func TestLoadApplication_badWorkspaceSlug(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "task.yaml", `
id: mch_task
name: Task
`)
	// "ws_default" was the *valid* form until 2026-09-22 and is now the invalid one: a manifest
	// names its Workspace by slug, and a slug carries no underscores.
	writeFile(t, dir, "app.yaml", `
workspace: ws_default
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
		t.Fatal("LoadApplication() error = nil, want error for a malformed workspace slug")
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
  - id: nav_overview
    label: Overview
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
workspace: default
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
  - id: nav_overview
    label: Overview
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
workspace: default
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
  - id: nav_overview
    label: Overview
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
workspace: default
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
workspace: default
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
  - id: nav_overview
    label: Overview
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
workspace: default
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
  - id: nav_overview
    label: Overview
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
workspace: default
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
  - id: nav_overview
    label: Overview
    route: /home
  - id: nav_overview
    label: Overview Again
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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
workspace: default
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

// rolePermissionManifest writes a two-Machine Workspace where one Machine is claimed by an
// Application declaring `roles`, and the other is claimed by nobody -- the shared-Machine case
// (mch_user, mch_activity) that the role arm has to answer for as well.
func rolePermissionManifest(t *testing.T, roles, permission string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "step.yaml", `
id: mch_step
name: Step
fields:
  - id: fld_assignee
    name: Assignee
    type: person
permissions:
`+permission)
	// mch_user is the person Field's implicit target, and doubles as the unclaimed, shared
	// Machine this test needs -- exactly what it is in the real manifest.
	writeFile(t, dir, "shared.yaml", `
id: mch_user
name: User
fields:
  - id: fld_name
    name: Name
    type: text
`)
	writeFile(t, dir, "app.yaml", `
workspace: default
machines:
  - step.yaml
  - shared.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_approval
name: Approval
machines:
  - mch_step
roles:
`+roles)
	return filepath.Join(dir, "app.yaml")
}

// A Machine's ApplicationID is an index over the Application's own machines: selection, never a
// second declaration -- and a Machine no Application claims keeps "".
func TestLoadApplication_stampsApplicationID(t *testing.T) {
	app, err := LoadApplication(rolePermissionManifest(t, "  - approver\n", "  - id: prm_decide\n    action: decide\n    actor_field: fld_assignee\n"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	for _, m := range app.Machines {
		want := "app_approval"
		if m.ID == domain.UserMachineID {
			want = ""
		}
		if m.ApplicationID != want {
			t.Errorf("%s.ApplicationID = %q, want %q", m.ID, m.ApplicationID, want)
		}
	}
}

// Both failures validatePermissionRoles catches are silent ones: the Permission parses, validates
// and then denies everyone forever while reading like a grant.
func TestLoadApplication_permissionRolesMustBeDeclared(t *testing.T) {
	tests := []struct {
		name       string
		roles      string
		permission string
		wantErr    bool
	}{
		{
			name:       "a role the application declares",
			roles:      "  - approver\n",
			permission: "  - id: prm_decide\n    action: decide\n    roles: [approver]\n    actor_field: fld_assignee\n",
		},
		{
			name:       "a role nobody declares -- nobody can hold it",
			roles:      "  - approver\n",
			permission: "  - id: prm_decide\n    action: decide\n    roles: [supervisor]\n    actor_field: fld_assignee\n",
			wantErr:    true,
		},
		{
			// The Application declares no vocabulary at all, so the word means nothing here.
			name:       "a role on an application that declares none",
			roles:      "",
			permission: "  - id: prm_decide\n    action: decide\n    roles: [approver]\n    actor_field: fld_assignee\n",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadApplication(rolePermissionManifest(t, tt.roles, tt.permission))
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadApplication() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// The unclaimed-Machine half, which no per-Machine validator can see: mch_user belongs to no
// Application, so domain.Actor.HasRole would return false for every role every time.
func TestLoadApplication_roleBearingPermissionOnUnclaimedMachine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "shared.yaml", `
id: mch_user
name: User
fields:
  - id: fld_assignee
    name: Assignee
    type: person
permissions:
  - id: prm_edit
    action: edit
    roles: [approver]
    actor_field: fld_assignee
`)
	writeFile(t, dir, "step.yaml", `
id: mch_step
name: Step
fields:
  - id: fld_title
    name: Title
    type: text
`)
	writeFile(t, dir, "app.yaml", `
workspace: default
machines:
  - shared.yaml
  - step.yaml
applications:
  - app-main.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_approval
name: Approval
machines:
  - mch_step
roles:
  - approver
`)
	if _, err := LoadApplication(filepath.Join(dir, "app.yaml")); err == nil {
		t.Error("LoadApplication() = nil, want an error -- a role-bearing permission on a machine no application claims can never be satisfied")
	}
}

// workflowManifest writes a Workspace holding one Application over two Machines, with whatever
// `workflow:` block a case wants to test. The Machine ids are deliberately *not* the template
// library's mch_document/mch_approval_step: the binding must work by what the Application declares,
// not by what anything is called, so a fixture using the familiar names could not tell the
// difference (ROADMAP.md Stage A).
func workflowManifest(t *testing.T, workflow string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "surat.yaml", "id: mch_surat\nname: Surat\n")
	writeFile(t, dir, "langkah.yaml", "id: mch_langkah\nname: Langkah\n")
	writeFile(t, dir, "ttd.yaml", "id: mch_ttd\nname: Tanda Tangan\n")
	writeFile(t, dir, "lain.yaml", "id: mch_lain\nname: Lain\n")
	writeFile(t, dir, "app.yaml", `
workspace: default
machines:
  - surat.yaml
  - langkah.yaml
  - ttd.yaml
  - lain.yaml
applications:
  - app-main.yaml
  - app-other.yaml
`)
	writeFile(t, dir, "app-main.yaml", `
id: app_persetujuan
name: Persetujuan
machines:
  - mch_surat
  - mch_langkah
  - mch_ttd
`+workflow)
	writeFile(t, dir, "app-other.yaml", "id: app_lain\nname: Lain\nmachines:\n  - mch_lain\n")
	return filepath.Join(dir, "app.yaml")
}

const validWorkflow = `workflow:
  engine: document_approval
  roles:
    document: mch_surat
    step: mch_langkah
`

// The binding is declared once, on the Application, and stamped onto each Machine it names -- the
// same derive-don't-retype relationship ApplicationID has with `machines:`. A Machine the binding
// does not name keeps both fields empty, which is what action.IsDocument/IsStep read as "not mine".
func TestLoadApplication_stampsWorkflowRoles(t *testing.T) {
	app, err := LoadApplication(workflowManifest(t, validWorkflow))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}

	want := map[string][2]string{
		"mch_surat":   {domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleDocument},
		"mch_langkah": {domain.WorkflowEngineDocumentApproval, domain.WorkflowRoleStep},
		"mch_lain":    {"", ""}, // another Application's Machine, bound to nothing
	}
	for _, m := range app.Machines {
		got := [2]string{m.WorkflowEngine, m.WorkflowRole}
		if got != want[m.ID] {
			t.Errorf("%s: engine/role = %v, want %v", m.ID, got, want[m.ID])
		}
	}

	// And the declaration itself survives on the Application, since that is what an installer has
	// to rewrite when a collision forces a rename (ROADMAP.md "Installing a template into a
	// Workspace that already uses its ids").
	binding := app.Workspace.Applications[0].Workflow
	if binding.MachineForRole(domain.WorkflowRoleDocument) != "mch_surat" {
		t.Errorf("Workflow.MachineForRole(document) = %q, want mch_surat", binding.MachineForRole(domain.WorkflowRoleDocument))
	}
	if app.Workspace.Applications[1].Workflow != nil {
		t.Error("an Application declaring no workflow: must load with a nil binding, not an empty one -- nil is what \"runs on no engine\" means")
	}
}

// Every one of these is silent if it loads: the Application renders its screens while the engine
// either never engages or engages over the wrong records. See validateWorkflowBinding.
func TestLoadApplication_workflowBindingMustBeRealizable(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		want     string
	}{
		{
			name: "unknown engine",
			workflow: `workflow:
  engine: persetujuan_berlapis
  roles:
    document: mch_surat
    step: mch_langkah
`,
			want: "not an engine this runtime realizes",
		},
		{
			name: "missing role",
			workflow: `workflow:
  engine: document_approval
  roles:
    document: mch_surat
`,
			want: "requires a machine for role \"step\"",
		},
		{
			name: "misspelled role, reported as itself and not only as the missing one",
			workflow: `workflow:
  engine: document_approval
  roles:
    document: mch_surat
    steps: mch_langkah
`,
			want: "declares no role \"steps\"",
		},
		{
			name: "role naming a machine this application does not claim",
			workflow: `workflow:
  engine: document_approval
  roles:
    document: mch_surat
    step: mch_lain
`,
			want: "not one of this application's own machines",
		},
		{
			name: "one machine in two roles",
			workflow: `workflow:
  engine: document_approval
  roles:
    document: mch_surat
    step: mch_surat
`,
			want: "declared for both workflow roles",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadApplication(workflowManifest(t, tt.workflow))
			if err == nil {
				t.Fatalf("LoadApplication() error = nil, want one containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("LoadApplication() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// An optional role omitted is a smaller installation, not an error: this Application runs the
// approval engine without reusable signatures and without saved default flows (see
// domain.WorkflowEngineSpec). The Machine it does not cast stays unstamped, which is what every
// role-resolving caller reads as "this Workspace has no such feature".
func TestLoadApplication_optionalWorkflowRolesMayBeOmitted(t *testing.T) {
	app, err := LoadApplication(workflowManifest(t, validWorkflow))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v", err)
	}
	for _, m := range app.Machines {
		if m.ID == "mch_ttd" && (m.WorkflowEngine != "" || m.WorkflowRole != "") {
			t.Errorf("mch_ttd is cast in no role but was stamped %q/%q", m.WorkflowEngine, m.WorkflowRole)
		}
	}

	// And casting it is equally valid -- the same Application, one role wider.
	withSignature := validWorkflow + "    signature: mch_ttd\n"
	app, err = LoadApplication(workflowManifest(t, withSignature))
	if err != nil {
		t.Fatalf("LoadApplication() with an optional role error = %v", err)
	}
	for _, m := range app.Machines {
		if m.ID == "mch_ttd" && m.WorkflowRole != domain.WorkflowRoleSignature {
			t.Errorf("mch_ttd.WorkflowRole = %q, want %q", m.WorkflowRole, domain.WorkflowRoleSignature)
		}
	}
}

// An optional role is optional about being *declared*, never about being declared correctly, and the
// flow-template pair is optional only together -- half a saved flow stores nothing.
func TestLoadApplication_optionalWorkflowRolesAreStillValidated(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		want     string
	}{
		{
			name:     "optional role naming a machine this application does not claim",
			workflow: validWorkflow + "    signature: mch_lain\n",
			want:     "not one of this application's own machines",
		},
		{
			name:     "optional role reusing a machine another role already holds",
			workflow: validWorkflow + "    signature: mch_surat\n",
			want:     "declared for both workflow roles",
		},
		{
			name:     "half a saved approval flow",
			workflow: validWorkflow + "    flow_template: mch_ttd\n",
			want:     "optional but inseparable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadApplication(workflowManifest(t, tt.workflow))
			if err == nil {
				t.Fatalf("LoadApplication() error = nil, want one containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("LoadApplication() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestDecodeStrict_rejectsAnUnknownKey closes this runtime's last validation hole, in the three
// document kinds it has (ROADMAP.md's "Reject unknown metadata keys at load").
//
// The Machine case is the 2026-09-20 finding turned into a test: a copy of metadata/ whose mch_document
// used the pre-2026-09-20 singular `view:` block loaded with no error at all and simply had zero Views.
// A retired or misspelled key was the one mistake that produced no error anywhere -- everything the
// loader *knows* has been validated strictly for months, which is exactly what made this easy to miss.
func TestDecodeStrict_rejectsAnUnknownKey(t *testing.T) {
	tests := []struct {
		name    string
		build   func(t *testing.T) string
		want    string
		concept string
	}{
		{
			name: "a machine file, using the retired singular view: block",
			build: func(t *testing.T) string {
				dir := t.TempDir()
				writeFile(t, dir, "task.yaml", "id: mch_task\nname: Task\nview:\n  type: board\n")
				writeFile(t, dir, "app.yaml", "workspace: default\nmachines:\n  - task.yaml\napplications: []\n")
				return filepath.Join(dir, "app.yaml")
			},
			want:    `"view" is not a key`,
			concept: "a machine file",
		},
		{
			name: "an application file",
			build: func(t *testing.T) string {
				dir := t.TempDir()
				writeFile(t, dir, "task.yaml", "id: mch_task\nname: Task\n")
				writeFile(t, dir, "app.yaml", "workspace: default\nmachines:\n  - task.yaml\napplications:\n  - app-main.yaml\n")
				writeFile(t, dir, "app-main.yaml", "id: app_task_tracker\nname: Task Tracker\nmachines:\n  - mch_task\nshow_navigation: false\n")
				return filepath.Join(dir, "app.yaml")
			},
			want:    `"show_navigation" is not a key`,
			concept: "an application file",
		},
		{
			name: "a workspace manifest",
			build: func(t *testing.T) string {
				dir := t.TempDir()
				writeFile(t, dir, "task.yaml", "id: mch_task\nname: Task\n")
				writeFile(t, dir, "app.yaml", "workspace: default\nmachines:\n  - task.yaml\napplications: []\nsuggested_apps: []\n")
				return filepath.Join(dir, "app.yaml")
			},
			want:    `"suggested_apps" is not a key`,
			concept: "a workspace manifest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadApplication(tt.build(t))
			if err == nil {
				t.Fatalf("LoadApplication() error = nil, want one containing %q -- an unknown key used to load clean and silently drop the capability", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v\nwant it to name the key: %s", err, tt.want)
			}
			if !strings.Contains(err.Error(), tt.concept) {
				t.Errorf("error = %v\nwant it to name the document kind (%q), not a Go type", err, tt.concept)
			}
			if !strings.Contains(err.Error(), "line ") {
				t.Errorf("error = %v\nwant yaml's own line number kept -- that is the useful half", err)
			}
		})
	}
}

// TestDecodeStrict_acceptsMapKeys is the property a later reader would most plausibly break while
// "tidying" the strict decoder: KnownFields constrains struct fields, not map keys, so an Application's
// own workflow.roles -- whose keys are role names, not schema -- stays exactly as legal as it was.
//
// Without this, adding a fifth role to an engine would look like a metadata error.
func TestDecodeStrict_acceptsMapKeys(t *testing.T) {
	app, err := LoadApplication(workflowManifest(t, validWorkflow+"    signature: mch_ttd\n"))
	if err != nil {
		t.Fatalf("LoadApplication() error = %v -- workflow.roles keys are map keys, which strict decoding does not constrain", err)
	}
	if got := app.Workspace.Applications[0].Workflow.Roles[domain.WorkflowRoleSignature]; got != "mch_ttd" {
		t.Errorf("roles[signature] = %q, want mch_ttd", got)
	}
}
