package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// The page is named after the menu entry it is, and a screen no entry matches (the first entry stands in to keep a tab lit) is named after none.
func TestActiveMenuNode_isTheEntryThePageIsAndNeverTheStandIn(t *testing.T) {
	if got := activeMenuNode([]navMenuItem{{ID: "nav_a"}, {ID: "nav_b", Active: true}}); got != "navigation/nav_b" {
		t.Errorf("got %q", got)
	}
	if got := activeMenuNode([]navMenuItem{{ID: "nav_a", Active: true, Fallback: true}, {ID: "nav_b"}}); got != "" {
		t.Errorf("a stand-in was named: %q", got)
	}
	items := []domain.NavigationItem{{ID: "nav_a", Label: "A", Route: "/a"}, {ID: "nav_b", Label: "B", Route: "/b"}}
	menu := defaultAppMenu(items, "/somewhere/else", "")
	if len(menu) == 0 || !menu[0].Active || !menu[0].Fallback {
		t.Fatalf("a path no entry matches should light the first entry as a stand-in: %+v", menu)
	}
	if menu = defaultAppMenu(items, "/b", ""); menu[1].Fallback || !menu[1].Active {
		t.Errorf("a path an entry matches is that entry, not a stand-in: %+v", menu)
	}
}

// A Field names the metadata path it edits in a builder preview and says nothing otherwise.
func TestField_namesItsMetadataPathOnlyInABuilderPreview(t *testing.T) {
	draw := func(ctx context.Context) string {
		var buf bytes.Buffer
		if err := field("c1", "Name", "", "", "machines/mch_x/fields/fld_name").Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	app := domain.Application{ID: "app_x"}
	in := WithBuilderMode(WithCurrentApplication(context.Background(), app))
	if got := draw(in); !strings.Contains(got, `data-node-path="app_x/machines/mch_x/fields/fld_name"`) {
		t.Errorf("builder preview: %s", got)
	}
	if got := draw(WithCurrentApplication(context.Background(), app)); strings.Contains(got, "data-node-path") {
		t.Errorf("an ordinary request: %s", got)
	}
}
