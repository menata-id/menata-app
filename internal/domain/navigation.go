package domain

// NavBadgeApprovalInboxPending is the one live count a navigation item may request: the
// signed-in identity's own pending-approval count (already computed by
// composition.ApprovalInbox, ROADMAP.md Phase 21 round 2 Step J's own badge). Named as a
// constant rather than a free string so a typo in metadata fails validation instead of silently
// rendering no badge.
const NavBadgeApprovalInboxPending = "approval_inbox_pending"

// KnownNavigationBadges is the closed set of badge kinds a NavigationItem may declare -- the
// same static-seam posture as KnownFieldTypes and KnownActions (007 §14): a navigation item
// naming a badge the runtime does not realize would silently show nothing, which is worse than
// no badge declared at all.
var KnownNavigationBadges = map[string]bool{
	NavBadgeApprovalInboxPending: true,
}

// NavigationItem is one destination in the Application's own menu (004 §Navigation Metadata,
// 006 §Navigation: "menus, breadcrumbs, tabs, shortcuts, contextual links, quick actions").
//
// It is a declared pointer to an already-existing route, not something that generates a route
// the way declaring a Machine generates that Machine's own CRUD page -- most of this
// Application's real destinations (/dashboard, /approval-inbox, ...) are bespoke composed
// routes in internal/web, not generic Machine pages, so a navigation schema here can only name
// *where to go*, never *what's there* (006 §Navigation: "must remain separate from business
// execution and physical data access"). Route existence is therefore not cross-checked against
// anything at load time, the one field this package cannot validate the way it validates a
// Constraint's related Field -- routes are a runtime/internal/web concern, not a metadata one.
type NavigationItem struct {
	ID    string
	Label string
	Route string
	// Group is the per-Application submenu this item belongs to (e.g. "Document Approval"),
	// matching ui-sample/nav-metadata.js's own per-Application nav list. Empty means the item
	// renders flat at the top level (e.g. "Home"), not inside a dropdown.
	Group string
	// Priority orders items within their own Group (and orders ungrouped items among
	// themselves); ties keep declaration order.
	Priority int
	// Badge, if non-empty, must be one of KnownNavigationBadges -- the live count the runtime
	// renders next to this item's label.
	Badge string
	// HomeCard marks the one navigation item Workspace Home's own Application card should link
	// to (rendering.WorkspaceHomePage) -- at most one item across an Application's navigation may
	// set this. It exists so that card's route comes from metadata (001 Principle #3 Metadata
	// First, #8 Reference over Duplication) instead of being retyped as a literal in
	// workspacehome.templ, which is exactly the drift internal/conformance's
	// TestWorkspaceHomeHasNoHardcodedApplicationRoute now catches. No item marked: Workspace Home
	// falls back to linking at itself ("/home"), never a guessed Application route.
	HomeCard bool
	// Title is the heading the destination screen renders for *itself* -- its own <h1> -- and
	// Description the sentence under it. Both optional; a screen with no Title declared falls back
	// to Label (rendering.titleByID), which is what every screen did unconditionally until
	// 2026-09-24.
	//
	// They are separate from Label because they answer a different question, and the Flow 2 mockup
	// is where that stopped being theoretical: its Approval Inbox is labelled "Inbox" in the menu
	// and headed "Pending my approval" on the page. A menu entry has to be short enough to sit in
	// a tab strip and a 4-column bottom bar; a heading is the screen telling you what you are
	// looking at, and the two are only the same string by coincidence. Forcing them to be one
	// meant every screen's heading was constrained by how it had to read in a bottom bar.
	//
	// Description had no home at all before this -- every screen's own subtitle was a literal in
	// its .templ (approvalinbox.templ carried two), which is the metadata-hardcoding violation
	// CLAUDE.md's decision path describes, hiding in the one place the label gate could not see:
	// a sentence is not a Title-Case phrase, so nothing matched it.
	Title       string
	Description string
	// SettingsHub marks the one navigation item that *is* an Application's Settings landing page
	// (ROADMAP.md "In progress", "Application Settings hub") -- at most one per Application,
	// validated the same way HomeCard is. Its own Label/Title/Description/Route/Icon are what the
	// Application chrome's "Settings" link and the hub page's own heading read, the same "declare
	// it once, read it everywhere" shape HomeRoute already established.
	SettingsHub bool
	// SettingsHubMember marks any *other* item reached from that hub (e.g. the Permissions page)
	// rather than from the main menu -- mutually exclusive with SettingsHub on one item, and
	// excluded from appshell.templ's tab strip / bottom bar exactly like the hub root is.
	//
	// Deliberately a bool, not a string naming which section it sits under (an earlier draft of
	// this field, SettingsSection): the hub has exactly one real section ("Access") today, so a
	// per-item classification value would be declaring a fact metadata does not yet have two real
	// answers for. The section's own label is a plain string in appsettings.templ instead --
	// CLAUDE.md's "second real case" trigger applies to the *grouping concept* here, not to
	// whether an item is reachable at all, which this field still says.
	//
	// What actually decides whether the hub's Access section (and this item's own row in it) is
	// worth showing is domain.Application.Roles, read directly by internal/composition/
	// appsettings.go -- not the mere existence of a SettingsHubMember-flagged item. An Application
	// declaring nav_app_settings_permissions and later removing all its roles would otherwise leave
	// a dead-looking row that nothing catches; deriving the row's *content* from Roles, while this
	// field only ever decided the item's *addressability*, is what keeps that from drifting.
	SettingsHubMember bool
	// Icon must be one of KnownIcons, the same convention as Application.Icon. It was a single
	// literal character until 2026-09-24 -- see KnownIcons for why that convention ended and what
	// a name buys over a glyph.
	//
	// Rendered by appShell's mobile bottom bar (owner spec, ui-sample/README.md's "Menu Navigasi":
	// "untuk mobile ada di bagian bawah berupa bottom bar menu, dengan 3-4 icon") -- the desktop
	// row stays plain text links, so an item with no Icon still renders fully there. Optional:
	// only the items a bottom bar actually shows (appshell.templ's bottomBarItems) need one.
	Icon string
}

// HomeCardRoute returns the route of items' one HomeCard item, or "" if none is marked --
// internal/web/workspacehome.go's own caller decides the fallback ("/home"), since a sensible
// default belongs with the handler that already knows "/home" always resolves, not duplicated
// here.
func HomeCardRoute(items []NavigationItem) string {
	for _, item := range items {
		if item.HomeCard {
			return item.Route
		}
	}
	return ""
}
