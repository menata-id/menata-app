package composition

import (
	"menata.app/internal/domain"
	"menata.app/internal/rendering"
)

// ApplicationSettingsHub composes an Application's Settings hub (ROADMAP.md S2.1) from what the
// Application declares about itself, so the hub names no Application:
//
//   - the heading and description are the hub item's own (domain.NavigationItem.Heading/Description);
//   - Access exists exactly when the Application declares `roles:` -- derived from app.Roles and the
//     viewer's own Workspace role, never from a nav item's mere existence (an Application removing
//     all its roles would otherwise leave a dead-looking row nothing catches);
//   - Configuration is one row per `settings_hub_member` item, in declaration order, each reading its
//     own route, label and description (001 #3: a row is added by declaring an item, not by editing
//     a template);
//   - About is the Application's own name, description and counts. An install date and the template
//     an Application came from are not stored anywhere, so the hub says nothing about them rather
//     than inventing a value.
func ApplicationSettingsHub(app domain.Application, hub domain.NavigationItem, isWorkspaceAdmin bool) rendering.SettingsHubView {
	showAccess := len(app.Roles) > 0
	view := rendering.SettingsHubView{
		HubRoute:       hub.Route,
		Title:          hub.Heading,
		Description:    hub.Description,
		ShowAccess:     showAccess,
		AppName:        app.Name,
		AppDescription: app.Description,
		MachineCount:   len(app.Machines),
		RoleCount:      len(app.Roles),
		// Members & roles and Groups are admin destinations: hidden rather than offered-then-403, the
		// convention appshell.templ's workspaceMenu already uses for the same two screens.
		ShowMembersAndGroups: showAccess && isWorkspaceAdmin,
	}
	for _, item := range app.AllNavigation {
		if item.SettingsHubMember {
			view.Members = append(view.Members, rendering.SettingsMember{Href: item.Route, Label: item.Label, Description: item.Description})
		}
	}
	return view
}

// SettingsHubOf finds the Settings hub item of app that has the given navigation id, or false. It
// searches the unfiltered navigation (AllNavigation), the list routeByID reads.
func SettingsHubOf(app domain.Application, navID string) (domain.NavigationItem, bool) {
	for _, item := range app.AllNavigation {
		if item.SettingsHub && item.ID == navID {
			return item, true
		}
	}
	return domain.NavigationItem{}, false
}
