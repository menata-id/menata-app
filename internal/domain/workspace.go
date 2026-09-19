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
}
