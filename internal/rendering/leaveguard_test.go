package rendering

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func renderLeaveGuardShell(t *testing.T, workspaceRole, switchHref string) string {
	t.Helper()
	all := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_workspace_groups", Label: "Groups", Route: "/workspace-groups"},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: all}, "Test Workspace", false)
	var buf bytes.Buffer
	if err := appShell("Home", "Dokter Kecil", Viewer{Initials: "AN", WorkspaceRole: workspaceRole}, switchHref).Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

func renderLeaveGuardInsideAnApplication(t *testing.T) string {
	t.Helper()
	items := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_one", Label: "One", Route: "/one"},
		{ID: "nav_two", Label: "Two", Route: "/two"},
	}
	app := domain.Application{ID: "app_x", Name: "Docs", Navigation: items, AllNavigation: items}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: items}, "Test Workspace", false)
	ctx = WithCurrentApplication(ctx, app)
	ctx = WithCurrentPath(ctx, "/one", "")
	var buf bytes.Buffer
	if err := appShell("Home", "Dokter Kecil", Viewer{Initials: "AN", WorkspaceRole: "admin"}, "/switch-workspace").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

func TestLeaveGuard_drawnOnlyWhereTheShellOffersASwitch(t *testing.T) {
	with := renderLeaveGuardShell(t, "admin", "/switch-workspace")
	if !strings.Contains(with, `id="menata-leave-guard"`) {
		t.Fatal("a shell offering the switch link must carry the leave dialog")
	}
	without := renderLeaveGuardShell(t, "admin", "")
	if strings.Contains(without, "menata-leave-guard") {
		t.Error("a shell with no switch link must not carry a dialog nothing can open")
	}
}

func TestLeaveGuard_everySwitchLinkIsMarkedAndNoneIsLeftUnguarded(t *testing.T) {
	// The picker is offered once, by the 9-dot launcher, on a Workspace screen and inside an Application alike: the mobile "More" sheet and
	// the avatar menu stopped repeating it (owner, 2026-10-22). Both shells are still drawn, so a fixture of only one cannot leave the other
	// unchecked.
	for name, c := range map[string]struct {
		out  string
		want int
	}{
		"workspace screen":   {renderLeaveGuardShell(t, "admin", "/switch-workspace"), 1},
		"application screen": {renderLeaveGuardInsideAnApplication(t), 1},
	} {
		anchors := regexp.MustCompile(`<a [^>]*href="/switch-workspace"[^>]*>`).FindAllString(c.out, -1)
		if len(anchors) < c.want {
			t.Fatalf("%s: expected the picker to be linked at least %d time(s), found %d", name, c.want, len(anchors))
		}
		for _, a := range anchors {
			if !strings.Contains(a, "data-leave-guard") {
				t.Errorf("%s: a link to the picker leaves unsaved input behind without asking: %s", name, a)
			}
		}
	}
}

func TestLeaveGuard_namesTheWorkspaceBeingLeftAndTheWayOut(t *testing.T) {
	out := renderLeaveGuardShell(t, "admin", "/switch-workspace")
	for _, want := range []string{
		"Leave Dokter Kecil?",
		"Stay in Dokter Kecil",
		"Leave and switch",
		`<form method="get" action="/switch-workspace"`,
		`<form method="dialog"`,
		`>admin<`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dialog is missing %q", want)
		}
	}
}

func TestLeaveGuard_roleBadgeIsOmittedForAnIdentityWithNoMembership(t *testing.T) {
	out := renderLeaveGuardShell(t, "", "/switch-workspace")
	start := strings.Index(out, `id="menata-leave-guard"`)
	end := strings.Index(out[start:], "</dialog>")
	if strings.Contains(out[start:start+end], "—") {
		t.Error("an empty role must not draw a dash badge in the dialog")
	}
}

func TestLeaveGuard_scriptCountsOnlyAPersonsEditsToAWriteForm(t *testing.T) {
	out := renderLeaveGuardShell(t, "member", "/switch-workspace")
	for _, want := range []string{"e.isTrusted", `"hidden"`, `closest("dialog")`, `"submit"`, "isConnected", `a[data-leave-guard]`} {
		if !strings.Contains(out, want) {
			t.Errorf("guard script lost %q", want)
		}
	}
}

