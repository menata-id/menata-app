package domain

// RuntimeScreens are the screens the *runtime* owns, as opposed to the ones an Application
// describes: Workspace Home, Workspace Members, Groups, Workspace settings.
//
// They were `workspace.navigation:` in metadata/app.yaml until 2026-09-21, when the owner removed
// that block: a Workspace's menu should be *derived* (the Applications it contains, plus "All
// Workspaces"), not hand-listed. Deleting the declarations alone would have left four live screens
// with no source for their own route and title -- routeByID/labelByID panic on an unknown id -- so
// their identity moves here rather than becoming literals retyped across eight .templ files.
//
// That is not metadata smuggled into Go. It is 001 Principle #2, Runtime First: Runtime Metadata
// defines *applications*, and these four are not one. They exist identically in a Workspace with
// ten Applications or none, which is the same test internal/conformance's `runtimeLevelRoutes`
// already applies to /home and /login -- and which appShell's breadcrumb already acted on, linking
// "/home" as a literal with a comment saying exactly this. No Application may redeclare one of
// these ids (metadata.validateNavigationIDsAreUnique), so an id still resolves to one screen
// workspace-wide.
//
// They are deliberately NOT a menu. Nothing ranges over this slice to draw navigation; the
// launcher and the pageShell topbar both project the Workspace's Applications now. This is the
// route/label registry those screens read their own identity from, nothing more -- which is why
// "All Machines" ("/") is absent: it is a real route, but no screen looks it up by id, and adding
// a row nothing reads would just be the hand-maintained list this change removed.
var RuntimeScreens = []NavigationItem{
	// Title/Description rather than Label alone, the same split an Application's own navigation
	// items gained on 2026-09-24: "Home" is what a menu and a breadcrumb call this screen, and
	// "Applications" is what the screen calls itself (Flow 2 mockup, board 03). The description
	// deliberately does not name the Workspace the way the board does -- a static string cannot,
	// and the Workspace's name is one row up in the chrome anyway.
	{
		ID:          "nav_home",
		Label:       "Home",
		Route:       "/home",
		Title:       "Applications",
		Description: "The applications you can open here, based on the roles assigned to you.",
		// A row of the Workspace settings sections (see the group below).
		SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "Workspace", Priority: 2,
	},
	// The Workspace settings sections, the rows of its NavList (sidebar and hub). General, Authentication and Audit log
	// have no screen: they are listed as not built, which is what the mockup shows, and link nowhere.
	{
		ID: "nav_workspace_general", Label: "General", Description: "Name, logo, time zone and language.",
		SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "Workspace", Priority: 1, Placeholder: true,
	},
	// "Members" is the board's own heading (04); "Workspace Members" stays the menu's word, where "Members"
	// alone would not say whose. The description cannot name the Workspace for the reason nav_home's
	// cannot.
	{
		ID:              "nav_workspace_members",
		Label:           "Workspace Members",
		Route:           "/workspace-members",
		Title:           "Members",
		Description:     "Everyone in this workspace, their workspace role and the applications they can open.",
		SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "People", Priority: 1,
	},
	{ID: "nav_workspace_groups", Label: "Groups", Route: "/workspace-groups", Description: "Assign a role to a Group once, then add or remove members instead of editing every person's own row. A member's effective role in an Application is their direct assignment plus every role their Groups hold there.", SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "People", Priority: 2},
	// An Application's own Members & roles (S2.2): who holds which role in that one Application, the role editable in place. The
	// route carries the Application's settings hub id (the record), which the page reads as $parameters.hub. An admin screen that
	// is not a settings row: its way back is the Application's own hub.
	{
		ID: "nav_application_members", Label: "Members & roles", Route: "/settings", Title: "Members & roles",
		RecordSource: "settings_applications", RecordParam: "hub",
	},
	// One member's own screen: their Workspace role, their role in each Application, and removing them from the Workspace.
	// Not a settings row; it sits under Members.
	{
		ID: "nav_workspace_member", Label: "Member", Route: "/workspace-members", Title: "Member",
		SettingsCurrent: "nav_workspace_members", OwnsHeading: true, RecordSource: "members",
	},
	// One Group's own screen: members, roles and delete, drawn for the record the route names. Not a settings row; it sits under
	// Groups.
	{
		ID: "nav_workspace_group", Label: "Group", Route: "/workspace-groups", Title: "Group",
		SettingsCurrent: "nav_workspace_groups", OwnsHeading: true, RecordSource: "groups",
	},
	// Invitations share Members' route: one page lists both (an invitation is not a membership, but it is drawn beside one).
	{
		ID: "nav_workspace_invitations", Label: "Invitations", Route: "/workspace-members", Description: "Pending and expired invitations.",
		SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "People", Priority: 3,
	},
	{
		ID: "nav_workspace_auth", Label: "Authentication", Description: "Two-step verification, allowed email domains.",
		SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "Security", Priority: 1, Placeholder: true,
	},
	{
		ID: "nav_workspace_audit", Label: "Audit log", Description: "Permanent record of changes.",
		SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "Security", Priority: 2, Placeholder: true,
	},
	{
		ID: "nav_workspace_danger", Label: "Danger zone", Route: "/workspace-settings#danger-zone",
		SettingsSection: NavSectionWorkspaceSettings, SettingsGroup: "Security", Priority: 3, SidebarOnly: true,
	},
	// nav_new_application (2026-10-17): the assistant's conversation, where an Application is described and generated as a draft.
	// A runtime screen for the reason the rest are: it exists identically in a Workspace with ten Applications or none.
	{
		ID: "nav_new_application", Label: "New application", Route: "/new-application", Title: "New application",
		Description: "Tell Menata what your team needs. It asks a few questions, shows what it will build, then generates a draft only you can see.",
	},
	// nav_notifications (2026-10-17): the viewer's own notifications, across Applications. A runtime screen for the
	// reason the rest are: it exists identically in a Workspace with any number of Applications. Its body is a runtime
	// page (metadata/runtimepages), and its Dataset sits on mch_notification, which every Workspace shares.
	{
		ID:          "nav_notifications",
		Label:       "Notifications",
		Route:       "/notifications",
		Title:       "Notifications",
		Description: "Approval requests and decisions that concern you.",
	},
	// nav_role_matrix ("Authorization Matrix", /authorization-matrix) lived here until 2026-09-27,
	// when it was deleted rather than merely left unlinked: Q1 of the Flow 2 canvas re-audit
	// (ROADMAP.md) confirmed board-06-new (the plain-language Permissions page,
	// internal/web/appsettings.go) supersedes board-06-old (this screen), so an unreachable second
	// implementation of the same fact was dead weight, not a kept exception. Its shared rendering
	// (RoleMatrixApp/roleMatrixApp, internal/composition.RoleMatrixForApplication) lives on --
	// appsettings.go's Permissions view is its one real caller now.
	//
	// nav_workspace_settings: Flow 2 gap study Tahap 5, 2026-09-25 (M04a-Settings.dc.html) --
	// the Workspace-level counterpart of an Application's own Settings hub
	// (nav_app_settings, metadata/applications/*.yaml). A runtime screen like the two above it,
	// not an Application concept, for the identical reason: it exists in a Workspace with any
	// number of Applications or none.
	// nav_install_application (2026-09-28): the template library, and the one screen that turns
	// metadata/applications/*.yaml from a directory nothing could reach into something a person can
	// install. A runtime screen by the same test the four above pass -- it exists identically in a
	// Workspace with zero Applications or ten, and what it lists is the library, not this Workspace.
	{
		ID:          "nav_install_application",
		Label:       "Install an application",
		Route:       "/install-application",
		Title:       "Ready-made applications",
		Description: "Install one of these into this workspace. Only workspace admins can.",
	},
	{
		ID:          "nav_workspace_settings",
		Label:       "Workspace settings",
		Route:       "/workspace-settings",
		Title:       "Workspace settings",
		Description: "Applies to everyone in this workspace. Only workspace admins can see it.",
	},
	// nav_workspace_files (2026-10-11): the screen for replacing one of the Workspace's own YAML files -- pick, edit,
	// review the loader's verdict and the diff, apply. A runtime screen for the reason nav_workspace_settings is: it
	// exists identically in a Workspace with any number of Applications, and what it edits is the Workspace's
	// installation rather than an Application. It draws /workspace-settings/validate and /apply; those routes are
	// unchanged.
	{
		ID:          "nav_workspace_files",
		Label:       "Workspace files",
		Route:       "/workspace-settings/files",
		Title:       "Replace a file",
		Description: "Paste the new content of a file this workspace already has. You see what the loader says and what would change before anything is written.",
	},
	// nav_inference (2026-09-29): the diagnostics surface 001 Principle #6's second clause requires --
	// "**Inference must be inspectable.** … the runtime should be able to expose the resolved result
	// through diagnostics or equivalent tooling", restated by 004 §Inference and 005 Phase 4.
	//
	// A runtime screen by the same test the five above pass, and the test is not a formality here: what
	// it shows is *how the runtime resolved* an Application's declarations, which is a fact about the
	// runtime rather than about any Application. It exists identically in a Workspace with zero
	// Applications or ten -- with nothing to explain in the first case, which is itself a true answer.
	//
	// **It deliberately is not, and must not become, an Application's own declared View.** 007 §18.13
	// ("Execution plans are physical runtime artifacts. They MUST NOT become user-authored metadata")
	// and 001 #17 say so directly, and 002's Runtime Boundary adds that those internal stages "must not
	// leak physical implementation choices back into metadata". A domain.Resolution describes the output
	// of Phase 4 normalisation; declaring a Dataset over it would couple an Application's metadata to a
	// runtime-internal shape. 007 §7.1's DataSource kinds confirm it from the other side -- Machine
	// records, another Dataset, a declared relation, or "a runtime service source where explicitly
	// supported" -- and a resolution is none of the first three. That last clause plus 007 §31's own
	// open question, "how to expose execution diagnostics without coupling metadata to physical plans",
	// are the forward-checkable pointers for whoever revisits this (CLAUDE.md step 2b).
	{
		ID:          "nav_inference",
		Label:       "Inference",
		Route:       "/inference",
		Title:       "What the runtime inferred",
		Description: "Every derivation this workspace's applications rely on, and the declaration each one was read from. Only workspace admins can see it.",
	},
}

