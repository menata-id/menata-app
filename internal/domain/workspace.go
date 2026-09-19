package domain

// Workspace is the highest organizational boundary (001-design-principles.md Principle #9,
// 004-runtime-metadata.md "Workspace"). Phase 2 hardcodes exactly one; the type exists now so
// Phase 3+ doesn't require a breaking metadata-shape change.
type Workspace struct {
	ID   string
	Name string
}

// Application is an independently realizable business solution within a Workspace
// (004-runtime-metadata.md "Application").
type Application struct {
	ID          string
	Name        string
	WorkspaceID string
	// Navigation is this Application's own declared menu (004 §Navigation Metadata, 006
	// §Navigation), replacing a hand-written topbar with a projection of metadata -- ROADMAP.md's
	// long-tracked "Navigation is code, not metadata" conformance gap.
	Navigation []NavigationItem
	// PrimaryNavGroup is the one named group (by its group: label) the topbar keeps open inline
	// rather than collapsed behind a dropdown -- decided once, from the full declared navigation:
	// order, before any hidden_nav_groups filtering removes items. Kept separate from Navigation
	// itself so hiding a group can never promote a different group into this role by accident
	// (owner correction, 2026-09-19: hiding Document Approval had silently promoted Project
	// Management from a collapsed dropdown to the always-open group, which was never the intent --
	// ui-sample's own nav mockup shows hiding one Application's menu has zero effect on any
	// other's). Empty when the declared navigation has no named group at all.
	PrimaryNavGroup string
	// HomeRoute is HomeCardRoute's result over the full declared navigation, decided at the same
	// point and for the same reason as PrimaryNavGroup: before hidden_nav_groups filtering runs,
	// so hiding the HomeCard item's own group can't silently blank it out. Empty when no
	// navigation item declares home_card: true -- WorkspaceHomePage's own caller
	// (internal/web/workspacehome.go) falls back to "/home" in that case.
	HomeRoute string
	// AllNavigation is the full declared navigation list, before hidden_nav_groups filtering --
	// unlike Navigation, hiding an item's group does not remove it here. It exists so a page can
	// look up *any* declared item's route by id (internal/rendering's routeByID) when linking to
	// one of this Application's own sibling screens, rather than hand-typing the route -- exactly
	// the contextual-link case hidden_nav_groups' own doc comment names (a hidden group's routes
	// "stay valid destinations, reachable by contextual in-page links"): those links must still
	// resolve the real route even though the item itself no longer appears in Navigation.
	AllNavigation []NavigationItem
}
