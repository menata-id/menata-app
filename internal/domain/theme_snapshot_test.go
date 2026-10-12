package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultThemeMatchesItsRecordedSnapshot is GS4 of menata-app-document's guides/menjaga-design-system.md. It
// holds the rule four of that guide's eleven inconsistencies broke: **folding, normalising or replacing a value is a
// decision, not a tidy-up** (R2). `DefaultTheme` is what every Workspace without a `theme:` block renders. That is
// every installed Workspace today, so one edited line here restyles the whole product, and nothing else in the suite
// notices when it does.
//
// The snapshot is `testdata/default_theme.json`: every role to its step, as `encoding/json` writes it (map keys
// sorted, so the file is stable). Any difference fails. The way to pass is to make the change on purpose:
//  1. record the decision, with its reversal condition, in guides/design-system-decisions.md;
//  2. mirror it in the Design System artifact (R8);
//  3. regenerate the snapshot with UPDATE_DEFAULT_THEME=1.
//
// The commit then carries the decision and the moved snapshot together.
//
// **What it does not hold:** the reader switches in internal/rendering (step → class), and the closed sets beside
// them. A step mapped to a different class is a reader change. TestThemeColourPairsMeetContrast measures the colour
// half of that, and nothing yet holds the size half.
func TestDefaultThemeMatchesItsRecordedSnapshot(t *testing.T) {
	got, err := json.MarshalIndent(DefaultTheme(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "default_theme.json")
	if os.Getenv("UPDATE_DEFAULT_THEME") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s -- the commit must carry the decision that moved it", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (generate it once with UPDATE_DEFAULT_THEME=1)", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("DefaultTheme differs from %s.\n"+
			"Every Workspace without a theme: block renders DefaultTheme, so this restyles the product. If it is "+
			"intended: (1) record the decision and its reversal condition in menata-app-document's "+
			"guides/design-system-decisions.md, (2) mirror it in the Design System artifact, (3) rerun with "+
			"UPDATE_DEFAULT_THEME=1.\n--- got ---\n%s", path, got)
	}
}
