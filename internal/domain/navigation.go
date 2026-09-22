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
	// Icon is a single character, the same convention as Application.Icon (ui-sample/
	// nav-metadata.js's own placeholder-glyph choice, "not-yet-scoped" for a real SVG set).
	// Rendered by appShell's mobile bottom bar (owner spec, ui-sample/README.md's "Menu Navigasi":
	// "untuk mobile ada di bagian bawah berupa bottom bar menu, dengan 3-4 icon") -- the desktop
	// row stays plain text links, so an item with no Icon still renders fully there. Optional:
	// only the items a bottom bar actually shows (appshell.templ's topByPriority) need one.
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
