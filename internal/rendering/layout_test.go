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
	"menata.app/internal/ir"
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
	if err := collection(domain.GapTight, "", false, items).Render(context.Background(), &buf); err != nil {
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

	// With no `empty` words an empty collection still renders the list, not nothing: the two hand-written
	// callers say "no rows" themselves, and a Component that vanished would change what they draw.
	buf.Reset()
	if err := collection(domain.GapTight, "", false, nil).Render(context.Background(), &buf); err != nil {
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
// with no recompile. Section and panel are equal by default since D6 (2026-10-07; panel was p-4) and a Workspace
// can still declare them apart.
func TestPaddingFollowsTheWorkspaceTheme(t *testing.T) {
	cases := []struct {
		name      string
		c         templ.Component
		was       string
		declared  map[domain.PaddingRole]domain.PaddingAmount
		wantAfter string
	}{
		{"panel", panelLayout(), "p-5", map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingPanel: domain.PaddingAmountFour}, "p-4"},
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

// TestStaticSubheading_defaultIsTheLiteralEachSiteReplaced pins the pre-migration `<h2>` string beside the new
// output and a declared theme moving it. Seven sites moved (2026-10-06); the live diff reaches Board Settings,
// the board's History and Comments, and the checklist/attachments sections that the dev database populates.
func TestStaticSubheading_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	var buf bytes.Buffer
	if err := staticText(domain.StaticSubheading, "Lists").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := `<h2 class="m-0 text-base font-medium">Lists</h2>`; buf.String() != want {
		t.Errorf("subheading rendered %q, want %q", buf.String(), want)
	}

	th := domain.Theme{
		Text:   map[domain.TextRole]domain.TextScale{domain.RoleSubheading: domain.TextLarge},
		Weight: map[domain.WeightRole]domain.WeightStep{domain.WeightEmphasis: domain.WeightStepNormal},
	}
	if got := renderWithTheme(t, th, staticText(domain.StaticSubheading, "Lists")); !strings.Contains(got, "text-xl font-normal") {
		t.Errorf("a declared subheading size and emphasis weight should move it; got %q", got)
	}
}

// TestStaticHeadingEyebrowParagraph_followTheTheme pins the literals 23 h1, 2 eyebrow and the paragraph kind
// wrote before they read the Theme (2026-10-06), and shows a declared role moving each. `heading` and
// `eyebrow` were declarable and read by nothing until this.
func TestStaticHeadingEyebrowParagraph_followTheTheme(t *testing.T) {
	cases := []struct {
		kind  domain.StaticKind
		want  string
		theme domain.Theme
		moved string
	}{
		{domain.StaticHeading, `<h1 class="m-0 text-xl font-medium">T</h1>`,
			domain.Theme{Text: map[domain.TextRole]domain.TextScale{domain.RoleHeading: domain.TextHuge}}, "text-2xl"},
		{domain.StaticEyebrow, `<span class="text-3xs tracking-wide text-blue-600 uppercase">T</span>`,
			domain.Theme{Text: map[domain.TextRole]domain.TextScale{domain.RoleEyebrow: domain.TextSmall}}, "text-xs tracking-wide"},
		{domain.StaticParagraph, `<div class="text-sm text-slate-500">T</div>`,
			domain.Theme{Text: map[domain.TextRole]domain.TextScale{domain.RoleBody: domain.TextSmall}}, "text-xs text-slate-500"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := staticText(c.kind, "T").Render(context.Background(), &buf); err != nil {
			t.Fatalf("%s: Render() error = %v", c.kind, err)
		}
		if buf.String() != c.want {
			t.Errorf("%s rendered %q, want %q", c.kind, buf.String(), c.want)
		}
		if got := renderWithTheme(t, c.theme, staticText(c.kind, "T")); !strings.Contains(got, c.moved) {
			t.Errorf("%s: declared theme should give %q; got %q", c.kind, c.moved, got)
		}
	}
}

// TestStaticOverline_defaultIsTheLiteralEachSiteReplaced pins the nine-site pre-migration `<span>` (Account x3,
// Administration x3, Drafts, Submitted, SLA) and shows the eyebrow role and the faint ink each moving it.
func TestStaticOverline_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	var buf bytes.Buffer
	if err := staticText(domain.StaticOverline, "Account").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := `<span class="text-3xs tracking-wide text-slate-400 uppercase">Account</span>`; buf.String() != want {
		t.Errorf("overline rendered %q, want %q", buf.String(), want)
	}
	th := domain.Theme{
		Text: map[domain.TextRole]domain.TextScale{domain.RoleEyebrow: domain.TextSmall},
		Ink:  map[domain.InkRole]domain.InkShade{domain.InkFaint: domain.InkShadeDarkest},
	}
	got := renderWithTheme(t, th, staticText(domain.StaticOverline, "Account"))
	if !strings.Contains(got, "text-xs") || !strings.Contains(got, "text-slate-900") {
		t.Errorf("a declared eyebrow size and faint ink should move it; got %q", got)
	}
}

// TestStaticPanelHeading_defaultIsTheLiteralEachSiteReplaced pins the 23-site pre-migration `<h2>` and shows the
// body role and the emphasis weight each moving it, independently of `subheading`.
func TestStaticPanelHeading_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	var buf bytes.Buffer
	if err := staticText(domain.StaticPanelHeading, "Password").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := `<h2 class="m-0 text-sm font-medium">Password</h2>`; buf.String() != want {
		t.Errorf("panel-heading rendered %q, want %q", buf.String(), want)
	}
	th := domain.Theme{
		Text:   map[domain.TextRole]domain.TextScale{domain.RoleBody: domain.TextLarge},
		Weight: map[domain.WeightRole]domain.WeightStep{domain.WeightEmphasis: domain.WeightStepNormal},
	}
	got := renderWithTheme(t, th, staticText(domain.StaticPanelHeading, "Password"))
	if !strings.Contains(got, "text-xl font-normal") {
		t.Errorf("a declared body size and emphasis weight should move it; got %q", got)
	}
	if sub := renderWithTheme(t, th, staticText(domain.StaticSubheading, "Lists")); strings.Contains(sub, "text-xl") {
		t.Errorf("subheading must not follow the body role; got %q", sub)
	}
}

func TestStaticCaption_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	var buf bytes.Buffer
	if err := staticText(domain.StaticCaption, "Help").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := `<p class="m-0 text-2xs text-slate-500">Help</p>`; buf.String() != want {
		t.Errorf("caption rendered %q, want %q", buf.String(), want)
	}
	th := domain.Theme{
		Text: map[domain.TextRole]domain.TextScale{domain.RoleCaption: domain.TextNormal},
		Ink:  map[domain.InkRole]domain.InkShade{domain.InkSecondary: domain.InkShadeDarkest},
	}
	got := renderWithTheme(t, th, staticText(domain.StaticCaption, "Help"))
	if !strings.Contains(got, "text-sm") || strings.Contains(got, "text-2xs") {
		t.Errorf("a declared caption size should move it; got %q", got)
	}
	if label := renderWithTheme(t, th, statusBadge("Open", domain.ToneNeutral, domain.BadgeRegular)); strings.Contains(label, "text-sm") {
		t.Errorf("label must not follow the caption role; got %q", label)
	}
}

