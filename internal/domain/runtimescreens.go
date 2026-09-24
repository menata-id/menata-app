package domain

// RuntimeScreens are the screens the *runtime* owns, as opposed to the ones an Application
// describes: Workspace Home, Workspace Members, Groups, the Authorization Matrix.
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
	},
	{ID: "nav_workspace_members", Label: "Workspace Members", Route: "/workspace-members"},
	{ID: "nav_workspace_groups", Label: "Groups", Route: "/workspace-groups"},
	{ID: "nav_role_matrix", Label: "Authorization Matrix", Route: "/authorization-matrix"},
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