// IsRuntimeScreenID reports whether id names one of RuntimeScreens.
func IsRuntimeScreenID(id string) bool {
	for _, s := range RuntimeScreens {
		if s.ID == id {
			return true
		}
	}
	return false
}

// RuntimeScreenRoute is one runtime screen's route by id, or "" for an unknown id.
//
// The handler-side counterpart of rendering.routeByID, added 2026-09-29 so a Go-side message can point
// at a runtime screen without retyping its route. /workspace-members happens to sit in
// internal/conformance's runtimeLevelRoutes exemption, so a literal would have passed the gate -- but
// the exemption exists for router.go's own registrations, and a message body is not one of those.
func RuntimeScreenRoute(id string) string {
	for _, s := range RuntimeScreens {
		if s.ID == id {
			return s.Route
		}
	}
	return ""
}

// RuntimeScreens are declared in Go, so they never pass through metadata.LoadApplication and would carry
// no resolved Heading -- which surfaced as an empty <h1> on every Workspace-level screen the moment
// titleByID stopped falling back at render time (2026-10-02).
//
// Resolved here through the same function the loader calls, rather than by writing Heading into each
// literal above: one implementation of the inference, which is the whole point of
// ResolveNavigationHeadings existing in this package (001 #8). An init rather than a hand-stamped field
// also means a screen added to the list above cannot forget it.
func init() { ResolveNavigationHeadings(RuntimeScreens) }