// TestButtonClasses_defaultIsTheLiteralEachSiteReplaced pins the three strings `controlPrimary`,
// `controlSecondary` and `controlDanger` held, so a default theme is provably the same bytes. They are written
// out here rather than read from the code, because a test that reads the answer from the thing under test
// cannot fail.
func TestButtonClasses_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	ctx := context.Background()
	for variant, want := range map[domain.ButtonVariant]string{
		domain.ButtonPrimary:   "inline-flex h-9 cursor-pointer items-center justify-center rounded-md border-0 bg-slate-900 px-4 font-sans text-sm font-medium text-white hover:bg-slate-800",
		domain.ButtonSecondary: "inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-slate-200 bg-white px-4 font-sans text-sm text-slate-700 hover:bg-slate-50",
		domain.ButtonDanger:    "inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-red-200 bg-white px-4 font-sans text-sm text-red-700 hover:bg-red-50",
	} {
		if got := buttonClasses(ctx, variant); got != want {
			t.Errorf("%s button rendered %q, want %q", variant, got, want)
		}
	}
}

// TestButtonClasses_followTheTheme is the part that makes `RadiusControl` and `InkBody` readable at all: a key
// that validates and changes nothing is the failure `TestEveryThemeRoleHasAReaderOrAReason` exists to catch,
// and this is the test that the reader it found actually reads. It also pins what must NOT move -- the primary
// fill is a dark surface and the danger ink is a tone, neither an ink role.
func TestButtonClasses_followTheTheme(t *testing.T) {
	th := domain.Theme{
		Radius: map[domain.RadiusRole]domain.RadiusStep{domain.RadiusControl: domain.RadiusStepFull},
		Ink:    map[domain.InkRole]domain.InkShade{domain.InkBody: domain.InkShadeMedium},
		Text:   map[domain.TextRole]domain.TextScale{domain.RoleBody: domain.TextSmall},
		Weight: map[domain.WeightRole]domain.WeightStep{domain.WeightEmphasis: domain.WeightStepNormal},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	primary := buttonClasses(ctx, domain.ButtonPrimary)
	secondary := buttonClasses(ctx, domain.ButtonSecondary)
	danger := buttonClasses(ctx, domain.ButtonDanger)
	for name, got := range map[string]string{"primary": primary, "secondary": secondary, "danger": danger} {
		if !strings.Contains(got, "rounded-full") || strings.Contains(got, "rounded-md") {
			t.Errorf("%s: a declared control radius should move the button; got %q", name, got)
		}
		if !strings.Contains(got, "text-xs") || strings.Contains(got, "text-sm") {
			t.Errorf("%s: a declared body size should move the label; got %q", name, got)
		}
	}
	if !strings.Contains(secondary, "text-slate-500") || strings.Contains(secondary, "text-slate-700") {
		t.Errorf("secondary: body ink should move its text; got %q", secondary)
	}
	if !strings.Contains(primary, "font-normal") || strings.Contains(primary, "font-medium") {
		t.Errorf("primary: emphasis weight should move its label; got %q", primary)
	}
	if !strings.Contains(primary, "text-white") || !strings.Contains(danger, "text-red-700") {
		t.Errorf("primary text and danger ink must not follow the body ink role; primary %q danger %q", primary, danger)
	}
}

// TestButton_rendersTheNamePairOnlyWhenGiven pins the attribute order the 17 migrated sites already wrote
// (`type`, `name`, `value`, `class`) and the absence of a stray empty `name=""` on the 13 that carry none.
func TestButton_rendersTheNamePairOnlyWhenGiven(t *testing.T) {
	render := func(c templ.Component) string {
		var buf bytes.Buffer
		if err := c.Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	plain := render(button("Add", domain.ButtonPrimary, "", ""))
	if strings.Contains(plain, "name=") || strings.Contains(plain, "value=") {
		t.Errorf("a button with no pair must not render name/value; got %q", plain)
	}
	if want := `<button type="submit" class="` + buttonClasses(context.Background(), domain.ButtonPrimary) + `">Add</button>`; plain != want {
		t.Errorf("plain button rendered %q, want %q", plain, want)
	}
	pair := render(button("Save as draft", domain.ButtonSecondary, "intent", "draft"))
	if !strings.HasPrefix(pair, `<button type="submit" name="intent" value="draft" class="`) {
		t.Errorf("a pair must render between type and class; got %q", pair)
	}
	if arrow := render(button("Continue →", domain.ButtonPrimary, "", "")); !strings.Contains(arrow, ">Continue →</button>") {
		t.Errorf("the label must reach the page unescaped for a plain arrow; got %q", arrow)
	}
}

// TestButton_reproducesTheUnreachedReviewAndInstallSites pins, byte for byte, three migrated sites that no
// screen in the dev database reaches (no pending step for the viewer, no installable template), so a live
// render-diff could not have caught a wrong argument there. The expected strings are the pre-migration
// literals written out by hand, not derived from buttonClasses.
func TestButton_reproducesTheUnreachedReviewAndInstallSites(t *testing.T) {
	render := func(c templ.Component) string {
		var buf bytes.Buffer
		if err := c.Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	const prefix = "inline-flex h-9 cursor-pointer items-center justify-center"
	for name, c := range map[string]struct{ got, want string }{
		"reject": {
			render(button("Reject", domain.ButtonDanger, "decision", "rejected")),
			`<button type="submit" name="decision" value="rejected" class="` + prefix + ` rounded-md border border-red-200 bg-white px-4 font-sans text-sm text-red-700 hover:bg-red-50">Reject</button>`,
		},
		"approve": {
			render(button("Approve", domain.ButtonPrimary, "decision", "approved")),
			`<button type="submit" name="decision" value="approved" class="` + prefix + ` rounded-md border-0 bg-slate-900 px-4 font-sans text-sm font-medium text-white hover:bg-slate-800">Approve</button>`,
		},
		"install": {
			render(button("Install", domain.ButtonPrimary, "", "")),
			`<button type="submit" class="` + prefix + ` rounded-md border-0 bg-slate-900 px-4 font-sans text-sm font-medium text-white hover:bg-slate-800">Install</button>`,
		},
	} {
		if c.got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", name, c.got, c.want)
		}
	}
}

func TestFieldClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	const want = "h-9 rounded-md border border-slate-300 bg-white px-3 font-sans text-sm text-slate-900 outline-none focus:outline-2 focus:outline-offset-1 focus:outline-blue-300"
	if got := fieldClasses(context.Background()); got != want {
		t.Errorf("default field rendered %q, want the pre-migration controlField literal %q", got, want)
	}
}

// TestFieldClasses_followTheTheme is what gives `BorderControl` its first reader: a key that validates and
// changes nothing is the failure `TestEveryThemeRoleHasAReaderOrAReason` exists to catch. It also pins what
// must not move -- the white fill and the focus ring are not roles.
func TestFieldClasses_followTheTheme(t *testing.T) {
	th := domain.Theme{
		Radius: map[domain.RadiusRole]domain.RadiusStep{domain.RadiusControl: domain.RadiusStepFull},
		Border: map[domain.BorderRole]domain.BorderShade{domain.BorderControl: domain.BorderSoft},
		Ink:    map[domain.InkRole]domain.InkShade{domain.InkStrong: domain.InkShadeMedium},
		Text:   map[domain.TextRole]domain.TextScale{domain.RoleBody: domain.TextSmall},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	got := fieldClasses(ctx)
	for _, want := range []string{"rounded-full", "border-slate-200", "text-slate-500", "text-xs", "bg-white", "focus:outline-blue-300"} {
		if !strings.Contains(got, want) {
			t.Errorf("a declared theme should yield %q; got %q", want, got)
		}
	}
	for _, gone := range []string{"rounded-md", "border-slate-300", "text-slate-900", "text-sm"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q should have moved with the theme; got %q", gone, got)
		}
	}
}

func TestTableCellClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	const was = "border-b border-slate-100 px-3 py-2 align-middle"
	if got := tableCellClasses(context.Background()); got != was {
		t.Errorf("the default theme must render the pre-migration literal\n got  %q\n want %q", got, was)
	}
}

