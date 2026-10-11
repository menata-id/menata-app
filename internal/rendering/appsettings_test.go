package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func renderSettingsHub(t *testing.T, hub SettingsHubView, section string) string {
	t.Helper()
	app := domain.Application{ID: "app_tally", Name: hub.AppName}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Applications: []domain.Application{app}}, "Tallyville", false)
	ctx = WithCurrentApplication(ctx, app)
	var buf bytes.Buffer
	if err := ApplicationSettingsPage(hub, RoleMatrixApp{}, section, "Tallyville", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// A fictional Application with no roles and one declared member: the page draws that member's row, an
// About with the Application's own name, no Access section, no role matrix -- and nothing that belongs
// to Document Approval. Before S2.1 the page carried a "Document types" placeholder, a Notifications
// row and an Approval flow row for every Application that had a hub.
func TestApplicationSettingsPage_drawsAnyApplicationFromItsView(t *testing.T) {
	html := renderSettingsHub(t, SettingsHubView{
		HubRoute: "/settings/nav_t", Title: "Tally settings", Description: "Applies to Tally only.",
		Members: []SettingsMember{{Href: "/machines/mch_unit", Label: "Units", Description: "What a count is measured in."}},
		AppName: "Tally", AppDescription: "Counts things.", MachineCount: 2,
	}, "")

	for _, want := range []string{"Tally settings", `href="/machines/mch_unit"`, "Units", "What a count is measured in.", "Configuration", "About", "Tally", "Counts things.", "2 machines", "Workspace settings"} {
		if !strings.Contains(html, want) {
			t.Errorf("page missing %q", want)
		}
	}
	for _, banned := range []string{"Access", "Permissions", "Document", "Which emails you receive", "Not built yet", "Approval flow"} {
		if strings.Contains(html, banned) {
			t.Errorf("page for an Application with no roles and one member contains %q", banned)
		}
	}
}

func TestApplicationSettingsPage_accessSectionAppearsWithRoles(t *testing.T) {
	hub := SettingsHubView{HubRoute: "/settings/nav_t", Title: "Tally settings", ShowAccess: true, ShowMembersAndGroups: true, AppName: "Tally", RoleCount: 3}
	html := renderSettingsHub(t, hub, "")
	for _, want := range []string{"Access", "Permissions", `href="/settings/nav_t?section=permissions"`, "Who can use this application", "3 roles"} {
		if !strings.Contains(html, want) {
			t.Errorf("page missing %q", want)
		}
	}
	hub.ShowMembersAndGroups = false
	if html := renderSettingsHub(t, hub, ""); strings.Contains(html, "Who can use this application") {
		t.Error("a non-admin is offered Members & roles")
	}
}

func TestApplicationSettingsPage_permissionsViewHasABackLinkToTheHub(t *testing.T) {
	hub := SettingsHubView{HubRoute: "/settings/nav_t", Title: "Tally settings", ShowAccess: true, AppName: "Tally"}
	html := renderSettingsHub(t, hub, permissionsSection)
	if !strings.Contains(html, `href="/settings/nav_t"`) || !strings.Contains(html, "← Tally settings") {
		t.Error("the mobile Permissions view lost its link back to the hub")
	}
	// ?section=permissions on an Application with no roles is the landing view, not an empty matrix.
	hub.ShowAccess = false
	if html := renderSettingsHub(t, hub, permissionsSection); strings.Contains(html, "← Tally settings") {
		t.Error("an Application with no roles drew a Permissions view")
	}
}

// Board M06a: Communication holds one honest placeholder, and only an Application with roles has it -- the
// role-less page above bans "Not built yet" outright.
func TestApplicationSettingsPage_communicationIsAPlaceholderOnlyWithRoles(t *testing.T) {
	hub := SettingsHubView{HubRoute: "/settings/nav_t", Title: "Tally settings", ShowAccess: true, AppName: "Tally"}
	html := renderSettingsHub(t, hub, "")
	// The phone's list and the desktop sidebar each draw it, and the page renders both.
	if n := strings.Count(html, "Not built yet"); n != 2 {
		t.Errorf("want the Communication placeholder in the list and in the sidebar, got %d", n)
	}
	if strings.Contains(html, `href="/settings/nav_t/notifications"`) {
		t.Error("the Notifications placeholder became a link to nowhere")
	}
}

// Board RoleMatrix: the desktop pane carries its own heading and the sentence that a role means nothing
// elsewhere; the card is titled for the roles, and links Members & roles only for a viewer who may open it.
func TestApplicationSettingsPage_permissionsPaneNamesTheApplicationAndLinksMembersOnlyForAdmins(t *testing.T) {
	hub := SettingsHubView{HubRoute: "/settings/nav_t", Title: "Tally settings", ShowAccess: true, ShowMembersAndGroups: true,
		MembersHref: "/settings/nav_t/members", AppName: "Tally"}
	html := renderSettingsHub(t, hub, "")
	for _, want := range []string{"Who can do what in Tally. A role here means nothing in other applications.", "Roles in this application", "Some rules aren’t this application’s to decide"} {
		if !strings.Contains(html, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if !strings.Contains(strings.ReplaceAll(html, "\n", ""), `href="/settings/nav_t/members"`) {
		t.Error("an admin has no link to the Application's Members & roles page")
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "T", false)
	var card bytes.Buffer
	if err := roleMatrixApp(RoleMatrixApp{Roles: []string{"a"}, RolesSummary: "A"}, "").Render(ctx, &card); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(card.String(), "href=") {
		t.Error("the roles card linked Members & roles for a viewer who may not open it")
	}
}
