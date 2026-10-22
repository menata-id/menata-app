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
		{"calendar.CalendarPage (migrated to nav_calendar, 2026-10-11)", domain.GapDefault, domain.GridCols1, domain.GridCols7, "grid grid-cols-1 gap-3 sm:grid-cols-7"},
		{"approvalinbox.pendingApprovalCardGrid", domain.GapComfortable, domain.GridCols1, domain.GridCols2, "grid grid-cols-1 gap-4 sm:grid-cols-2"},
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
		got := resolveColourRoles(t, context.Background(), buf.String())
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
		got := resolveColourRoles(t, context.Background(), buf.String())
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
		return resolveColourRoles(t, ctx, buf.String())
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
	if got := resolveColourRoles(t, context.Background(), buf.String()); got != "" {
		t.Errorf("an absent SLA badge rendered %q -- a Field with no value must produce no markup, not an empty styled element", got)
	}

	buf.Reset()
	if err := slaBadge(experience.SLABadge{Label: "OVERDUE", Tone: domain.ToneBad, Present: true}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	// The deliberate change: overdue is a red *pill* now, not red text.
	if got := resolveColourRoles(t, context.Background(), buf.String()); !strings.Contains(got, "rounded-full") || !strings.Contains(got, "bg-red-50 text-red-700") {
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
		got := resolveColourRoles(t, context.Background(), buf.String())
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
	if err := collection(domain.GapTight, "", false, false, false, Truncation{}, "", "", items).Render(context.Background(), &buf); err != nil {
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
	if err := collection(domain.GapTight, "", false, false, false, Truncation{}, "", "", nil).Render(context.Background(), &buf); err != nil {
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
	got := resolveColourRoles(t, context.Background(), buf.String())
	if want := `<a href="/register" class="text-blue-600 hover:text-blue-700">Create a workspace</a>`; got != want {
		t.Errorf("staticLink emitted %q, want %q -- this is the pre-migration literal from git history, not from the renderer", got, want)
	}
	if strings.Contains(got, "@staticLink") {
		t.Error("staticLink rendered its own call as text")
	}
}

// TestMetric_inkFollowsTheWorkspaceTheme proves text colour is declarable, with no recompile, and that the
// default is exactly what the Metric hardcoded before (`slate-900` value, `slate-500` label), except the hint: it was
// `slate-400` and reads `secondary` since rencana 1.2b, because a page may bind it to a computed figure (`hint_figure:`),
// which is content, not a hint.
func TestMetric_inkFollowsTheWorkspaceTheme(t *testing.T) {
	render := func(ctx context.Context) string {
		var buf bytes.Buffer
		if err := metric("Open", "7", "since Monday", "", domain.ToneNeutral).Render(ctx, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	def := resolveColourRoles(t, context.Background(), render(context.Background()))
	for _, want := range []string{"text-slate-900", "text-xs text-slate-500", "text-2xs text-slate-500"} {
		if !strings.Contains(def, want) {
			t.Errorf("default Metric lost %q; got %q", want, def)
		}
	}

	ws := domain.Workspace{Slug: "test", Theme: domain.Theme{Ink: map[domain.InkRole]domain.InkShade{
		domain.InkSecondary: domain.InkShadeDark,
	}}}
	wctx := WithCurrentWorkspace(context.Background(), ws, "Test Workspace", false)
	got := resolveColourRoles(t, wctx, render(wctx))
	if !strings.Contains(got, "text-xs text-slate-700") || !strings.Contains(got, "text-2xs text-slate-700") {
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
		if err := panelLayout(domain.PanelRegular).Render(templ.WithChildren(ctx, templ.Raw("x")), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return resolveColourRoles(t, templ.WithChildren(ctx, templ.Raw("x")), buf.String())
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
	return resolveColourRoles(t, ctx, buf.String())
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
		{"panel", panelLayout(domain.PanelRegular), "p-5", map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingPanel: domain.PaddingAmountFour}, "p-4"},
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
		if got := resolveColourRoles(t, context.Background(), buf.String()); got != tc.want {
			t.Errorf("%s rendered %q, want %q", tc.kind, got, tc.want)
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
		if got := resolveColourRoles(t, context.Background(), buf.String()); got != c.want {
			t.Errorf("%s rendered %q, want %q", c.kind, got, c.want)
		}
		if got := renderWithTheme(t, c.theme, staticText(c.kind, "T")); !strings.Contains(got, c.moved) {
			t.Errorf("%s: declared theme should give %q; got %q", c.kind, c.moved, got)
		}
	}
}

// TestStaticOverline_defaultIsTheLiteralEachSiteReplaced pins the nine-site pre-migration `<span>` (Account x3,
// Administration x3, Drafts, Submitted, SLA) and shows the eyebrow role and the secondary ink each moving it. The ink
// changed once on purpose (rencana 1.2b, 2026-10-13): a section label names its section, so it is content, and the
// Design System keeps `faint` for hints and decoration. slate-400 became slate-500.
func TestStaticOverline_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	var buf bytes.Buffer
	if err := staticText(domain.StaticOverline, "Account").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got, want := resolveColourRoles(t, context.Background(), buf.String()), `<span class="text-3xs tracking-wide text-slate-500 uppercase">Account</span>`; got != want {
		t.Errorf("overline rendered %q, want %q", got, want)
	}
	th := domain.Theme{
		Text: map[domain.TextRole]domain.TextScale{domain.RoleEyebrow: domain.TextSmall},
		Ink:  map[domain.InkRole]domain.InkShade{domain.InkSecondary: domain.InkShadeDarkest},
	}
	got := renderWithTheme(t, th, staticText(domain.StaticOverline, "Account"))
	if !strings.Contains(got, "text-xs") || !strings.Contains(got, "text-slate-900") {
		t.Errorf("a declared eyebrow size and secondary ink should move it; got %q", got)
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
	if got, want := resolveColourRoles(t, context.Background(), buf.String()), `<p class="m-0 text-2xs text-slate-500">Help</p>`; got != want {
		t.Errorf("caption rendered %q, want %q", got, want)
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
//
// The primary hover changed once on purpose (owner decision CP1, 2026-10-13): slate-800 became `strong` at 90%, so the
// button's hover follows the fill it belongs to.
func TestButtonClasses_defaultIsTheLiteralEachSiteReplaced(t *testing.T) {
	ctx := context.Background()
	for variant, want := range map[domain.ButtonVariant]string{
		domain.ButtonPrimary:   "inline-flex h-9 cursor-pointer items-center justify-center rounded-md border-0 bg-slate-900 px-4 font-sans text-sm font-medium text-white hover:bg-slate-900/90",
		domain.ButtonSecondary: "inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-slate-200 bg-white px-4 font-sans text-sm text-slate-700 hover:bg-slate-50",
		domain.ButtonDanger:    "inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-red-200 bg-white px-4 font-sans text-sm text-red-700 hover:bg-red-50",
	} {
		if got := resolveColourRoles(t, ctx, buttonClasses(ctx, variant)); got != want {
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
	primary := resolveColourRoles(t, ctx, buttonClasses(ctx, domain.ButtonPrimary))
	secondary := resolveColourRoles(t, ctx, buttonClasses(ctx, domain.ButtonSecondary))
	danger := resolveColourRoles(t, ctx, buttonClasses(ctx, domain.ButtonDanger))
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
		return resolveColourRoles(t, context.Background(), buf.String())
	}
	const prefix = "inline-flex h-9 cursor-pointer items-center justify-center"
	for name, c := range map[string]struct{ got, want string }{
		"reject": {
			render(button("Reject", domain.ButtonDanger, "decision", "rejected")),
			`<button type="submit" name="decision" value="rejected" class="` + prefix + ` rounded-md border border-red-200 bg-white px-4 font-sans text-sm text-red-700 hover:bg-red-50">Reject</button>`,
		},
		"approve": {
			render(button("Approve", domain.ButtonPrimary, "decision", "approved")),
			`<button type="submit" name="decision" value="approved" class="` + prefix + ` rounded-md border-0 bg-slate-900 px-4 font-sans text-sm font-medium text-white hover:bg-slate-900/90">Approve</button>`,
		},
		"install": {
			render(button("Install", domain.ButtonPrimary, "", "")),
			`<button type="submit" class="` + prefix + ` rounded-md border-0 bg-slate-900 px-4 font-sans text-sm font-medium text-white hover:bg-slate-900/90">Install</button>`,
		},
	} {
		if c.got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", name, c.got, c.want)
		}
	}
}

// The literal changed once on purpose (2026-10-12, DS-D14): `outline-none` became `focus:outline-solid`. Under
// Tailwind v4, `outline-none` sets `--tw-outline-style: none`, which `focus:outline-2` then reuses, so no field drew
// its focus ring at all (measured in Chromium: outline-style none on a focused input).
// A field states its own weight (gap plan K-17, 2026-10-15): form controls inherit `font-weight` from the label that wraps them, and the
// Move and Copy forms on the record page had been drawing their inputs at 500 because their labels were medium. `font-normal` is the one
// token beyond the literal this test is named for.
func TestFieldClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	const want = "h-9 rounded-md border border-slate-300 bg-white px-3 font-sans text-sm font-normal text-slate-900 focus:outline-solid focus:outline-2 focus:outline-offset-1 focus:outline-blue-300"
	if got := resolveColourRoles(t, context.Background(), fieldClasses(context.Background())); got != want {
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
	got := resolveColourRoles(t, ctx, fieldClasses(ctx))
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
	if got := resolveColourRoles(t, context.Background(), tableCellClasses(context.Background())); got != was {
		t.Errorf("the default theme must render the pre-migration literal\n got  %q\n want %q", got, was)
	}
}

func TestDividerAndBodyWeightFollowTheTheme(t *testing.T) {
	th := domain.Theme{
		Border: map[domain.BorderRole]domain.BorderShade{domain.BorderDivider: domain.BorderDefined},
		Weight: map[domain.WeightRole]domain.WeightStep{domain.WeightBody: domain.WeightStepMedium},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	if got := resolveColourRoles(t, ctx, tableCellClasses(ctx)); !strings.Contains(got, "border-slate-300") || strings.Contains(got, "border-slate-100") {
		t.Errorf("theme.border.divider should move a table cell's rule; got %q", got)
	}
	if got := weightClass(ctx, domain.WeightBody); got != "font-medium" {
		t.Errorf("theme.weight.body should move a reset-weight heading; got %q", got)
	}
}

func TestSurfaceClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	if got, want := resolveColourRoles(t, context.Background(), surfaceClasses(context.Background())), "rounded-lg border border-slate-200 bg-white"; got != want {
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
	if got, want := resolveColourRoles(t, ctx, surfaceClasses(ctx)), "rounded-md border border-slate-300 bg-slate-50"; got != want {
		t.Errorf("a declared theme must move the surface recipe; got %q, want %q", got, want)
	}

	// roleMatrixApp was one of the 34 literal sites: it must follow the theme, not stay on slate-200.
	var buf strings.Builder
	if err := roleMatrixApp(RoleMatrixApp{Name: "App"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if out := resolveColourRoles(t, ctx, buf.String()); !strings.Contains(out, "rounded-md border border-slate-300 bg-slate-50") {
		t.Errorf("a migrated hand-written surface did not follow the theme; got %q", out)
	}
}

func TestTableHeadCellClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	const old = "border-b border-slate-200 px-3 py-2 text-left text-2xs font-medium tracking-wide text-slate-500 uppercase"
	if got := resolveColourRoles(t, context.Background(), tableHeadCellClasses(context.Background())); got != old {
		t.Errorf("default table head cell must render the constant it replaced; got %q, want %q", got, old)
	}
}

func TestRegionRules_followBorderSurfaceAtAHandWrittenSite(t *testing.T) {
	th := domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	if got := resolveColourRoles(t, ctx, tableHeadCellClasses(ctx)); !strings.HasPrefix(got, "border-b border-slate-300 ") {
		t.Errorf("a table head rule must follow border.surface; got %q", got)
	}

	// roleMatrixApp carries three of the thirteen region rules (header, column head, "happens on its own"),
	// beside its surface edge: four slate-300 borders and no slate-200 left to prove a site was missed.
	app := RoleMatrixApp{Name: "App", Roles: []string{"approver"}, RolesSummary: "Approver", Automatic: []string{"x"}}
	var buf strings.Builder
	if err := roleMatrixApp(app, "").Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := resolveColourRoles(t, ctx, buf.String())
	if n := strings.Count(out, "border-slate-300"); n != 4 {
		t.Errorf("want the surface edge and three region rules on slate-300, got %d in %q", n, out)
	}
	if strings.Contains(out, "border-slate-200") {
		t.Errorf("a region rule stayed on slate-200 under a declared border.surface: %q", out)
	}
}

func TestTileClasses_defaultIsTheLiteralItReplaced(t *testing.T) {
	if got, want := resolveColourRoles(t, context.Background(), tileClasses(context.Background())), "rounded-md border border-slate-200"; got != want {
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
		return resolveColourRoles(t, ctx, buf.String())
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
		return resolveColourRoles(t, ctx, buf.String())
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
	if got := resolveColourRoles(t, context.Background(), buttonClasses(context.Background(), domain.ButtonSecondary)); !strings.Contains(got, "border-slate-200") {
		t.Fatalf("default secondary outline must stay slate-200; got %q", got)
	}
	at := func(role domain.BorderRole) string {
		th := domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{role: domain.BorderFaint}}
		ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
		return resolveColourRoles(t, ctx, buttonClasses(ctx, domain.ButtonSecondary))
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
	if got, want := renderWithTheme(t, domain.Theme{}, panelLayout(domain.PanelRegular)),
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
		if got := resolveColourRoles(t, ctx, dividedRowClasses(ctx, gap)); got != want {
			t.Errorf("dividedRowClasses(%v) = %q, want %q", gap, got, want)
		}
	}
	th := domain.Theme{Border: map[domain.BorderRole]domain.BorderShade{domain.BorderDivider: domain.BorderDefined}}
	ctx = WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "test", Theme: th}, "Test Workspace", false)
	if got := resolveColourRoles(t, ctx, dividedRowClasses(ctx, domain.GapTight)); !strings.Contains(got, "border-slate-300") || strings.Contains(got, "border-slate-100") {
		t.Errorf("a declared border.divider should move the row rule; got %q", got)
	}
}

// TestCollection_drawsItsEmptyWordsInPlaceOfAnEmptyList: the words are the author's (content), the Component
// only decides *when* they show -- no items -- and what shape they take: the shared `message` role, so a
// Theme that restyles messages restyles this too. With items the words must not appear at all.
func TestCollection_drawsItsEmptyWordsInPlaceOfAnEmptyList(t *testing.T) {
	render := func(empty string, items []templ.Component) string {
		var buf bytes.Buffer
		if err := collection(domain.GapTight, empty, false, false, false, Truncation{}, "", "", items).Render(context.Background(), &buf); err != nil {
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
		// A Tag's hue is a property since dark mode (DK8): compared as what it draws by default.
		return resolveColourRoles(t, context.Background(), buf.String())
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
	got := resolveColourRoles(t, context.Background(), buf.String())
	for _, want := range []string{"Urgent", "bg-amber-800/10 text-amber-800", "size-1.5 rounded-full bg-amber-800"} {
		if !strings.Contains(got, want) {
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
	if err := collection(domain.GapTight, "", true, false, false, Truncation{}, "", "", items).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := resolveColourRoles(t, context.Background(), buf.String())
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
	if err := collection(domain.GapTight, "", false, false, false, Truncation{}, "", "", items).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "aria-hidden") {
		t.Errorf("an unordered collection draws no ordinal; got %q", buf.String())
	}
}

// TestCollection_dividedDrawsOneSurfaceWithARuleBetweenRows pins the mockup's list shape (PM06): one bordered,
// raised surface, each row padded and ruled in the divider role, the last row ruleless. Expected strings are
// written out, not derived from the readers. Ordered keeps the `<ol>` and the counted ordinal; undivided draws
// neither the surface nor the rule.
func TestCollection_dividedDrawsOneSurfaceWithARuleBetweenRows(t *testing.T) {
	items := []templ.Component{staticText(domain.StaticParagraph, "one"), staticText(domain.StaticParagraph, "two")}
	render := func(ordered, divided bool) string {
		var buf bytes.Buffer
		if err := collection(domain.GapTight, "", ordered, divided, false, Truncation{}, "", "", items).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return resolveColourRoles(t, context.Background(), buf.String())
	}
	got := render(false, true)
	for _, want := range []string{
		`<ul class="m-0 list-none overflow-hidden p-0 rounded-lg border border-slate-200 bg-white">`,
		// `relative` makes the item the ancestor a `variant: row` link stretches over; it draws nothing.
		`<li class="relative flex flex-wrap items-center gap-2 border-b border-slate-100 px-4 py-3 text-sm last:border-b-0">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("divided collection missing %q; got %q", want, got)
		}
	}
	if n := strings.Count(got, "<li "); n != 2 {
		t.Errorf("want one <li> per item, got %d", n)
	}
	ord := render(true, true)
	if !strings.Contains(ord, `<ol class="m-0 list-none overflow-hidden p-0 rounded-lg`) || !strings.Contains(ord, `aria-hidden="true">2</span>`) {
		t.Errorf("a divided, ordered collection keeps its <ol> and counted ordinals; got %q", ord)
	}
	for _, plain := range []string{render(false, false), render(true, false)} {
		if strings.Contains(plain, "rounded-lg") || strings.Contains(plain, "border-b") {
			t.Errorf("an undivided collection draws no surface and no rule; got %q", plain)
		}
	}
}

// TestUINode_aCollectionNamesItsBoundOnlyWhenTheLoweredTreeCarriesOne: the walker hands the `truncated` prop to
// the same notice the hand-written lists use, after the list; a Collection without one draws no notice.
func TestUINode_aCollectionNamesItsBoundOnlyWhenTheLoweredTreeCarriesOne(t *testing.T) {
	item := ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": "one"}}
	render := func(props map[string]string) string {
		var buf bytes.Buffer
		n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentCollection), Props: props, Children: []ir.UINode{item}}
		if err := uiNode(n).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	cut := render(map[string]string{"gap": "tight", "truncated": "200"})
	if !strings.Contains(cut, "Showing the first 200 only") {
		t.Errorf("a cut list did not name its bound:\n%s", cut)
	}
	if strings.Index(cut, "</ul>") > strings.Index(cut, "Showing the first") {
		t.Errorf("the notice should follow the list:\n%s", cut)
	}
	for _, props := range []map[string]string{{"gap": "tight"}, {"gap": "tight", "truncated": "nonsense"}} {
		if got := render(props); strings.Contains(got, "Showing the first") {
			t.Errorf("props %v drew a notice:\n%s", props, got)
		}
	}
}

// A Metric with no href is byte-for-byte what it was before the input existed; one with an href is the same
// tile inside one anchor, and the anchor carries the href through templ's URL sanitiser.
func TestMetric_hrefWrapsTheSameTileInOneAnchor(t *testing.T) {
	render := func(href string) string {
		var buf bytes.Buffer
		if err := metric("Open", "7", "since Monday", href, domain.ToneNeutral).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	plain := render("")
	if strings.Contains(plain, "<a") {
		t.Errorf("a Metric with no destination drew an anchor: %q", plain)
	}
	linked := render("/pages/nav_x?status=in+review")
	if !strings.HasPrefix(linked, `<a href="/pages/nav_x?status=in+review"`) || strings.Count(linked, "<a ") != 1 || !strings.Contains(linked, plain) {
		t.Errorf("a linking Metric is not the plain tile inside one anchor; got %q", linked)
	}
}

// TestRequestButton_patchSendsTheNamePairAsOneJSONObjectAndAsksNothing: a transition Button is a PATCH whose body is
// the one Field it sets. The pair travels in `hx-vals` as JSON built by encoding/json (a value with a quote must
// not break out of the attribute), the swap is `none` like the other request Buttons, and no `hx-confirm` is drawn.
func TestRequestButton_patchSendsTheNamePairAsOneJSONObjectAndAsksNothing(t *testing.T) {
	var buf bytes.Buffer
	if err := requestButton("Mark done", domain.ButtonSecondary, "fld_status", `do"ne`, "/machines/m/records/r", domain.ButtonMethodPatch, "", "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	for _, want := range []string{`type="button"`, `hx-patch="/machines/m/records/r"`, `hx-swap="none"`, `hx-vals="{&#34;fld_status&#34;:&#34;do\&#34;ne&#34;}"`, ">Mark done</button>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, banned := range []string{"hx-confirm", "hx-delete", "hx-post", "hx-target"} {
		if strings.Contains(got, banned) {
			t.Errorf("a patch Button drew %q: %s", banned, got)
		}
	}
	if requestVals("a", "b") != `{"a":"b"}` {
		t.Errorf("requestVals = %q", requestVals("a", "b"))
	}
}

// TestCollection_headingDrawsATitleAndACountOfWhatIsUnderIt: a headed Collection is a section -- the title as a
// heading element, the number of items as a badge, the hint after both -- and an unheaded one draws none of it.
func TestCollection_headingDrawsATitleAndACountOfWhatIsUnderIt(t *testing.T) {
	items := []templ.Component{staticText(domain.StaticParagraph, "one"), staticText(domain.StaticParagraph, "two"), staticText(domain.StaticParagraph, "three")}
	var buf bytes.Buffer
	if err := collection(domain.GapTight, "", false, false, false, Truncation{}, "Overdue", "needs a tick", items).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	h, c, hint, first := strings.Index(got, ">Overdue<"), strings.Index(got, ">3<"), strings.Index(got, "needs a tick"), strings.Index(got, ">one<")
	if !strings.Contains(got, "<h2") || h < 0 || c < h || hint < c || first < hint {
		t.Errorf("want heading, count, hint, then the first item in that order; got %q", got)
	}
	buf.Reset()
	if err := collection(domain.GapTight, "", false, false, false, Truncation{}, "", "", items).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "<h2") || strings.Contains(buf.String(), ">3<") {
		t.Errorf("an unheaded Collection drew a heading or a count: %q", buf.String())
	}
}

// TestCollection_aHeadedSectionStillSaysItsEmptyWordsAndItsTruncation: the headed branch draws the body and the
// notice itself, so a section with a title must keep both -- the empty words over no items (the badge says 0),
// and the truncation notice after a cut list.
func TestCollection_aHeadedSectionStillSaysItsEmptyWordsAndItsTruncation(t *testing.T) {
	render := func(empty string, truncated Truncation, items []templ.Component) string {
		var buf bytes.Buffer
		if err := collection(domain.GapTight, empty, false, false, false, truncated, "Later", "", items).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	none := render("Nothing here yet.", Truncation{}, nil)
	if !strings.Contains(none, ">Later<") || !strings.Contains(none, ">0<") || !strings.Contains(none, "Nothing here yet.") {
		t.Errorf("a headed empty section lost its heading, its 0 or its words: %q", none)
	}
	cut := render("", Truncation{Limit: 200, Hit: true}, []templ.Component{staticText(domain.StaticParagraph, "one")})
	if !strings.Contains(cut, "Showing the first 200 only") {
		t.Errorf("a headed section lost its truncation notice: %q", cut)
	}
}

// The current section keeps the surface of every other section and swaps its fill for the info tone's, with no
// text colour; the ordinary one is byte-for-byte what sectionLayout drew before the current variant existed.
func TestUINode_aCurrentSectionIsTintedAndAnnounced(t *testing.T) {
	draw := func(props map[string]string) string {
		var buf bytes.Buffer
		n := ir.UINode{Kind: ir.NodeLayout, Type: string(domain.LayoutSection), Props: props,
			Children: []ir.UINode{{Kind: ir.NodeStatic, Type: string(domain.StaticNote), Props: map[string]string{"text": "x"}}}}
		if err := uiNode(n).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return resolveColourRoles(t, context.Background(), buf.String())
	}
	plain := draw(map[string]string{"gap": "tight"})
	if strings.Contains(plain, "aria-current") || !strings.Contains(plain, "bg-white") || strings.Contains(plain, "bg-blue-50") {
		t.Errorf("an ordinary section changed:\n%s", plain)
	}
	cur := draw(map[string]string{"gap": "tight", "current": "true"})
	for _, want := range []string{`aria-current="date"`, "bg-blue-50", "border-slate-200", "rounded-lg"} {
		if !strings.Contains(cur, want) {
			t.Errorf("current section missing %q:\n%s", want, cur)
		}
	}
	for _, not := range []string{"bg-white", "text-blue-700"} {
		if strings.Contains(cur, not) {
			t.Errorf("current section carries %q:\n%s", not, cur)
		}
	}
}

// Rencana 0.2b: corners that are a control, a surface or a pill read radiusClass, so theme.radius moves them. The
// migration was byte-identical under the default theme; this is the other half (R9), one site of each role, under a
// theme that moves all three -- including two roles onto one step, the shape that broke the approver chip (L12).
func TestRadii_followTheThemeAtMigratedSites(t *testing.T) {
	th := domain.Theme{Radius: map[domain.RadiusRole]domain.RadiusStep{
		domain.RadiusControl: domain.RadiusStepFull,
		domain.RadiusSurface: domain.RadiusStepFull,
		domain.RadiusPill:    domain.RadiusStepSmall,
	}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	render := func(c templ.Component) string {
		var buf strings.Builder
		if err := c.Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	for name, tc := range map[string]struct {
		c          templ.Component
		want, gone string
	}{
		"control: the overflow menu's row": {applicationMenuRow([]navMenuItem{{Label: "A", Href: "/a"}}, domain.Application{}), "rounded-full px-2.5", "rounded-md px-2.5"},
		"pill: the approver chip":          {approverChip(StepApprover{Name: "Ana", State: "done"}), "rounded-md py-0.5", "rounded-full py-0.5"},
	} {
		got := render(tc.c)
		if !strings.Contains(got, tc.want) || strings.Contains(got, tc.gone) {
			t.Errorf("%s: a moved theme.radius must move the corner (want %q, not %q):\n%s", name, tc.want, tc.gone, got)
		}
	}
}

// Rencana 0.2a: the 137 hand-written ladder gaps read layoutGap, so theme.gap moves them (R9). Two sites, under a
// theme that moves `default` and also puts `comfortable` on the same amount (L12's shape). The notification row that was the
// first of the two is a declared page now (metadata/runtimepages), so the shell is the one site left here.
func TestLadderGaps_followTheThemeAtMigratedSites(t *testing.T) {
	th := domain.Theme{Gap: map[domain.Gap]domain.GapAmount{
		domain.GapDefault:     domain.GapAmountSix,
		domain.GapComfortable: domain.GapAmountSix,
	}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	var buf strings.Builder
	if err := appShell("T", "WS", Viewer{}, "/s").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<main class="mx-auto w-full max-w-[1180px] px-4 sm:px-6 flex flex-col gap-6`) {
		t.Error("a moved theme.gap must move the shell's main column gap")
	}
}

// Rencana 0.4: hand-written panel/section paddings read paddingClass, and loose surface edges read borderClass, so
// theme.padding and theme.border.surface move them (R9). Panel and section are both `five` by default and are moved
// apart here, so a site reading the wrong one of the two shows.
func TestPanelPaddingAndSurfaceEdges_followTheThemeAtMigratedSites(t *testing.T) {
	th := domain.Theme{
		Padding: map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingPanel: domain.PaddingAmountThree, domain.PaddingSection: domain.PaddingAmountFour},
		Border:  map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	render := func(c templ.Component) string {
		var buf strings.Builder
		if err := c.Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return resolveColourRoles(t, ctx, buf.String())
	}
	modal := render(signatureModal("s1"))
	if !strings.Contains(modal, "border border-slate-300 p-3 bg-white text-slate-900 shadow-lg") {
		t.Errorf("the signature dialog must take panel padding and the surface edge from the theme: %s", modal)
	}
}

// Rencana 0.4, after the independent review: every surface edge that moved to borderClass(BorderSurface) is pinned
// under the **default** theme, at its own element. `surface`, `control` and `divider` have different defaults
// (slate-200/300/100), so reading the wrong role at any one site changes these bytes. The moved-theme test above
// covers only two sites, and the review showed 6 of 8 edge swaps surviving it.
func TestSurfaceEdges_eachMigratedSiteReadsTheSurfaceRole(t *testing.T) {
	m, v := boardMachine()
	records, cards, columns := boardFixture()
	sla := experience.SLABadge{Label: "Due today", Tone: domain.ToneWarn, Urgent: true, Present: true}
	for name, tc := range map[string]struct {
		c    templ.Component
		want string
	}{
		"approval card (rest)": {pendingApprovalCard(PendingApprovalCard{ID: "d", Title: "T", SLA: sla}), `class="flex flex-col gap-3 rounded-lg border bg-white p-5 text-slate-900 hover:border-blue-600/40 border-slate-200"`},
		"signature dialog":     {signatureModal("s1"), `class="sig-modal m-auto w-[calc(100%-2rem)] max-w-md rounded-lg border border-slate-200 p-5 bg-white text-slate-900 shadow-lg backdrop:bg-[color-mix(in_oklab,_var(--color-slate-900)_40%,_transparent)]"`},
		"signature frame":      {reviewSignaturePanel(ReviewView{SignaturePage: 1, SignaturePreviewHref: "/p", SignatureStepCount: 1}), `class="relative overflow-hidden rounded-md border border-slate-200 bg-slate-50"`},
		"board lane":           {boardLayout(m, v, records, columns, cards, domain.Actor{}, nil), `flex w-40.5 shrink-0 flex-col gap-2 rounded-lg border p-2 border-slate-200 bg-slate-100`},
		"pdf thumbnail":        {pdfThumbnail("mch_document", "rec_1"), `class="block max-w-40 rounded border border-slate-200 bg-white"`},
		"placement page frame": {placementCanvas(PlacementView{DocumentID: "d", Page: 1, TotalPages: 1}), `class="block w-full rounded-md border border-slate-200 bg-white"`},
	} {
		var buf strings.Builder
		if err := tc.c.Render(context.Background(), &buf); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(resolveColourRoles(t, context.Background(), buf.String()), tc.want) {
			t.Errorf("%s: the surface edge is not border.surface's default here; want %s", name, tc.want)
		}
	}
}

// DS-D10 (R9): a Workspace that declares `theme.ink.accent: violet` moves links, accent text, the eyebrow and the
// active bottom tab together, hover included, and nothing grey moves with them.
func TestAccentInk_followsTheThemeAtMigratedSites(t *testing.T) {
	th := domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkAccent: domain.InkHueViolet}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	var buf strings.Builder
	if err := staticLink("/x", "Open").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if got := resolveColourRoles(t, ctx, buf.String()); got != `<a href="/x" class="text-violet-600 hover:text-violet-700">Open</a>` {
		t.Errorf("a static link must take the accent and its hover from the theme: %s", got)
	}
	buf.Reset()
	if err := staticText(domain.StaticEyebrow, "Account").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if got := resolveColourRoles(t, ctx, buf.String()); !strings.Contains(got, "text-violet-600") || strings.Contains(got, "blue") {
		t.Errorf("the eyebrow is accent text and must follow theme.ink.accent: %s", got)
	}
	if (domain.Theme{Ink: map[domain.InkRole]domain.InkShade{domain.InkStrong: domain.InkHueViolet}}).InkFor(domain.InkStrong) != domain.InkShadeDarkest {
		t.Error("a grey role must not take a hue: InkFor must fall back to the default")
	}
}

// DS-D16 (R9): a tone's edge follows its palette with its fill and ink. A Workspace mapping `warn` to red gets a red
// rim on the truncation notice, not an amber one; mapping `bad` to the grey palette keeps the overdue card's edge
// distinct from the surface edge only in fill and ink, and the edge chosen in Go cannot be dropped.
func TestToneEdge_followsTheToneAtMigratedSites(t *testing.T) {
	th := domain.Theme{Tone: map[domain.BadgeTone]domain.TonePalette{domain.ToneWarn: domain.PaletteRed, domain.ToneBad: domain.PaletteGrey}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	var buf strings.Builder
	if err := truncationNotice(Truncation{Limit: 5, Hit: true}).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resolveColourRoles(t, ctx, buf.String()), "border border-red-200 bg-red-50") || strings.Contains(resolveColourRoles(t, ctx, buf.String()), "amber") {
		t.Errorf("a warn remapped to red must take the red edge too: %s", resolveColourRoles(t, ctx, buf.String()))
	}
	if got := resolveColourRoles(t, ctx, approvalCardEdge(ctx, true)); got != "border-slate-200" {
		t.Errorf("an overdue card under a grey bad tone must still carry an edge class, got %q", got)
	}
}

// W1 (R9): wells follow their colour's background role. Moving `base` and `sunken` apart shows a well that read the
// wrong one: the progress track is sunken, the role matrix's header band is base.
func TestWells_followTheirBackgroundRole(t *testing.T) {
	th := domain.Theme{Background: map[domain.BackgroundRole]domain.BackgroundShade{
		domain.BackgroundBase:   domain.BackgroundShadeLight,
		domain.BackgroundSunken: domain.BackgroundShadeLightest,
	}}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	var buf strings.Builder
	if err := progressRow("Ana", "1", "2", "").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resolveColourRoles(t, ctx, buf.String()), "h-2 grow overflow-hidden rounded-full bg-white") {
		t.Errorf("a progress track is a sunken well and must follow theme.background.sunken: %s", resolveColourRoles(t, ctx, buf.String()))
	}
}

// DS-D16: every palette's edge, pinned. GS1 checks only the two edges the Design System names (warn, bad); this holds
// the rest, so a changed arm (the 2026-10-12 review moved green to -300 and nothing failed) is seen.
func TestToneEdgeClass_eachPaletteHasItsEdge(t *testing.T) {
	want := map[domain.TonePalette]string{
		domain.PaletteGrey: "border-slate-200", domain.PaletteGreyFaint: "border-slate-100", domain.PaletteBlue: "border-blue-200",
		domain.PaletteGreen: "border-emerald-200", domain.PaletteRed: "border-red-200", domain.PaletteAmber: "border-amber-200",
	}
	for pal, w := range want {
		ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.Theme{Tone: map[domain.BadgeTone]domain.TonePalette{domain.ToneInfo: pal}}}, "WS", false)
		if got := resolveColourRoles(t, ctx, toneEdgeClass(ctx, domain.ToneInfo)); got != w {
			t.Errorf("palette %s: edge %q, want %q", pal, got, w)
		}
	}
	if len(want) != len(domain.KnownTonePalettes) {
		t.Errorf("pinned %d palettes, the domain has %d -- pin the new one", len(want), len(domain.KnownTonePalettes))
	}
}

// DS-D11 (R9): hover lives in recipes. A secondary button and a menu row hover with background.base (the Design
// System's hover-wash alias); a danger control hovers with the bad tone's fill, edge and ink moving with it.
func TestHoverRecipes_followTheTheme(t *testing.T) {
	th := domain.Theme{
		Background: map[domain.BackgroundRole]domain.BackgroundShade{domain.BackgroundBase: domain.BackgroundShadeLight},
		Tone:       map[domain.BadgeTone]domain.TonePalette{domain.ToneBad: domain.PaletteAmber},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	if got := resolveColourRoles(t, ctx, buttonClasses(ctx, domain.ButtonSecondary)); !strings.HasSuffix(got, " hover:bg-slate-100") {
		t.Errorf("a secondary button must hover with background.base, got %q", got)
	}
	got := resolveColourRoles(t, ctx, buttonClasses(ctx, domain.ButtonDanger))
	for _, want := range []string{"border-amber-200", "text-amber-800", "hover:bg-amber-50"} {
		if !strings.Contains(got, want) {
			t.Errorf("a danger button under bad=amber must carry %s: %q", want, got)
		}
	}
}

// DS-D13 (R9): an item's title follows theme.text.item. Under item: medium every migrated site draws text-base, and
// no default 15px survives on the pages that carry one.
func TestRoleItem_followsTheTheme(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: domain.Theme{Text: map[domain.TextRole]domain.TextScale{domain.RoleItem: domain.TextMedium}}}, "WS", false)
	if got := textClass(ctx, domain.RoleItem); got != "text-base" {
		t.Errorf("item under medium: %q, want text-base", got)
	}
	if got := textClass(context.Background(), domain.RoleItem); got != "text-item" {
		t.Errorf("item by default: %q, want text-item", got)
	}
}

// Rencana 2.3d (R9): both modal dialogs draw their shell from dialogClasses, so one moved theme moves both. Radius,
// panel padding and the surface edge are moved off their defaults, and each dialog must carry exactly the shell the
// reader answers under that theme (not the default's).
func TestDialogShell_bothDialogsFollowTheTheme(t *testing.T) {
	th := domain.Theme{
		Radius:  map[domain.RadiusRole]domain.RadiusStep{domain.RadiusSurface: domain.RadiusStepFull},
		Padding: map[domain.PaddingRole]domain.PaddingAmount{domain.PaddingPanel: domain.PaddingAmountThree},
		Border:  map[domain.BorderRole]domain.BorderShade{domain.BorderSurface: domain.BorderDefined},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	// The shell pinned whole under the default theme (the review of 2.3d: moving only radius, padding and border and then
	// comparing each dialog with the reader itself let an ink, background, elevation or scrim swap through).
	const defaultShell = "m-auto w-[calc(100%-2rem)] max-w-md rounded-lg border border-slate-200 p-5 bg-white text-slate-900 shadow-lg backdrop:bg-[color-mix(in_oklab,_var(--color-slate-900)_40%,_transparent)]"
	if got := resolveColourRoles(t, context.Background(), dialogClasses(context.Background())); got != defaultShell {
		t.Errorf("the dialog shell under the default theme changed:\n got %s\nwant %s", got, defaultShell)
	}
	th.Ink = map[domain.InkRole]domain.InkShade{domain.InkStrong: domain.InkShadeMedium}
	ctx = WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	if got := resolveColourRoles(t, ctx, dialogClasses(ctx)); strings.Contains(got, "text-slate-900") {
		t.Errorf("a moved theme.ink.strong must move the dialog's ink: %s", got)
	}
	shell := strings.Fields(dialogClasses(ctx))
	if !strings.Contains(dialogClasses(ctx), "rounded-full") || !strings.Contains(dialogClasses(ctx), "p-3") {
		t.Fatalf("the moved theme never reached dialogClasses: %s", dialogClasses(ctx))
	}
	for name, c := range map[string]templ.Component{
		"leave guard":     leaveWorkspaceGuard("WS", "admin", "/switch-workspace"),
		"signature modal": signatureModal("s1"),
	} {
		var buf strings.Builder
		if err := c.Render(ctx, &buf); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		m := regexp.MustCompile(`<dialog[^>]*\sclass="([^"]*)"`).FindStringSubmatch(buf.String())
		if m == nil {
			t.Fatalf("%s: no <dialog class> found", name)
		}
		have := map[string]bool{}
		for _, tok := range strings.Fields(m[1]) {
			have[tok] = true
		}
		for _, tok := range shell {
			if !have[tok] {
				t.Errorf("%s: dialog lacks shell class %q under the moved theme: %s", name, tok, m[1])
			}
		}
	}
}

// An icon control's touch target is a pseudo-element under a coarse pointer only, on a `relative` element (gap plan K-05).
func TestTouchTargetClass_growsOnlyUnderACoarsePointer(t *testing.T) {
	got := touchTargetClass()
	for _, want := range []string{"relative", "pointer-coarse:after:absolute", "pointer-coarse:after:-inset-1.5"} {
		if !strings.Contains(got, want) {
			t.Errorf("touchTargetClass() = %q, missing %q", got, want)
		}
	}
	for _, f := range strings.Fields(got) {
		if f != "relative" && !strings.HasPrefix(f, "pointer-coarse:") {
			t.Errorf("%q applies to a fine pointer too; only `relative` may", f)
		}
	}
}

// A notice about a grid takes the whole row; a day added only to fill a week is not drawn on a phone; a day a card may be dropped on
// carries its whole name for the script that announces a move.
func TestUINodeSpanOutsideDayAndDropLabel(t *testing.T) {
	render := func(n ir.UINode) string {
		var buf bytes.Buffer
		if err := uiNode(n).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	note := render(ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentCallout), Props: map[string]string{"text": "cut", "tone": "warn", ir.SpanProp: "all"}})
	if !strings.Contains(note, `class="col-span-full"`) {
		t.Errorf("a spanning node is not wrapped to take the row:\n%s", note)
	}
	plain := render(ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentCallout), Props: map[string]string{"text": "cut", "tone": "warn"}})
	if strings.Contains(plain, "col-span-full") {
		t.Errorf("a node that did not ask to span did:\n%s", plain)
	}
	day := render(ir.UINode{Kind: ir.NodeLayout, Type: string(domain.LayoutSection), Props: map[string]string{"gap": "tight", ir.SectionDropFieldProp: "fld_due", ir.SectionDropValueProp: "2026-10-18", ir.SectionLabelProp: "Sun 18 Oct"}})
	if !strings.Contains(day, `data-drop-label="Sun 18 Oct"`) || strings.Contains(day, "hidden sm:flex") {
		t.Errorf("a droppable day:\n%s", day)
	}
	quiet := render(ir.UINode{Kind: ir.NodeLayout, Type: string(domain.LayoutSection), Props: map[string]string{"gap": "tight", ir.SectionOutsideProp: "true"}})
	if !strings.Contains(quiet, "hidden sm:flex") {
		t.Errorf("an outside day is drawn on a phone:\n%s", quiet)
	}
}

// A paragraph may carry a tone: its words in that tone's ink, with no badge around them -- a date that is overdue in red, a finished one in green, a
// plain one in the quiet ink. A tone that is no tone is the neutral one, as a badge's is.
func TestUINode_aParagraphCanBeInATone(t *testing.T) {
	render := func(props map[string]string) string {
		var buf bytes.Buffer
		n := ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticParagraph), Props: props}
		if err := uiNode(n).Render(context.Background(), &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		return buf.String()
	}
	for tone, want := range map[string]string{"bad": "text-tone-bad-ink", "good": "text-tone-good-ink", "nonsense": "text-tone-neutral-ink"} {
		if got := render(map[string]string{"text": "03 Oct", "tone": tone}); !strings.Contains(got, want) || !strings.Contains(got, "03 Oct") || strings.Contains(got, "rounded-full") {
			t.Errorf("tone %q: %s, want the words in %s and no badge", tone, got, want)
		}
	}
	if got := render(map[string]string{"text": "plain"}); strings.Contains(got, "text-tone-") {
		t.Errorf("a paragraph with no tone took one: %s", got)
	}
}
