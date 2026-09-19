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
}