func TestDividerAndBodyWeightFollowTheTheme(t *testing.T) {
	th := domain.Theme{
		Border: map[domain.BorderRole]domain.BorderShade{domain.BorderDivider: domain.BorderDefined},
		Weight: map[domain.WeightRole]domain.WeightStep{domain.WeightBody: domain.WeightStepMedium},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	if got := tableCellClasses(ctx); !strings.Contains(got, "border-slate-300") || strings.Contains(got, "border-slate-100") {
		t.Errorf("theme.border.divider should move a table cell's rule; got %q", got)
	}
	if got := weightClass(ctx, domain.WeightBody); got != "font-medium" {
		t.Errorf("theme.weight.body should move a reset-weight heading; got %q", got)
	}
}

func TestSurfaceClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	if got, want := surfaceClasses(context.Background()), "rounded-lg border border-slate-200 bg-white"; got != want {
		t.Errorf("default surface must render the literal 34 sites carried; got %q, want %q", got, want)
	}
}

func TestSurfaceClasses_followTheThemeAtAHandWrittenSite(t *testing.T) {
	th := domain.Theme{
		Radius:     map[domain.RadiusRole]domain.RadiusStep{domain.RadiusSurface: domain.RadiusStepSmall},
		Border:     map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined},
		Background: map[domain.BackgroundRole]domain.BackgroundShade{domain.BackgroundRaised: domain.BackgroundShadeLighter},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	if got, want := surfaceClasses(ctx), "rounded-md border border-slate-300 bg-slate-50"; got != want {
		t.Errorf("a declared theme must move the surface recipe; got %q, want %q", got, want)
	}

	// roleMatrixApp was one of the 34 literal sites: it must follow the theme, not stay on slate-200.
	var buf strings.Builder
	if err := roleMatrixApp(RoleMatrixApp{Name: "App"}).Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "rounded-md border border-slate-300 bg-slate-50") {
		t.Errorf("a migrated hand-written surface did not follow the theme; got %q", out)
	}
}

func TestTableHeadCellClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	const old = "border-b border-slate-200 px-3 py-2 text-left text-2xs font-medium tracking-wide text-slate-500 uppercase"
	if got := tableHeadCellClasses(context.Background()); got != old {
		t.Errorf("default table head cell must render the constant it replaced; got %q, want %q", got, old)
	}
}

func TestRegionRules_followBorderSurfaceAtAHandWrittenSite(t *testing.T) {
	th := domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	if got := tableHeadCellClasses(ctx); !strings.HasPrefix(got, "border-b border-slate-300 ") {
		t.Errorf("a table head rule must follow border.surface; got %q", got)
	}

	// roleMatrixApp carries three of the thirteen region rules (header, column head, "happens on its own"),
	// beside its surface edge: four slate-300 borders and no slate-200 left to prove a site was missed.
	app := RoleMatrixApp{Name: "App", Roles: []string{"approver"}, RolesSummary: "Approver", Automatic: []string{"x"}}
	var buf strings.Builder
	if err := roleMatrixApp(app).Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	if n := strings.Count(out, "border-slate-300"); n != 4 {
		t.Errorf("want the surface edge and three region rules on slate-300, got %d in %q", n, out)
	}
	if strings.Contains(out, "border-slate-200") {
		t.Errorf("a region rule stayed on slate-200 under a declared border.surface: %q", out)
	}
}

func TestTileClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	if got, want := tileClasses(context.Background()), "rounded-md border border-slate-200"; got != want {
		t.Errorf("default tile must render the literal 11 sites carried; got %q, want %q", got, want)
	}
}

// ChooseWorkspacePage holds two of the eleven tiles and renders before a Workspace exists, so it also pins that
// the readers fall back to the default theme there.
func TestTiles_followTheThemeAtHandWrittenSites(t *testing.T) {
	choices := []WorkspaceChoice{
		{ID: "ws_live", Name: "Live Co", Role: "member"},
		{ID: "ws_old", Name: "Old Co", Role: "admin", Archived: true, ArchivedAt: "12 Jul 2026"},
	}
	render := func(ctx context.Context) string {
		var buf strings.Builder
		if err := ChooseWorkspacePage(choices, "", "/switch-workspace", "/home", "Back", false, "a@b.c").Render(ctx, &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}

	// The tails anchor each tile: the page's own surface wrapper carries the same radius/border/fill triple.
	tiles := func(radius, edge, fill string) []string {
		return []string{
			"gap-3 " + radius + " border " + edge + " " + fill + " p-4 text-left",
			"gap-3 " + radius + " border " + edge + " " + fill + " p-3.5",
		}
	}
	out := render(context.Background())
	for _, want := range tiles("rounded-md", "border-slate-200", "bg-white") {
		if !strings.Contains(out, want) {
			t.Errorf("default theme must keep the tile on the literal it replaced: missing %q", want)
		}
	}

	th := domain.Theme{
		Radius:     map[domain.RadiusRole]domain.RadiusStep{domain.RadiusControl: domain.RadiusStepLarge},
		Border:     map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined},
		Background: map[domain.BackgroundRole]domain.BackgroundShade{domain.BackgroundRaised: domain.BackgroundShadeLighter},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	out = render(ctx)
	for _, want := range tiles("rounded-lg", "border-slate-300", "bg-slate-50") {
		if !strings.Contains(out, want) {
			t.Errorf("a declared theme must move the tile: missing %q", want)
		}
	}
	if strings.Contains(out, "border-slate-200") {
		t.Errorf("a tile stayed on the literal edge colour under a declared theme")
	}
}

func TestAuthenticatedFields_followTheThemeAtHandWrittenSites(t *testing.T) {
	render := func(ctx context.Context, c templ.Component) string {
		var buf strings.Builder
		if err := c.Render(ctx, &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}
	app := RoleApplication{ID: "app_x", Name: "X", Field: "role_x", Roles: []string{"approver"}}
	sites := map[string]func(context.Context) string{
		"account name field": func(ctx context.Context) string {
			return render(ctx, ProfilePage("Ana", "a@b.c", "WS", Viewer{}, "/switch", ""))
		},
		"invite role select": func(ctx context.Context) string { return render(ctx, roleSelect("r1", app, "")) },
		"signature width box": func(ctx context.Context) string {
			return render(ctx, widthControl(PlacementStep{StepID: "s1", Width: 20}, 1))
		},
	}
	th := domain.Theme{
		Radius: map[domain.RadiusRole]domain.RadiusStep{domain.RadiusControl: domain.RadiusStepLarge},
		Border: map[domain.BorderRole]domain.BorderShade{domain.BorderControl: domain.BorderSoft},
	}
	themed := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)

	// Anchors are each field's own tail, so a neighbouring surface carrying a similar edge cannot satisfy them.
	for name, draw := range sites {
		def := draw(context.Background())
		if !strings.Contains(def, "border-slate-300") {
			t.Errorf("%s: the default theme must keep the edge on the literal it replaced (border-slate-300)", name)
		}
		got := draw(themed)
		if strings.Contains(got, "border-slate-300") && name != "account name field" {
			t.Errorf("%s: stayed on border-slate-300 under a declared border.control", name)
		}
	}
	acct := sites["account name field"](themed)
	if !strings.Contains(acct, "h-10.5 rounded-lg border border-slate-200 bg-white px-3") {
		t.Errorf("account name field did not move with radius.control + border.control:\n%s", acct)
	}
	sel := sites["invite role select"](themed)
	if !strings.Contains(sel, "h-10.5 rounded-lg border border-slate-200 bg-white px-3") {
		t.Errorf("roleSelect did not move with radius.control + border.control")
	}
}

// TestSecondaryButtonOutlineReadsTheSurfaceBorder pins owner decision D8 (2026-10-07): a secondary button's outline
// is the surface edge, so declaring `border.surface` moves it and declaring `border.control` does not (that key is
// an input's own rim). The default stays the slate-200 the literal wrote.
func TestSecondaryButtonOutlineReadsTheSurfaceBorder(t *testing.T) {
	if got := buttonClasses(context.Background(), domain.ButtonSecondary); !strings.Contains(got, "border-slate-200") {
		t.Fatalf("default secondary outline must stay slate-200; got %q", got)
	}
	at := func(role domain.BorderRole) string {
		th := domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{role: domain.BorderFaint}}
		ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
		return buttonClasses(ctx, domain.ButtonSecondary)
	}
	if got := at(domain.BorderSurface); !strings.Contains(got, "border-slate-100") || strings.Contains(got, "border-slate-200") {
		t.Errorf("a declared border.surface should move the secondary outline; got %q", got)
	}
	if got := at(domain.BorderControl); !strings.Contains(got, "border-slate-200") {
		t.Errorf("border.control must not move a button's outline; got %q", got)
	}
}

