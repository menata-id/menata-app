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
	// Outside an Application the picker is offered once (the launcher); inside one the mobile "More" sheet adds a second. A fixture
	// drawing only one of the two shells would leave the other unchecked.
	for name, c := range map[string]struct {
		out  string
		want int
	}{
		"workspace screen":   {renderLeaveGuardShell(t, "admin", "/switch-workspace"), 1},
		"application screen": {renderLeaveGuardInsideAnApplication(t), 2},
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
