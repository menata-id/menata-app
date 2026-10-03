package rendering

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestGridLayout_rendersTheClassStringEachCallSiteReplaced pins what `gridLayout` emits for every
// combination its five call sites pass, against the exact string each of them used to write by hand.
//
// **It exists because the live render-diff could not reach one of them.** The migration was verified by
// fetching nine screens from a worktree baseline and the current build and comparing every byte, which came
// back identical -- but `/approval-inbox` had no pending, draft or own documents in the dev database, so
// `pendingApprovalCardGrid` rendered nothing and its call site was covered by reading alone. Three of the
// five combinations were genuinely exercised; the fourth and fifth were not.
//
// So the table below is the proof for all five, and it is a better one than a fetch: it survives the
// database being empty, and it fails if a future change to `layoutGap` or either column switch silently
// restyles a screen. The `want` column is the pre-migration literal, copied from git history rather than
// from the new renderer -- a `want` derived from the code under test would pass no matter what it emitted.
func TestGridLayout_rendersTheClassStringEachCallSiteReplaced(t *testing.T) {
	class := regexp.MustCompile(`class="([^"]*)"`)

	for _, tc := range []struct {
		site    string
		gap     domain.Gap
		mobile  domain.GridCols
		columns domain.GridCols
		want    string
	}{
		{"appshell.summaryCountTiles", domain.GapDefault, domain.GridCols2, domain.GridCols4, "grid grid-cols-2 gap-3 sm:grid-cols-4"},
		{"calendar.CalendarPage", domain.GapDefault, domain.GridCols1, domain.GridCols7, "grid grid-cols-1 gap-3 sm:grid-cols-7"},
		{"approvalinbox.pendingApprovalCardGrid", domain.GapComfortable, domain.GridCols1, domain.GridCols2, "grid grid-cols-1 gap-4 sm:grid-cols-2"},
		{"groups.GroupsPage", domain.GapComfortable, domain.GridCols1, domain.GridCols2, "grid grid-cols-1 gap-4 sm:grid-cols-2"},
		{"machine.cardsLayout", domain.GapDefault, domain.GridCols1, domain.GridCols2, "grid grid-cols-1 gap-3 sm:grid-cols-2"},
	} {
		var buf bytes.Buffer
		if err := gridLayout(tc.gap, tc.mobile, tc.columns).Render(context.Background(), &buf); err != nil {
			t.Fatalf("%s: Render() error = %v", tc.site, err)
		}
		m := class.FindStringSubmatch(buf.String())
		if m == nil {
			t.Errorf("%s: rendered no class attribute; got %q", tc.site, buf.String())
			continue
		}
		if m[1] != tc.want {
			t.Errorf("%s: gridLayout emitted %q, but that call site rendered %q before the migration", tc.site, m[1], tc.want)
		}
	}
}

