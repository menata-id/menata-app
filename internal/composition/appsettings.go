package composition

import (
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// ApplicationSettingsHub composes an Application's Settings hub (ROADMAP.md "In progress",
// "Application Settings hub"): which of its own real sections have anything to show.
//
// Deliberately two booleans, not a generic list of declared items to iterate: Phase 1's own
// 2026-09-25 correction is what this function exists to honor -- the Access section is derived
// from app.Roles and the viewer's own Workspace role here, never from a nav item's mere existence
// (an Application declaring nav_app_settings_permissions and later removing all its roles would
// otherwise leave a dead-looking row that nothing catches). A generic, metadata-iterated row list
// would also be exactly the "shape before need" this repo's own decomposition criteria warn
// against -- only one real Application has ever needed this hub.
func ApplicationSettingsHub(app domain.Application, isWorkspaceAdmin bool) rendering.SettingsHubView {
	showAccess := len(app.Roles) > 0
	return rendering.SettingsHubView{
		// ShowAccess is len(app.Roles) > 0 -- an Application declaring no roles has nothing to
		// configure access for, so the whole section is absent, not three empty-looking rows. Same
		// derivation internal/composition/rolematrix.go's own "Access" row already uses.
		ShowAccess: showAccess,
		// ShowMembersAndGroups additionally requires the viewer to be a Workspace admin -- hidden,
		// not shown-then-403, the same convention internal/rendering/appshell.templ's workspaceMenu
		// already uses for these same two destinations (its own `!= WorkspaceRoleMember` gate).
		// Permissions itself carries no such gate: it is read-only information for any member.
		ShowMembersAndGroups: showAccess && isWorkspaceAdmin,
	}
}
