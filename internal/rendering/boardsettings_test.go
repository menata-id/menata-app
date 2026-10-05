package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func renderBoardSettings(t *testing.T, c BoardSettingsContent) string {
	t.Helper()
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Navigation: resolvedNav([]domain.NavigationItem{
		{ID: "nav_board_settings", Label: "Board Settings", Description: "Lists and labels.", Route: "/board-settings"},
	})}, "Test Workspace", false)
	var buf bytes.Buffer
	if err := BoardSettingsPage(c, "Acme", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

func TestBoardSettingsPage_drawsListsAndLabelsWithUsage(t *testing.T) {
	html := renderBoardSettings(t, BoardSettingsContent{
		Lists:      []SettingsList{{Name: "Backlog"}, {Name: "Done"}},
		Labels:     []SettingsLabel{{Name: "Location", Color: domain.TagBlue, Records: 3}, {Name: "Legal", Color: domain.TagRose, Records: 1}, {Name: "Spare", Color: domain.TagSlate}},
		ListsHref:  "/machines/mch_list",
		LabelsHref: "/machines/mch_label",
	})
	for _, want := range []string{
		"Backlog", "Done", "Location", "bg-blue-600", "used on 3 cards", "used on 1 card<", "used on 0 cards",
		`href="/machines/mch_list"`, `href="/machines/mch_label"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("board settings is missing %q", want)
		}
	}
	if strings.Contains(html, "Showing the first") {
		t.Error("selections inside their bounds drew a truncation notice")
	}
}

func TestBoardSettingsPage_saysSoWhenThereIsNothingYet(t *testing.T) {
	html := renderBoardSettings(t, BoardSettingsContent{})
	if !strings.Contains(html, "No lists yet.") || !strings.Contains(html, "No labels yet.") {
		t.Errorf("an empty Workspace drew neither empty state: %s", html)
	}
}

func TestBoardSettingsPage_saysSoWhenASelectionWasCut(t *testing.T) {
	html := renderBoardSettings(t, BoardSettingsContent{LabelsTruncation: Truncation{Limit: 200, Hit: true}})
	if !strings.Contains(html, "Showing the first") || !strings.Contains(html, "200") {
		t.Error("a truncated label list did not say so")
	}
}
