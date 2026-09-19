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
}