// TestSectionMigration_defaultsAreTheLiteralsEachSiteReplaced pins the pre-migration literal beside the
// primitive's default for the sites a live diff cannot reach (2026-10-07). `inferenceSummary` was reached and
// byte-identical; `newapplication`'s change summary needs an AI session, and `ChildSectionView` needs a child
// collection, neither of which the dev database holds.
func TestSectionMigration_defaultsAreTheLiteralsEachSiteReplaced(t *testing.T) {
	// was `<section class={ surfaceClasses(ctx), "p-5" }>` (inference.templ, newapplication.templ)
	if got, want := renderWithTheme(t, domain.Theme{}, panelLayout()),
		`<section class="rounded-lg border border-slate-200 bg-white p-5">x</section>`; got != want {
		t.Errorf("panelLayout default = %q, want %q", got, want)
	}
	// was `<section class="flex flex-col gap-2">` (detail.templ, machine.templ x2); the element is now a <div>
	if got, want := renderWithTheme(t, domain.Theme{}, stackLayout(domain.GapTight)),
		`<div class="flex flex-col gap-2">x</div>`; got != want {
		t.Errorf("stackLayout(GapTight) default = %q, want %q", got, want)
	}
}

// TestDividedRowClasses_defaultsAreTheLiteralsThreeSitesReplaced pins the pre-migration literal beside the
// reader's output (2026-10-07): approvalinbox (gap-2) and workspacemembers x2 (gap-3) each wrote
// `flex flex-col gap-N border-b border-slate-100 px-4 py-3 last:border-b-0 sm:flex-row sm:items-center sm:gap-4`.
// A declared `border.divider` and `gap` must move it with no recompile.
func TestDividedRowClasses_defaultsAreTheLiteralsThreeSitesReplaced(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test"}, "Test Workspace", false)
	for gap, want := range map[domain.Gap]string{
		domain.GapTight:   "flex flex-col gap-2 border-b border-slate-100 px-4 py-3 last:border-b-0 sm:flex-row sm:items-center sm:gap-4",
		domain.GapDefault: "flex flex-col gap-3 border-b border-slate-100 px-4 py-3 last:border-b-0 sm:flex-row sm:items-center sm:gap-4",
	} {
		if got := dividedRowClasses(ctx, gap); got != want {
			t.Errorf("dividedRowClasses(%v) = %q, want %q", gap, got, want)
		}
	}
	th := domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{domain.BorderDivider: domain.BorderDefined}}
	ctx = WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	if got := dividedRowClasses(ctx, domain.GapTight); !strings.Contains(got, "border-slate-300") || strings.Contains(got, "border-slate-100") {
		t.Errorf("a declared border.divider should move the row rule; got %q", got)
	}
}

