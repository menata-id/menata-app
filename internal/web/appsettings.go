package web

import "net/http"

// showApplicationSettingsHub and showApplicationPermissions are registered ahead of their real
// content (ROADMAP.md "In progress", "Application Settings hub", Phase 1) purely so
// TestNavigationRoutesAreRegistered has a handler to find for nav_app_settings/
// nav_app_settings_permissions, declared in the same change (metadata/applications/document-
// approval.yaml). Phase 3 replaces both bodies with the real hub/Permissions pages.
//
// Neither route is reachable from the running app yet: appshell.templ's defaultAppMenu already
// excludes both from the tab strip/bottom bar (SettingsHub/SettingsHubMember), and Phase 4 is what
// adds the chrome's own "Settings" link that would otherwise point here. A member who types either
// URL directly sees an honest 501, not a page pretending to work.
func showApplicationSettingsHub(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Application Settings hub is under construction (ROADMAP.md \"Application Settings hub\", Phase 3)", http.StatusNotImplemented)
}

func showApplicationPermissions(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Permissions page is under construction (ROADMAP.md \"Application Settings hub\", Phase 3)", http.StatusNotImplemented)
}
