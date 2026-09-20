package domain

// Workspace is the highest organizational boundary (001-design-principles.md Principle #9,
// 004-runtime-metadata.md "Workspace"). It owns the Machines and the Applications inside it.
//
// Machines are workspace-level and unique by id across the Workspace (Fase 3, 2026-09-20), not
// owned per Application: several Applications genuinely share one (mch_user is every
// Application's identity, mch_activity is written by all of them), so per-Application ownership
// would load the same file twice and produce two Machine objects with one id. An Application
// *selects* from this set instead -- see Application.Machines.
type Workspace struct {
	ID   string
	Name string
	// Applications are the Applications declared inside this Workspace, in declaration order --
	// which is the order the launcher and Workspace Home list them in.
	Applications []Application
	// Navigation is the Workspace's own menu: destinations belonging to no single Application
	// (Home, All Machines, Workspace Members). They stay reachable however many Applications
	// exist, and they are what the launcher shows above the Application list.
	Navigation []NavigationItem
}

// ApplicationByID returns the Application with the given id. Applications are few and this is
// called once per request at most, so a linear scan beats maintaining a parallel map.
func (w Workspace) ApplicationByID(id string) (Application, bool) {
	for _, app := range w.Applications {
		if app.ID == id {
			return app, true
		}
	}
	return Application{}, false
}

// ApplicationForMachine answers "which Application does this Machine belong to", the primary half
// of resolving which Application a request is in.
//
// It exists because navigation membership alone cannot answer that: the whole
// /machines/{machineID}/... surface -- record detail, /decide, /edit, /pdf-preview,
// /signature-placement -- plus /documents and the member-edit routes are named by no navigation
// item at all, and those are most of Document Approval's real screens. Deriving from the Machine
// covers them; navigation is only the fallback, for routes naming no Machine.
//
// A Machine claimed by no Application (mch_user, mch_activity -- the shared, Workspace-level
// ones) returns false, and so does an unknown id. metadata.validateMachineClaims guarantees no
// Machine is claimed by two Applications, which is what makes the answer unambiguous.
func (w Workspace) ApplicationForMachine(machineID string) (Application, bool) {
	for _, app := range w.Applications {
		for _, id := range app.Machines {
			if id == machineID {
				return app, true
			}
		}
	}
	return Application{}, false
}

// ApplicationForRoute is the fallback half: the Application whose own navigation declares this
// exact route. Used only when the route names no Machine (/dashboard, /my-tasks, ...).
//
// It matches against the *unfiltered* navigation (AllNavigation), so a route whose menu chrome is
// suppressed by show_nav still resolves to its Application -- the same reason routeByID resolves
// against the unfiltered list.
func (w Workspace) ApplicationForRoute(route string) (Application, bool) {
	for _, app := range w.Applications {
		for _, item := range app.AllNavigation {
			if item.Route == route {
				return app, true
			}
		}
	}
	return Application{}, false
}

// Application is an independently realizable business solution within a Workspace
// (004-runtime-metadata.md "Application").
type Application struct {
	ID          string
	Name        string
	WorkspaceID string
	// Machines are the ids this Application exposes, selected from its Workspace's own set. Not
	// file paths: the Machines themselves are loaded once, at Workspace level.
	//
	// The list is presentational and derivational, never an access gate -- Permission remains the
	// single authority on who may do what (006 §Behavioral Model). Its load-bearing job is
	// Workspace.ApplicationForMachine above.
	Machines []string
	// Roles is this Application's own role vocabulary -- what a member may hold *here*, which is a
	// different question from their Workspace role (admin/member). Declared per Application
	// because the vocabularies genuinely differ between them; an Application may declare none,
	// and then offers no role at all rather than borrowing another's words.
	//
	// Plain strings, not id-keyed objects: a role is a vocabulary word today, with no identity of
	// its own to reference. It fills the member-role selects and captions the Members list, and
	// nothing gates on it -- authorization.AllowsAction never sees it. Roles become grantable,
	// and therefore worth giving ids, in ROADMAP.md's Case 03 Fase 7.
	Roles []string
	// ShowNav reports whether this Application renders persistent menu chrome. False is
	// ui-sample/nav-metadata.js's own `showNav: false` (owner request, 2026-09-19, for Document
	// Approval), and replaces what app.yaml used to express as a `hidden_nav_groups` entry naming
	// a group label. It suppresses the *menu* only: every route stays a valid destination,
	// reachable from contextual in-page links and from the launcher, which is uniform and never
	// reads this field.
	ShowNav bool
	// Navigation is this Application's own declared menu (004 §Navigation Metadata, 006
	// §Navigation), as rendered -- empty when ShowNav is false.
	Navigation []NavigationItem
	// PrimaryNavGroup is the one named group (by its group: label) the topbar keeps open inline
	// rather than collapsed behind a dropdown. Decided from the full declared navigation, before
	// any filtering, so suppressing a menu can never promote a different group into this role by
	// accident. Empty when this Application's navigation declares no named group at all -- which
	// is the normal case now that the groups themselves became Applications.
	PrimaryNavGroup string
	// HomeRoute is HomeCardRoute's result over the full declared navigation: the route this
	// Application's own card on Workspace Home links to. Decided at the same point and for the
	// same reason as PrimaryNavGroup. Empty when no navigation item declares home_card: true.
	HomeRoute string
	// AllNavigation is the full declared navigation list, before ShowNav suppresses anything --
	// unlike Navigation, it is never emptied.
	//
	// It exists so a page can look up *any* declared item's route or label by id
	// (internal/rendering's routeByID/labelByID) when linking to a sibling screen, rather than
	// hand-typing it. That must keep working for an Application whose menu is hidden: its routes
	// "stay valid destinations, reachable by contextual in-page links", so those links have to
	// resolve the real route even though the item never appears in a menu. Both
	// metadata-hardcoding conformance gates depend on this property.
	AllNavigation []NavigationItem
}