// TestSplitLayout_rendersTheClassStringEachCallSiteReplaced is `grid`'s counterpart, and it carries more
// weight because **two of `split`'s five sites deliberately render differently now**. The `want` column is
// therefore the *intended* class string, with the pre-migration literal beside it where they differ, so a
// reader can see which screens moved and by how much without going to git.
//
// Live coverage reached four of the five: `/documents/new`, `/new-application`,
// `/machines/{id}/records/{id}/review` and `.../signature-placement` were all fetched from a worktree
// baseline and this build and compared byte for byte. The fifth, New Application's **review** pane, needs a
// generated AI session in a Workspace this credential can enter, and fabricating one in the shared dev
// database is not worth it -- so that site is covered by this pin plus the fact that it passes the same three
// arguments as the conversation pane, which *was* fetched.
func TestSplitLayout_rendersTheClassStringEachCallSiteReplaced(t *testing.T) {
	class := regexp.MustCompile(`class="([^"]*)"`)

	for _, tc := range []struct {
		site  string
		gap   domain.Gap
		side  domain.SplitSide
		aside domain.AsideWidth
		want  string
		was   string // "" when the migration left this site byte-identical
	}{
		{"documentsubmit.DocumentSubmitPage", domain.GapComfortable, domain.SplitAsideStart, domain.AsideNarrow,
			"grid grid-cols-1 gap-4 lg:grid-cols-[320px_minmax(0,1fr)]", ""},
		{"reviewdocument.ReviewDocumentPage", domain.GapComfortable, domain.SplitAsideEnd, domain.AsideNarrow,
			"grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_320px]", ""},
		{"signatureplacement.SignaturePlacementPage", domain.GapComfortable, domain.SplitAsideEnd, domain.AsideNarrow,
			"grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_320px]",
			"grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_316px]"}, // aside +4px
		{"newapplication.ConversationPage", domain.GapLoose, domain.SplitAsideEnd, domain.AsideWide,
			"grid grid-cols-1 gap-5 lg:grid-cols-[minmax(0,1fr)_360px]",
			"grid grid-cols-1 gap-5 lg:grid-cols-[1fr_360px]"}, // main column may now shrink
		{"newapplication.ReviewPage", domain.GapLoose, domain.SplitAsideEnd, domain.AsideWide,
			"grid grid-cols-1 gap-5 lg:grid-cols-[minmax(0,1fr)_360px]",
			"grid grid-cols-1 gap-5 lg:grid-cols-[1fr_340px]"}, // aside +20px, and may now shrink
	} {
		var buf bytes.Buffer
		if err := splitLayout(tc.gap, tc.side, tc.aside).Render(context.Background(), &buf); err != nil {
			t.Fatalf("%s: Render() error = %v", tc.site, err)
		}
		m := class.FindStringSubmatch(buf.String())
		if m == nil {
			t.Errorf("%s: rendered no class attribute; got %q", tc.site, buf.String())
			continue
		}
		if m[1] != tc.want {
			t.Errorf("%s: splitLayout emitted %q, want %q", tc.site, m[1], tc.want)
		}
		if tc.was != "" && tc.was == tc.want {
			t.Errorf("%s: the `was` column equals `want`, so this row records no change -- drop it or correct it", tc.site)
		}
	}
}

// TestSplitLayout_unmeasuredCombinationFallsBackRatherThanEmittingANewClass is the split-side twin of the
// grid fallback below. Three of the four (side, width) combinations have a site; a start-side wide aside has
// none, so drawing it would put an arbitrary-value class in the bundle for nothing.
func TestSplitLayout_unmeasuredCombinationFallsBackRatherThanEmittingANewClass(t *testing.T) {
	var buf bytes.Buffer
	if err := splitLayout(domain.GapDefault, domain.SplitAsideStart, domain.AsideWide).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "lg:grid-cols-[minmax(0,1fr)_320px]") {
		t.Errorf("an unmeasured (side, width) combination should fall back to the most common track list, not emit a new class; got %q", got)
	}
}

// TestGridLayout_unmeasuredColumnCountFallsBackRatherThanEmittingANewClass documents the one footgun
// `domain.GridCols` names in its own comment, by asserting it rather than leaving it to be discovered.
//
// A count is drawn only at the breakpoint where the corpus measures it: one and two below `sm:`, two, four
// and seven from `sm:` up. Passing a desktop-only count as the mobile one therefore renders single-column
// instead of a new utility class. That is deliberate -- the alternative ships classes no screen uses, which
// is the defect the first pass of this work actually shipped -- but it is the kind of deliberate that reads
// as a bug to the next person, so it is written down as a test.
func TestGridLayout_unmeasuredColumnCountFallsBackRatherThanEmittingANewClass(t *testing.T) {
	var buf bytes.Buffer
	if err := gridLayout(domain.GapDefault, domain.GridCols7, domain.GridCols2).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := buf.String(); !regexp.MustCompile(`class="grid grid-cols-1 `).MatchString(got) {
		t.Errorf("a mobile count the corpus does not measure should fall back to single-column, not emit a new class; got %q", got)
	}
}