// TestCollection_drawsItsEmptyWordsInPlaceOfAnEmptyList: the words are the author's (content), the Component
// only decides *when* they show -- no items -- and what shape they take: the shared `message` role, so a
// Theme that restyles messages restyles this too. With items the words must not appear at all.
func TestCollection_drawsItsEmptyWordsInPlaceOfAnEmptyList(t *testing.T) {
	render := func(empty string, items []templ.Component) string {
		var buf bytes.Buffer
		if err := collection(domain.GapTight, empty, false, items).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	if got := render("Nothing here yet.", nil); !strings.Contains(got, "Nothing here yet.") || strings.Contains(got, "<ul") {
		t.Errorf("no items: want the words and no list; got %q", got)
	}
	if got := render("Nothing here yet.", []templ.Component{staticText(domain.StaticParagraph, "one")}); strings.Contains(got, "Nothing here yet.") || !strings.Contains(got, "<li") {
		t.Errorf("with an item: want the list and not the words; got %q", got)
	}
}

// TestTagChip_reproducesThePreMigrationMarkup pins, byte for byte, what `tagChip(CardTag)` drew before it took
// two bounded parameters. The expected string is the old literal written out by hand, not derived from
// tagClasses, so a change to the markup fails here rather than passing against itself.
func TestTagChip_reproducesThePreMigrationMarkup(t *testing.T) {
	render := func(c templ.Component) string {
		var buf bytes.Buffer
		if err := c.Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	got := render(tagChip("Bug", domain.TagRose))
	want := `<span class="inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2 py-0.5 text-2xs border-rose-700/20 bg-rose-700/10 text-rose-700">` +
		`<span class="size-1.5 rounded-full bg-rose-700" aria-hidden="true"></span> Bug</span>`
	if strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(want), " ") {
		t.Errorf("tagChip rendered\n%s\nwant\n%s", got, want)
	}
	if unknown := render(tagChip("Legacy", "chartreuse")); !strings.Contains(unknown, "bg-slate-600/10 text-slate-600") {
		t.Errorf("a colour outside the palette must fall back to the neutral entry, got %s", unknown)
	}
}

// TestUINode_drawsATagFromItsDeclaredProps: the walker arm hands a declared Tag's two props to the same
// renderer the five existing sites use, so a page and a board card draw one chip.
func TestUINode_drawsATagFromItsDeclaredProps(t *testing.T) {
	var buf bytes.Buffer
	n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentTag), Props: map[string]string{"label": "Urgent", "color": "amber"}}
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	for _, want := range []string{"Urgent", "bg-amber-700/10 text-amber-700", "size-1.5 rounded-full bg-amber-700"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("declared Tag missing %q:\n%s", want, buf.String())
		}
	}
}

