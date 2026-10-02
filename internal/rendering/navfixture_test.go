package rendering

import "menata.app/internal/domain"

// resolvedNav is how a test fixture gets navigation the way the loader produces it, rather than the way
// a literal happens to look.
//
// titleByID reads NavigationItem.Heading, which metadata.LoadApplication resolves from Title-or-Label at
// load (005 Phase 4). A hand-built fixture bypasses that, so before 2026-10-02 two fixtures here rendered
// an **empty heading** and their tests failed -- correctly, and for the same reason the "Normalize first"
// rule exists for Machine fixtures: a fixture that skips the runtime's own resolution is not standing in
// for the real thing.
//
// It calls domain.ResolveNavigationHeadings, the identical function the loader calls. Hand-writing Heading
// in each fixture was the alternative and is the inversion TestMachineFixturesPassProductionValidation
// rejects for RelatedMachine -- an inference the runtime makes is not a fixture's to know.
//
// internal/rendering may not import internal/metadata (plane boundary), which is exactly why the resolver
// lives in domain and not beside the loader.
func resolvedNav(items []domain.NavigationItem) []domain.NavigationItem {
	domain.ResolveNavigationHeadings(items)
	return items
}