func TestRoleTone(t *testing.T) {
	if roleTone(domain.WorkspaceRoleAdmin) != domain.ToneInfo || roleTone(domain.WorkspaceRoleMember) != domain.ToneNeutral {
		t.Error("admin is drawn in the info tone, member in neutral")
	}
}

// Each area of the chrome answers one question (owner, 2026-10-22): the 9-dot launcher is where to go (Home, the Applications, the Workspace
// picker), the avatar is who is signed in, and the bottom bar is where you are inside this Application. So the avatar menu holds the account
// and nothing else, and the "More" sheet holds the Application's own screens and settings -- neither repeats the launcher.
func TestChrome_eachAreaHoldsItsOwnThingsOnly(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_one", Label: "One", Route: "/one", Icon: "board"},
		{ID: "nav_two", Label: "Two", Route: "/two", Icon: "board"},
		{ID: "nav_three", Label: "Three", Route: "/three", Icon: "board"},
		{ID: "nav_four", Label: "Four", Route: "/four", Icon: "board"},
	}
	app := domain.Application{ID: "app_x", Name: "Docs", Navigation: items[1:], AllNavigation: items[1:]}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: items}, "Test Workspace", false)
	ctx = WithCurrentApplication(ctx, app)
	ctx = WithCurrentPath(ctx, "/one", "")
	viewer := Viewer{Initials: "AN", Name: "Ana", Email: "ana@example.com", WorkspaceRole: "admin", Applications: []domain.Application{app, {ID: "app_y", Name: "Other App"}}}
	var buf bytes.Buffer
	if err := appShell("One", "Dokter Kecil", viewer, "/switch-workspace").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	section := func(id string) string {
		i := strings.Index(html, `id="`+id+`"`)
		if i < 0 {
			t.Fatalf("no %s in the shell", id)
		}
		return html[i : i+strings.Index(html[i:], "</div></div>")]
	}
	avatar := section("menata-account-menu")
	for _, want := range []string{"Profile", "Notifications", "Security", "Sign out"} {
		if !strings.Contains(avatar, want) {
			t.Errorf("the avatar menu lost %q", want)
		}
	}
	for _, gone := range []string{"Workspaces", "Switch workspace", "account-menu-workspaces", "/api/account-menu/workspaces"} {
		if strings.Contains(avatar, gone) {
			t.Errorf("the avatar menu still holds %q, which is the launcher's", gone)
		}
	}
	more := section("menata-more-sheet")
	if !strings.Contains(more, ">Four<") {
		t.Errorf("the More sheet does not hold the screen the three tabs have no room for:\n%s", more)
	}
	for _, gone := range []string{"Other App", "All Workspaces", "Dokter Kecil", ">Home<"} {
		if strings.Contains(more, gone) {
			t.Errorf("the More sheet still holds %q, which is the launcher's", gone)
		}
	}
	for _, shown := range []string{">One<", ">Two<", ">Three<"} {
		if strings.Contains(more, shown) {
			t.Errorf("the More sheet repeats %q, a tab the bar already draws", shown)
		}
	}
}

// An Application of three screens and no settings has a bar of three: a "More" tab with nothing behind it is not drawn.
func TestBottomBar_noMoreTabWhenThereIsNothingForItToHold(t *testing.T) {
	items := []domain.NavigationItem{
		{ID: "nav_home", Label: "Home", Route: "/home"},
		{ID: "nav_one", Label: "One", Route: "/one", Icon: "board"},
		{ID: "nav_two", Label: "Two", Route: "/two", Icon: "board"},
	}
	app := domain.Application{ID: "app_x", Name: "Docs", Navigation: items[1:], AllNavigation: items[1:]}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: items}, "Test Workspace", false)
	ctx = WithCurrentApplication(ctx, app)
	ctx = WithCurrentPath(ctx, "/one", "")
	var buf bytes.Buffer
	if err := appShell("One", "Dokter Kecil", Viewer{Initials: "AN", WorkspaceRole: "admin"}, "/switch-workspace").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "menata-more-sheet") {
		t.Error("a More tab is drawn for an Application with nothing to put behind it")
	}
}