// TestCollection_orderedDrawsAnOlWithOneOrdinalPerItem pins the shape Board Settings' list drew by hand: an
// `<ol>` (no bullets, no margin), and each item preceded by its position, in the `meta` size and `faint` ink.
// The expected strings are written out, not derived from the readers, so a change to the markup fails here.
// The ordinal is aria-hidden because the `<ol>` already announces position; an unordered collection draws no
// ordinal at all.
func TestCollection_orderedDrawsAnOlWithOneOrdinalPerItem(t *testing.T) {
	var buf bytes.Buffer
	items := []templ.Component{staticText(domain.StaticParagraph, "one"), staticText(domain.StaticParagraph, "two")}
	if err := collection(domain.GapTight, "", true, items).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `<ol class="m-0 flex list-none flex-col p-0 gap-2">`) || strings.Contains(got, "<ul") {
		t.Errorf("ordered collection should be an <ol>; got %q", got)
	}
	for _, n := range []string{"1", "2"} {
		want := `<span class="w-5 text-xs text-slate-400" aria-hidden="true">` + n + `</span>`
		if !strings.Contains(got, want) {
			t.Errorf("missing ordinal %q; got %q", want, got)
		}
	}
	if strings.Contains(got, ">3<") {
		t.Errorf("two items must not produce a third ordinal; got %q", got)
	}

	buf.Reset()
	if err := collection(domain.GapTight, "", false, items).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "aria-hidden") {
		t.Errorf("an unordered collection draws no ordinal; got %q", buf.String())
	}
}
