package rendering

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
	"menata.app/internal/experience"
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

// TestRowLayout_rendersTheClassStringEachCallSiteReplaced pins the three argument combinations `row`'s 22
// migrated sites use, in the class order all 22 already wrote: `flex flex-wrap`, each axis that is not its
// default, then the gap.
//
// **The double-space case is the one worth having a test for.** templ's `class={ a, b, c }` does not drop an
// empty value, it emits two spaces -- measured by rendering the combinations before writing any call site,
// which is why `rowClasses` joins in Go. Eighteen of the 22 pass at least one default, so the naive form
// would have left eighteen screens differing from their baseline by a space nobody would think to look for.
// Each `want` below is the exact pre-migration literal.
func TestRowLayout_rendersTheClassStringEachCallSiteReplaced(t *testing.T) {
	class := regexp.MustCompile(`class="([^"]*)"`)

	for _, tc := range []struct {
		shape   string
		sites   int
		gap     domain.Gap
		align   domain.RowAlign
		justify domain.RowJustify
		want    string
	}{
		{"centred, spread apart", 6, domain.GapDefault, domain.RowAlignCenter, domain.RowJustifySpread,
			"flex flex-wrap items-center justify-between gap-3"},
		{"centred", 7, domain.GapTight, domain.RowAlignCenter, domain.RowJustifyStart,
			"flex flex-wrap items-center gap-2"},
		{"plain wrapping flow", 9, domain.GapTight, domain.RowAlignStretch, domain.RowJustifyStart,
			"flex flex-wrap gap-2"},
	} {
		var buf bytes.Buffer
		if err := rowLayout(tc.gap, tc.align, tc.justify).Render(context.Background(), &buf); err != nil {
			t.Fatalf("%s: Render() error = %v", tc.shape, err)
		}
		m := class.FindStringSubmatch(buf.String())
		if m == nil {
			t.Errorf("%s: rendered no class attribute; got %q", tc.shape, buf.String())
			continue
		}
		if m[1] != tc.want {
			t.Errorf("%s (%d sites): rowLayout emitted %q, want %q", tc.shape, tc.sites, m[1], tc.want)
		}
		if strings.Contains(m[1], "  ") {
			t.Errorf("%s: class attribute has a double space (%q) -- an empty axis value is being joined instead of skipped", tc.shape, m[1])
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

// TestStatusBadge_rendersEachDeclaredTone pins the registered Component's output per tone, and
// TestSlaBadge_rendersNothingWhenAbsent pins the one conditional in its adapter.
//
// **These carry the whole verification of the markup change, because the live diff could not reach it.**
// Thirteen screens were fetched from a worktree baseline and all thirteen came back token-identical -- no
// record in the dev database holds a non-empty due date, so the badge has never rendered in this environment.
// The change is real: `slaBadgePill` drew overdue as red *text* with no pill and everything else as a slate
// pill a half-step tighter than this one. Overdue is now a red pill, which is what `domain.ToneBad` already
// meant on the eight sites that were already using the shared shape.
func TestStatusBadge_rendersEachDeclaredTone(t *testing.T) {
	want := map[domain.BadgeTone]string{
		domain.ToneNeutral: "bg-slate-100 text-slate-600",
		domain.ToneInfo:    "bg-blue-50 text-blue-700",
		domain.ToneGood:    "bg-emerald-50 text-emerald-700",
		domain.ToneBad:     "bg-red-50 text-red-700",
		domain.ToneWarn:    "bg-amber-50 text-amber-800",
		domain.ToneMuted:   "bg-slate-50 text-slate-400",
	}
	if len(want) != len(domain.KnownBadgeTones) {
		t.Fatalf("this test pins %d tones and domain.KnownBadgeTones declares %d -- add the new one here with the classes it draws", len(want), len(domain.KnownBadgeTones))
	}
	for tone, classes := range want {
		var buf bytes.Buffer
		if err := statusBadge("OVERDUE", tone, domain.BadgeRegular).Render(context.Background(), &buf); err != nil {
			t.Fatalf("%s: Render() error = %v", tone, err)
		}
		got := buf.String()
		if !strings.Contains(got, classes) {
			t.Errorf("statusBadge(_, %s) does not carry %q; got %q", tone, classes, got)
		}
		if !strings.Contains(got, ">OVERDUE<") {
			t.Errorf("statusBadge(%q, %s) did not render its label; got %q", "OVERDUE", tone, got)
		}
	}
}

// TestStatusBadge_sizeChoosesTheRoomAroundTheLabel pins what the 2026-10-05 chip slice could not diff: the
// six compact chips sit on screens whose dev data renders none of them (my-tasks, the activity feed, the
// workspace home), so the exact class set each size draws is held here instead. The literals are the
// pre-migration ones -- `px-2 py-0.5` for the six dense-row chips, `px-2.5 py-1` for the nine that were the
// subject of their line -- so the test reads without git.
func TestStatusBadge_sizeChoosesTheRoomAroundTheLabel(t *testing.T) {
	for _, tc := range []struct {
		size domain.BadgeSize
		want string
		not  string
	}{
		{domain.BadgeCompact, "px-2 py-0.5", "px-2.5"},
		{domain.BadgeRegular, "px-2.5 py-1", "py-0.5"},
		{"", "px-2.5 py-1", "py-0.5"}, // an unset size is regular, never "no padding"
		{"jumbo", "px-2.5 py-1", "py-0.5"},
	} {
		var buf bytes.Buffer
		if err := statusBadge("3", domain.ToneNeutral, tc.size).Render(context.Background(), &buf); err != nil {
			t.Fatalf("%q: Render() error = %v", tc.size, err)
		}
		got := buf.String()
		if !strings.Contains(got, tc.want) || strings.Contains(got, tc.not) {
			t.Errorf("statusBadge(_, _, %q) should carry %q and not %q; got %q", tc.size, tc.want, tc.not, got)
		}
		if !strings.Contains(got, "text-2xs rounded-full bg-slate-100 text-slate-600") {
			t.Errorf("statusBadge(_, _, %q) drew the wrong type/shape/colour; got %q", tc.size, got)
		}
	}
}

// TestStatusBadge_followsTheWorkspaceTheme is the proof that a tone's colour is declarable and not just
// mapped: the same tone renders a different palette under a Workspace that declares one, with no recompile.
// It also pins the property that makes `theme.tone` safe to partially declare -- remapping one tone leaves the
// other five exactly as they were.
func TestStatusBadge_followsTheWorkspaceTheme(t *testing.T) {
	ws := domain.Workspace{Slug: "test", Theme: domain.Theme{Tone: map[domain.BadgeTone]domain.TonePalette{
		domain.ToneWarn: domain.PaletteRed,
	}}}
	ctx := WithCurrentWorkspace(context.Background(), ws, "Test Workspace", false)

	render := func(tone domain.BadgeTone) string {
		var buf bytes.Buffer
		if err := statusBadge("X", tone, domain.BadgeRegular).Render(ctx, &buf); err != nil {
			t.Fatalf("%s: Render() error = %v", tone, err)
		}
		return buf.String()
	}
	if got := render(domain.ToneWarn); !strings.Contains(got, "bg-red-50 text-red-700") || strings.Contains(got, "amber") {
		t.Errorf("a declared warn=red should draw red and no amber; got %q", got)
	}
	if got := render(domain.ToneGood); !strings.Contains(got, "bg-emerald-50 text-emerald-700") {
		t.Errorf("an undeclared tone should keep its default under a partial theme; got %q", got)
	}
}

func TestSlaBadge_rendersNothingWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	if err := slaBadge(experience.SLABadge{}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := buf.String(); got != "" {
		t.Errorf("an absent SLA badge rendered %q -- a Field with no value must produce no markup, not an empty styled element", got)
	}

	buf.Reset()
	if err := slaBadge(experience.SLABadge{Label: "OVERDUE", Tone: domain.ToneBad, Present: true}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	// The deliberate change: overdue is a red *pill* now, not red text.
	if got := buf.String(); !strings.Contains(got, "rounded-full") || !strings.Contains(got, "bg-red-50 text-red-700") {
		t.Errorf("an overdue badge should render as a red pill through statusBadge; got %q", got)
	}
}

// TestAvatar_rendersEachCallSiteCombination pins the four combinations the migrated sites pass, and the
// accessible name the contract made required.
//
// **Three of the four were verified live and the fourth could not be.** `/workspace-members`, a Group's
// detail page and `/workspace-members/{id}/edit` were diffed against a worktree baseline: identical class
// *sets* (reordered only) plus the added `aria-label`/`title`. The `pending` variant renders only for an
// outstanding invitation and the dev database has zero, so it is pinned here instead -- the same gap that hit
// `pendingApprovalCardGrid`, New Application's review pane and the SLA badge. Check whether a diff can reach
// the code before reporting it as verification.
func TestAvatar_rendersEachCallSiteCombination(t *testing.T) {
	for _, tc := range []struct {
		site     string
		size     domain.AvatarSize
		presence domain.AvatarPresence
		want     []string
		absent   string
	}{
		{"workspacemembers (member row)", domain.AvatarInline, domain.AvatarPresent,
			[]string{"size-8", "text-2xs", "bg-slate-200", "text-slate-700"}, "border-dashed"},
		{"groups (member row)", domain.AvatarInline, domain.AvatarPresent,
			[]string{"size-8", "text-2xs", "bg-slate-200"}, "border-dashed"},
		{"workspacemembers (invited row)", domain.AvatarInline, domain.AvatarPending,
			[]string{"size-8", "border-dashed", "border-slate-300", "text-slate-400"}, "bg-slate-200"},
		{"workspacemembers (edit member header)", domain.AvatarLead, domain.AvatarPresent,
			[]string{"size-10", "text-sm", "bg-slate-200"}, "border-dashed"},
		{"board card (assignee)", domain.AvatarCompact, domain.AvatarPresent,
			[]string{"size-6", "text-3xs", "bg-slate-200"}, "border-dashed"},
	} {
		var buf bytes.Buffer
		if err := avatar("SI", "Silvia Indah Rini", tc.size, tc.presence).Render(context.Background(), &buf); err != nil {
			t.Fatalf("%s: Render() error = %v", tc.site, err)
		}
		got := buf.String()
		for _, cls := range tc.want {
			if !strings.Contains(got, cls) {
				t.Errorf("%s: avatar missing %q; got %q", tc.site, cls, got)
			}
		}
		if strings.Contains(got, tc.absent) {
			t.Errorf("%s: avatar carries %q, which belongs to the other %s", tc.site, tc.absent, "variant")
		}
		// The clause the second Component turned from prose into a requirement.
		if !strings.Contains(got, `aria-label="Silvia Indah Rini"`) || !strings.Contains(got, `title="Silvia Indah Rini"`) {
			t.Errorf("%s: avatar carries no accessible name -- \"SI\" is not a name; got %q", tc.site, got)
		}
		if !strings.Contains(got, ">SI<") {
			t.Errorf("%s: avatar did not render its initials; got %q", tc.site, got)
		}
	}
}

// TestCollection_rendersTheListShapeBothCallersHad pins §12.3's `Collection` against the two class strings its
// callers used, and covers the one the live diff could not reach.
//
// `activityFeedListRow` was exercised for real -- 30 items on /activity, 10 on /dashboard, byte-identical --
// but `taskRowList` rendered **zero** items because this credential has no assigned tasks. Same gap as four
// earlier slices, so it is pinned here and said out loud rather than counted as verified.
//
// The two class strings are measured, not chosen: both callers used exactly these, 2 of 2.
func TestCollection_rendersTheListShapeBothCallersHad(t *testing.T) {
	var buf bytes.Buffer
	items := []templ.Component{staticText(domain.StaticParagraph, "one"), staticText(domain.StaticParagraph, "two")}
	if err := collection(domain.GapTight, items).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `<ul class="flex flex-col gap-2">`) {
		t.Errorf("collection's list wrapper changed; got %q", got)
	}
	if n := strings.Count(got, `<li class="flex flex-wrap items-center gap-2 text-sm">`); n != 2 {
		t.Errorf("collection rendered %d item wrappers for 2 items; got %q", n, got)
	}
	if !strings.Contains(got, ">one<") || !strings.Contains(got, ">two<") {
		t.Errorf("collection did not render its slot children; got %q", got)
	}

	// An empty collection still renders the list, not nothing: a screen showing "no rows" decides that
	// itself, and a Component that vanished would make the empty state the Component's business.
	buf.Reset()
	if err := collection(domain.GapTight, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "<ul") || strings.Contains(got, "<li") {
		t.Errorf("an empty collection should render an empty list; got %q", got)
	}
}

// TestStaticLink_rendersAnAnchorNotLiteralText pins §12.6's `link` and the failure that nearly shipped.
//
// templ interprets `@component(...)` **only in child position**. Three of the eight sites this primitive was
// written for were `<a>` inline inside a text run, and templ rendered the call there as literal text -- the
// string `@staticLink("/register", "Create a workspace")` printed into the page, with a green build and a
// green conformance suite. Only the byte-diff caught it. Those three stay hand-written.
//
// This test cannot detect the inline-position case (it calls the component directly, which is always child
// position). It pins what the primitive emits; the byte-diff remains the only thing that catches a caller
// placing it where templ will not read it.
func TestStaticLink_rendersAnAnchorNotLiteralText(t *testing.T) {
	var buf bytes.Buffer
	if err := staticLink("/register", "Create a workspace").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if want := `<a href="/register" class="text-blue-600 hover:text-blue-700">Create a workspace</a>`; got != want {
		t.Errorf("staticLink emitted %q, want %q -- this is the pre-migration literal from git history, not from the renderer", got, want)
	}
	if strings.Contains(got, "@staticLink") {
		t.Error("staticLink rendered its own call as text")
	}
}

// TestMetric_inkFollowsTheWorkspaceTheme proves text colour is declarable, with no recompile, and that the
// default is exactly what the Metric hardcoded before (`slate-900` value, `slate-500` label, `slate-400` hint).
func TestMetric_inkFollowsTheWorkspaceTheme(t *testing.T) {
	render := func(ctx context.Context) string {
		var buf bytes.Buffer
		if err := metric("Open", "7", "since Monday", domain.ToneNeutral).Render(ctx, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	def := render(context.Background())
	for _, want := range []string{"text-slate-900", "text-xs text-slate-500", "text-2xs text-slate-400"} {
		if !strings.Contains(def, want) {
			t.Errorf("default Metric lost %q; got %q", want, def)
		}
	}

	ws := domain.Workspace{Slug: "test", Theme: domain.Theme{Ink: map[domain.InkRole]domain.InkShade{
		domain.InkSecondary: domain.InkShadeDark,
		domain.InkFaint:     domain.InkShadeMedium,
	}}}
	got := render(WithCurrentWorkspace(context.Background(), ws, "Test Workspace", false))
	if !strings.Contains(got, "text-xs text-slate-700") || !strings.Contains(got, "text-2xs text-slate-500") {
		t.Errorf("a declared ink theme should recolour the label and hint; got %q", got)
	}
	if !strings.Contains(got, "text-slate-900") {
		t.Errorf("an undeclared role (strong) should keep its default; got %q", got)
	}
}

// TestPanelLayout_backgroundFollowsTheWorkspaceTheme proves the raised surface is declarable with no
// recompile, and that the default is exactly the `bg-white` the panel hardcoded before.
func TestPanelLayout_backgroundFollowsTheWorkspaceTheme(t *testing.T) {
	render := func(ctx context.Context) string {
		var buf bytes.Buffer
		if err := panelLayout().Render(templ.WithChildren(ctx, templ.Raw("x")), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	if def := render(context.Background()); !strings.Contains(def, "bg-white") {
		t.Errorf("default panel lost bg-white; got %q", def)
	}
	ws := domain.Workspace{Slug: "test", Theme: domain.Theme{Background: map[domain.BackgroundRole]domain.BackgroundShade{
		domain.BackgroundRaised: domain.BackgroundShadeLighter,
	}}}
	got := render(WithCurrentWorkspace(context.Background(), ws, "Test Workspace", false))
	if !strings.Contains(got, "bg-slate-50") || strings.Contains(got, "bg-white") {
		t.Errorf("a declared raised=lighter should recolour the panel; got %q", got)
	}
}

func renderWithTheme(t *testing.T, th domain.Theme, c templ.Component) string {
	t.Helper()
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	var buf bytes.Buffer
	if err := c.Render(templ.WithChildren(ctx, templ.Raw("x")), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// TestPaddingFollowsTheWorkspaceTheme pins the **pre-migration literal beside the intended one** for all three
// primitives: the default must be byte-for-byte the class each hardcoded, and a declaration must replace it
// with no recompile. Section and panel stay different by default (D6) and become one by declaration.
func TestPaddingFollowsTheWorkspaceTheme(t *testing.T) {
	cases := []struct {
		name      string
		c         templ.Component
		was       string
		declared  map[domain.PaddingRole]domain.PaddingAmount
		wantAfter string
	}{
		{"panel", panelLayout(), "p-4", map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingPanel: domain.PaddingAmountFive}, "p-5"},
		{"section", sectionLayout(domain.GapDefault), "p-5", map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingSection: domain.PaddingAmountFour}, "p-4"},
		{"badge", statusBadge("x", domain.ToneNeutral, domain.BadgeRegular), "px-2.5 py-1", map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingBadgeX: domain.PaddingAmountThree, domain.PaddingBadgeY: domain.PaddingAmountTwo}, "px-3 py-2"},
		{"compact badge", statusBadge("x", domain.ToneNeutral, domain.BadgeCompact), "px-2 py-0.5", map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingBadgeCompactX: domain.PaddingAmountOneHalf, domain.PaddingBadgeCompactY: domain.PaddingAmountOne}, "px-1.5 py-1"},
	}
	for _, c := range cases {
		def := renderWithTheme(t, domain.Theme{}, c.c)
		if !strings.Contains(def, `"`+c.was+`"`) && !strings.Contains(def, " "+c.was+" ") && !strings.Contains(def, c.was+`"`) && !strings.Contains(def, `"`+c.was+" ") {
			t.Errorf("%s: default lost %q; got %q", c.name, c.was, def)
		}
		got := renderWithTheme(t, domain.Theme{Padding: c.declared}, c.c)
		if !strings.Contains(got, c.wantAfter) || strings.Contains(got, c.was) {
			t.Errorf("%s: a declared padding should replace %q with %q; got %q", c.name, c.was, c.wantAfter, got)
		}
	}
}

// TestPaddingClassCoversEveryLadderAmount: a ladder member with no case would fall to the default and look
// like a Theme that quietly ignores a valid declaration.
func TestPaddingClassCoversEveryLadderAmount(t *testing.T) {
	amounts := []domain.PaddingAmount{domain.PaddingAmountHalf, domain.PaddingAmountOne, domain.PaddingAmountOneHalf, domain.PaddingAmountTwo,
		domain.PaddingAmountTwoHalf, domain.PaddingAmountThree, domain.PaddingAmountFour, domain.PaddingAmountFive}
	prefix := map[domain.PaddingAxis]string{domain.PaddingAxisAll: "p-", domain.PaddingAxisX: "px-", domain.PaddingAxisY: "py-"}
	for role := range domain.KnownPaddingRoles {
		seen := map[string]domain.PaddingAmount{}
		for _, a := range amounts {
			if !domain.PaddingAmountAllowed(role.Axis(), a) {
				continue
			}
			ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.Theme{Padding: map[domain.PaddingRole]domain.PaddingAmount{role: a}}}, "W", false)
			cls := paddingClass(ctx, role)
			if !strings.HasPrefix(cls, prefix[role.Axis()]) {
				t.Errorf("%s=%s rendered %q, want prefix %q", role, a, cls, prefix[role.Axis()])
			}
			if prev, dup := seen[cls]; dup {
				t.Errorf("%s: amounts %q and %q both render %q -- a case is missing", role, prev, a, cls)
			}
			seen[cls] = a
		}
	}
}

// TestStaticMessageAndNote_defaultIsTheLiteralEachSiteReplaced pins the pre-migration `<p>` string beside the
// new output, and a declared theme moving both. The live render-diff reached five of the eleven migrated
// sites (Board Settings' two notes, My Tasks, New Application's lead-in, the board summary); the empty states
// ("No lists yet.", "Nothing has happened to this record yet.") and New Application's publish notes need
// data the dev database does not hold, so this table is the proof for those.
func TestStaticMessageAndNote_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	for _, tc := range []struct {
		kind domain.StaticKind
		want string // copied from the hand-written `<p>` each site carried before 2026-10-06
	}{
		{domain.StaticMessage, `<p class="m-0 text-sm text-slate-500">hi</p>`},
		{domain.StaticNote, `<p class="m-0 text-xs text-slate-500">hi</p>`},
	} {
		var buf bytes.Buffer
		if err := staticText(tc.kind, "hi").Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render(%q) error = %v", tc.kind, err)
		}
		if buf.String() != tc.want {
			t.Errorf("%s rendered %q, want %q", tc.kind, buf.String(), tc.want)
		}
	}

	th := domain.Theme{
		Text: map[domain.TextRole]domain.TextScale{domain.RoleBody: domain.TextMedium, domain.RoleMeta: domain.TextNormal},
		Ink:  map[domain.InkRole]domain.InkShade{domain.InkSecondary: domain.InkShadeDark},
	}
	if got := renderWithTheme(t, th, staticText(domain.StaticMessage, "hi")); !strings.Contains(got, "text-base text-slate-700") {
		t.Errorf("a declared body size and secondary ink should move a message; got %q", got)
	}
	if got := renderWithTheme(t, th, staticText(domain.StaticNote, "hi")); !strings.Contains(got, "text-sm text-slate-700") {
		t.Errorf("a declared meta size and secondary ink should move a note; got %q", got)
	}
}
