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
	// Slug names which Workspace this manifest installs into, matched against the `workspaces`
	// table's own slug column at load time (metadata/workspaces/<slug>.yaml, 2026-09-22).
	//
	// It replaced an `ID`/`Name` pair that the manifest used to declare. Both were wrong once a
	// Workspace could be created through the UI: the `ws_...` id is generated at that moment, so
	// nobody can write it into a file by hand, and the name already lives in the Workspace's own
	// row -- restating it here would be the duplication 001 Principle #8 rules out. What a
	// manifest legitimately says is *which* Workspace it is for, and the slug is the only key a
	// person can both read and type.
	Slug string
	// MachineIDs are the Machines this Workspace's manifest declares, in declaration order -- the
	// membership list, which is what decides whether a Machine exists here at all.
	//
	// Without it a Workspace with nothing installed still listed every Machine in the process, and
	// /machines/<id> served a page for one it had never installed -- empty, since records are
	// workspace-scoped, but reachable and listed.
	MachineIDs []string
	// Machines are those same Machines, loaded -- this Workspace's own, in the same declaration
	// order, and the authoritative answer to "which Machine does id X mean *here*".
	//
	// It sits on the Workspace because which Workspace a request is in is a per-request fact
	// (CLAUDE.md, "One manifest per Workspace"), so what a Machine id means is one too. Until
	// 2026-09-27 the Machines themselves were loaded process-wide and shared instead -- one file,
	// one object, however many Workspaces installed it -- with cmd/server unioning every
	// Workspace's Machines into a single id-keyed map and *deduping by id, first loaded wins*. Two
	// Workspaces could therefore not hold different definitions under one id: the second silently
	// got the first one's Machine. That is what this field exists to end (owner decision, same
	// day: Workspaces are isolated, and installing a template copies it).
	//
	// Two Workspaces installing the same template file each get their own *domain.Machine value
	// from it. Nothing compares Machines by pointer -- every lookup here is by id -- and that
	// independence is the point, since either Workspace may later edit only its own copy.
	Machines []*Machine
	// Applications are the Applications declared inside this Workspace, in declaration order --
	// which is the order the launcher and Workspace Home list them in.
	Applications []Application
	// Navigation is the Workspace's own menu: destinations belonging to no single Application
	// (Home, All Machines, Workspace Members). They stay reachable however many Applications
	// exist, and they are what the launcher shows above the Application list.
	Navigation []NavigationItem
	// SuggestedApplications are Workspace Home's own "Add an application" chips (Flow 2 canvas
	// re-audit, ROADMAP.md, 2026-09-27) -- example prompts for the AI Metadata Assistant, declared
	// here rather than as literals in internal/rendering so a Workspace whose business the
	// mockup's own three examples don't fit can declare its own. Empty is valid: a Workspace with
	// no suggestions renders the input box alone, the chip row omitted, the same "empty list is a
	// normal state" posture Applications itself already takes.
	SuggestedApplications []ApplicationSuggestion
}

// ApplicationSuggestion is one chip in Workspace Home's own "Add an application" box: Label is the
// chip's own text, Prompt is what it pre-fills into the AI Metadata Assistant's first message
// (internal/web.showNewApplication's own ?idea= handling), which the conversation screen then
// auto-submits on load (rendering.NewApplicationPage) -- clicking a chip starts the conversation
// in one step, not two.
type ApplicationSuggestion struct {
	Label  string
	Prompt string
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

// KnownApplicationColors is the closed set of colour tokens an Application may declare -- the
// same static-seam posture as KnownFieldTypes and KnownNavigationBadges (007 §14). Closed for two
// reasons here: an unrecognized colour would render nothing, and the renderer maps each token to
// whole literal Tailwind class strings because Tailwind's scanner cannot see a class name built at
// runtime. An open set would mean building names, which fails silently in the browser rather than
// at load (capabilities.md, "two scanner rules").
//
// The tokens are Tailwind's own scale names, matching ui-sample/nav-metadata.js's `color` field.
var KnownApplicationColors = map[string]bool{
	"blue":    true,
	"emerald": true,
	"amber":   true,
	"slate":   true,
}

// Application is an independently realizable business solution within a Workspace
// (004-runtime-metadata.md "Application").
type Application struct {
	ID   string
	Name string
	// WorkspaceSlug is the Workspace whose manifest installed this Application -- the slug, since
	// that is what a manifest names (domain.Workspace.Slug). Several Workspaces may each install
	// their own *copy* of the same template (metadata/applications/, 2026-09-27 Workspace
	// isolation), so this says which installation produced *this* value, not something the
	// Application file itself declares.
	WorkspaceSlug string
	// Machines are the ids this Application exposes, selected from its Workspace's own set. Not
	// file paths: the Machines themselves are loaded once, at Workspace level.
	//
	// The list is presentational and derivational, never an access gate -- Permission remains the
	// single authority on who may do what (006 §Behavioral Model). Its load-bearing job is
	// Workspace.ApplicationForMachine above.
	Machines []string
	// Description, Icon and Color are this Application's card face on Workspace Home.
	//
	// Icon must be one of KnownIcons and Color one of KnownApplicationColors -- two closed sets
	// for the same reason, since both are names the renderer must recognize to draw anything at
	// all. Icon was a single literal character until 2026-09-24, the ui-sample/nav-metadata.js
	// placeholder its own comment called "not-yet-scoped" for a real SVG set; KnownIcons is that
	// set arriving.
	//
	// All three are optional: an Application declaring none still renders, just plainly -- with
	// its initials in place of an icon (rendering.ApplicationCard.Initials).
	Description string
	Icon        string
	Color       string
	// SummaryMachine names the Machine whose record count this Application's card reports, or ""
	// for no count.
	//
	// Declared rather than derived, because deriving it is misleading: summing every Machine an
	// Application claims counts configuration and plumbing as work (Project Management's Machines
	// total 13 -- mostly lists, labels and join rows, not its 4 tasks). Validated to be a Machine
	// this Application itself claims, so a card can never report another Application's number.
	SummaryMachine string
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
	// Workflow is the runtime workflow engine this Application runs on, and which of its own
	// Machines plays each role in it -- nil for an Application that runs on none, which is every
	// plain CRUD Application. See Workflow's own doc comment for why the binding is declared here
	// rather than matched as a literal in Go.
	Workflow *Workflow
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

// HasMachine reports whether machineID is installed in this Workspace. A Workspace with no
// manifest has none, which is the correct answer rather than a missing one.
func (w Workspace) HasMachine(machineID string) bool {
	for _, id := range w.MachineIDs {
		if id == machineID {
			return true
		}
	}
	return false
}
