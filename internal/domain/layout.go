package domain

// LayoutKind is the closed set of generic spatial composition primitives
// (007-composable-runtime-architecture.md §12.2).
//
// **Seven of §12.2's eight are built, and each arrived with the uses it replaces.** §12.2 lists `stack`,
// `row`, `columns`, `grid`, `split`, `tabs`, `panel` and `section`. Counted by *meaning* across the 38
// bespoke screens (see the retraction below for why the first two counts were wrong): vertical flow with a
// uniform gap appears **61 times** (`stack`), the bordered padded surface twelve times across six screens
// (`panel`), a responsive card grid **five** times (`grid`) and a main-plus-aside split **five** more
// (`split`), horizontal wrapping flow **24** times (`row`, of 33 sites) and content that stacks on a phone
// then sits side by side **9** times (`columns`, of which 5 migrated).
//
// **`tabs` and `section` are the two still unbuilt, and neither is unbuilt for lack of a case.** An earlier
// version of this comment said `columns`, `tabs` and `section` had "**zero** measured uses and are
// deliberately absent". All three were wrong, and the owner disproved the first with one observation --
// decompose the Document Approval app and its sub-components become columns on desktop, with sections.
// Measured: `columns` 9, `section` **34** (the largest remaining population in the corpus), `tabs` at least
// the Approval Inbox's three, which have been running in production the whole time.
// The method was the fault, not the corpus: all three were searched for as *class strings* -- `columns-2`,
// CSS multi-column -- instead of being read as shapes on a screen. That is the same failure that wrongly
// rejected `row` and `grid` the day before, and it happened three times in one session
// (`menata-app-document`'s `guides/subordinate-mechanisms-must-not-outrank-the-vision.md`).
// `conformance.handWrittenLayoutSites` now counts both, which took its reported debt from 13 sites to 51 --
// a directive gate that under-reports by four times directs nothing -- §12.2 is a permitted vocabulary, not a quota, and building the rest now would be
// the shape-before-need 007 §34 forbids.
//
// **Why a closed set rather than a class string**: §15.2 forbids an intermediate representation embedding
// "HTML, CSS framework classes", and §12.6 says static content "does not imply arbitrary HTML or a code
// hatch". The mapping from a kind to Tailwind classes lives in internal/rendering, which is the only plane
// entitled to know them. A `bool`-valued closed set is vocabulary, so it belongs here and not in
// internal/registry (conformance.TestDomainHoldsVocabularyAndRegistryHoldsDispatch).
//
// **What this does not yet do, stated so a reader is not misled by its existence**: no metadata file
// declares a layout. The primitives' first consumer is `rendering.pageHeader`, which composes the block
// eleven screens repeat. A *screen* becomes declarable at Stage 2-3 of the plan in
// `menata-app-document`'s `audits/2026-09-30-kajian-ui-ir-untuk-apa-dan-bagaimana-merealisasikannya.md`,
// because a body needs a bounded Component (§12.3) and a Page tree — Layout and Static Content alone
// cannot express "a panel per rule, each with a heading and three labelled values".
type LayoutKind string

const (
	// LayoutStack is vertical flow with a uniform gap. The most-used shape by an order of magnitude.
	LayoutStack LayoutKind = "stack"
	// LayoutPanel is a bordered, padded surface -- named for what it is rather than for the `<section>` it
	// happens to render, so a later change of element is not a change of vocabulary (§12.3's naming rule).
	LayoutPanel LayoutKind = "panel"
	// LayoutGrid is a card grid whose column count responds to viewport width. Its parameters are GridCols
	// and Gap, both closed sets -- which is what makes it bounded under §12.3, a rule about *arbitrary*
	// properties and not about having parameters at all. A grid taking a free CSS track list would be the
	// thing §12.3 forbids; one taking a column count from a closed set is vocabulary.
	LayoutGrid LayoutKind = "grid"
	// LayoutSplit is a main area beside a fixed-width aside, stacking to one column on a narrow viewport.
	// §12.2 lists it, and an earlier measurement of mine claimed it had zero uses in this corpus; it has
	// five, and they differ only in which side the aside is on and how wide it is.
	LayoutSplit LayoutKind = "split"
	// LayoutRow is horizontal flow that wraps. Its parameters are the two flexbox axes as closed sets --
	// `RowAlign` and `RowJustify` -- which is what Bootstrap, Tailwind and every component library expose as
	// named options rather than as free CSS.
	LayoutRow LayoutKind = "row"
	// LayoutColumns is content that stacks on a narrow viewport and sits side by side from `sm:` up.
	//
	// **It is not `grid` and not `split`**, and the distinction is what made it invisible to two earlier
	// measurements of mine. `grid` flows *items* into equal cells; `split` is one main area beside a
	// fixed-width aside; `columns` is *different content* in each column, equal width, collapsing to a stack.
	// A Document Approval screen decomposed into sub-components is exactly this on desktop -- which is how the
	// owner found it (2026-10-03) after I twice recorded `columns` as having "zero measured uses". Both times I
	// searched for a class string (`columns-2`, CSS multi-column) instead of for the meaning.
	LayoutColumns LayoutKind = "columns"
	// LayoutSection is a bordered, padded surface that stacks its children -- §12.2's `section`, a titled
	// grouping of content.
	//
	// **It is deliberately separate from `panel` even though both are bordered padded surfaces, and the
	// difference is 4px of padding.** I argued for merging them: §24's Specialization Rule says not to add a
	// type when existing primitives express the requirement, and `section` is arguably `panel` + an internal
	// stack + a heading child. Measured, the real distinction in this corpus is *internal stacking* -- `panel`
	// has none and none of its 12 callers stacks immediately, while all 17 `section` sites stack with a gap --
	// and the padding correlates with it, which usually means the padding is the accident.
	//
	// **The owner chose to keep them separate (2026-10-03), and the defence is risk rather than taste**: it is
	// the only option where *no screen moves*, and three of the six slices that day already moved pixels.
	// §12.2 lists both names, so two primitives is within the vocabulary. Recorded here with the argument
	// against it so the next reader does not "fix" the padding and silently restyle 12 or 21 screens.
	LayoutSection LayoutKind = "section"
)

// ColumnsAlign is how a columns layout aligns its children on the cross axis **once it is a row**. Below the
// breakpoint it is a stack and alignment does not apply.
//
// Two members, which is what the five migrated sites use: `end` for a heading beside its action (they sit on
// one baseline at the bottom), `center` for row content. The main axis is not a parameter -- all five push
// their children apart, so a `justify` enum would be a value with one case.
type ColumnsAlign string

const (
	ColumnsAlignEnd    ColumnsAlign = "end"
	ColumnsAlignCenter ColumnsAlign = "center"
)

// KnownColumnsAligns is the closed set.
var KnownColumnsAligns = map[ColumnsAlign]bool{ColumnsAlignEnd: true, ColumnsAlignCenter: true}

// RowAlign is a row's cross-axis alignment, as a closed set.
//
// Two members, because the 24 migrated sites use two. `RowAlignStretch` is flexbox's own default and emits no
// class at all, which is why it is named `stretch` rather than `start`: `align-items: normal` behaves as
// stretch, and a vocabulary word that lies about what it renders is worse than no word.
//
// **`baseline` is deliberately absent.** Three sites use it and all three are excluded from the migration for
// other reasons (see rendering.rowLayout), so declaring it would be a member no screen calls -- which
// `conformance.TestLayoutVocabularyIsRenderedAndUsed` fails, and rightly: the first pass of this work shipped
// three such primitives.
type RowAlign string

const (
	RowAlignStretch RowAlign = "stretch"
	RowAlignCenter  RowAlign = "center"
)

// KnownRowAligns is the closed set.
var KnownRowAligns = map[RowAlign]bool{
	RowAlignStretch: true,
	RowAlignCenter:  true,
}

// RowJustify is a row's main-axis distribution, as a closed set. Two members: flow from the start, or push the
// children apart. Six of the 24 migrated sites do the second.
type RowJustify string

const (
	RowJustifyStart  RowJustify = "start"
	RowJustifySpread RowJustify = "spread"
)

// KnownRowJustifies is the closed set.
var KnownRowJustifies = map[RowJustify]bool{
	RowJustifyStart:  true,
	RowJustifySpread: true,
}

// SplitSide says which side of a split the fixed-width aside sits on.
//
// `start`/`end` rather than `left`/`right`, matching CSS logical properties: a vocabulary that names a
// physical direction is wrong the first time something renders right-to-left, and this set is meant to
// outlive that.
type SplitSide string

const (
	SplitAsideStart SplitSide = "start"
	SplitAsideEnd   SplitSide = "end"
)

// KnownSplitSides is the closed set.
var KnownSplitSides = map[SplitSide]bool{
	SplitAsideStart: true,
	SplitAsideEnd:   true,
}

// AsideWidth is how wide a split's fixed track is, as a **named step rather than a pixel count**.
//
// Two steps, where the corpus had **four distinct widths across five sites** -- 316, 320, 320, 340 and 360
// pixels. Those are not four design decisions. They are roughly two, written five times, which is what a
// repeated literal turns into. Naming the steps is therefore a correction and not only a
// generalisation -- and it **changes what renders** on two screens, which is recorded where the migration is
// rather than discovered later: the signature-placement aside gains 4px, and the second New Application
// aside gains 20px.
//
// A third step is not declared. If a screen genuinely needs one, it arrives with that screen, the same rule
// `Gap` and `GridCols` follow.
type AsideWidth string

const (
	AsideNarrow AsideWidth = "narrow"
	AsideWide   AsideWidth = "wide"
)

// KnownAsideWidths is the closed set.
var KnownAsideWidths = map[AsideWidth]bool{
	AsideNarrow: true,
	AsideWide:   true,
}

// GridCols is a grid's column count: a closed set, not a free integer.
//
// Closed because an open one is how a logical primitive becomes a CSS passthrough. The four members are
// exactly the counts the corpus uses, and they are **not interchangeable across breakpoints** -- the
// measured mobile counts are one and two, the measured desktop counts are two, four and seven, and
// `internal/rendering` emits a class only for a count measured at that breakpoint. Passing a desktop-only
// count as the mobile one therefore renders the single-column default rather than a new class, which is the
// deliberate trade: the alternative is shipping utility classes no screen uses, which is the defect the
// first pass of this work actually caused.
//
// Growing this set is gated from both ends: `conformance.TestLayoutVocabularyIsRenderedAndUsed` fails a
// member no renderer draws *and* a member no screen calls.
type GridCols int

const (
	GridCols1 GridCols = 1
	GridCols2 GridCols = 2
	GridCols4 GridCols = 4
	GridCols7 GridCols = 7
)

// KnownGridCols is the closed set.
var KnownGridCols = map[GridCols]bool{
	GridCols1: true,
	GridCols2: true,
	GridCols4: true,
	GridCols7: true,
}

// **`row`, `grid` and `split` are primitives, and the comment that stood here said otherwise. Retracted
// 2026-10-02 after the owner asked the obvious question: every real framework has row and grid, so how
// could this corpus not need them.**
//
// The rejected reasoning was: 21 `row` sites span "six shapes" and 6 `grid` sites span "six arbitrary
// track lists", so neither is one shape. **That counted class strings and called the result shapes.** A
// class-string difference is a *configuration* difference, and absorbing configuration is exactly what a
// parameterised primitive is for -- `grid-cols-2` and `grid-cols-4` are not two primitives, they are
// `columns: 2` and `columns: 4`.
//
// What §12.3 actually forbids is a component becoming "'generic' merely by accepting **arbitrary**
// properties". A grid taking a free CSS track string would be that -- CSS smuggled through the logical
// plane, §15.2's own prohibition with different syntax. A grid taking `columns` from a closed set is
// bounded. The constraint belongs on the *parameters*, not on the primitive's existence.
//
// Re-measured by **meaning** rather than by class string, and the counts changed materially:
//
//   - `grid`, responsive column count: **6** sites -- `sm:grid-cols-2` x3, `sm:grid-cols-4`,
//     `sm:grid-cols-7`, `grid-cols-2 sm:grid-cols-4`. One primitive, one enum parameter.
//   - `split`, a main area beside a fixed-width aside: **5** sites --
//     `lg:grid-cols-[320px_minmax(0,1fr)]`, `[1fr_360px]`, `[1fr_340px]`, `[minmax(0,1fr)_320px]`,
//     `[minmax(0,1fr)_316px]`. **This is §12.2's own `split`, which the earlier comment dismissed as
//     having "zero measured uses".** It has five, and the five differ only in a pixel width and which
//     side the aside is on.
//   - `row`: alignment is a **closed set of three** -- `items-center` (11), `justify-between` (5),
//     `items-baseline` (2) -- which is precisely what Bootstrap, Tailwind and every component library
//     expose as named options. The gap ladder needs more steps than the two currently declared
//     (`gap-2` 11, `gap-1.5` 5, `gap-3` 4, `gap-1` 2, `gap-3.5` 1, plus one `gap-x`/`gap-y` pair).
//   - genuine one-offs: **2** -- a `[6rem_1fr]` label/value definition list, and rolematrix's bordered
//     row grid. Those stay hand-written, which is what a one-off is.
//
// So the next slice builds `row`, `grid` and `split` **with their callers**, parameters as closed enums,
// and the two one-offs left alone. The error chain worth remembering: the first measurement counted
// frequency and not uniformity; the second counted uniformity and not parameterisability. Both were
// caught by being asked whether the conclusion was plausible, not by a gate.

// KnownLayoutKinds is the closed set. A kind absent from it is a load-time error, never a silent skip
// (capability-lifecycle.md §4 rule 3, "Unknown = explicit").
var KnownLayoutKinds = map[LayoutKind]bool{
	LayoutStack:   true,
	LayoutPanel:   true,
	LayoutGrid:    true,
	LayoutSplit:   true,
	LayoutRow:     true,
	LayoutColumns: true,
	LayoutSection: true,
}

// Gap is the spacing between a layout's children, as an enum rather than a number.
//
// Deliberately not a pixel or rem value: a declaration that carries `gap: 12px` has smuggled a physical
// presentation choice into the logical plane, which is what §15.2 forbids and what would make this a page
// builder rather than a composition vocabulary.
//
// A step is declared when a migration needs it, never in advance -- an unused step is one more utility class
// in the shipped bundle for nothing, and `conformance.TestLayoutVocabularyIsRenderedAndUsed` fails one. The
// third arrived with the `grid` migration on 2026-10-03, because two of its five sites space their cards one
// step wider than the other three and collapsing that would have silently restyled them.
//
// The fourth arrived with `split` the same day, for the two New Application screens, which space their two
// columns one step wider than the three approval screens do.
//
// **No half-step was declared, and that was the `row` migration's one judgement call.** The corpus used two
// half-steps between tight and default. Five sites at one and a half fold into `GapTight`, which moves each of
// the five by **two pixels**; declaring the half-step instead would have preserved an accident as vocabulary.
// The one site at two and a half is not in the migrated set at all -- it is a `<form>`, and this primitive
// renders a `<div>` -- so it keeps its own class and no decision was needed about it. Worth saying because the
// first count folded it in and was wrong by one: the gap ladder and the migrated population are two different
// measurements.
type Gap string

const (
	GapTight       Gap = "tight"
	GapDefault     Gap = "default"
	GapComfortable Gap = "comfortable"
	GapLoose       Gap = "loose"
)

// GapAmount is how much space a Gap step actually renders — the fourth Theme category, and **the one with a
// different shape from the other three.**
//
// Radius, weight and text all needed a *role* vocabulary invented for them, because their call sites carried a
// raw class. `Gap` is already the role: a caller says `GapTight`, never `gap-2`. So its Theme entry maps the
// existing step straight to an amount, and no second vocabulary is needed. Worth stating because the first
// three slices established a pattern this one correctly breaks.
//
// Six amounts, all already in the shipped CSS bundle from hand-written use, so remapping emits nothing new.
//
// **What this slice deliberately does not do**: add the two steps the token inventory found missing. `gap-1`
// (49 uses) and `gap-0.5` (24) sit below `GapTight`, but both live in hand-written markup rather than in a
// primitive call — and `conformance.TestLayoutVocabularyIsRenderedAndUsed` requires every declared step to
// have a caller. Declaring them now would be a vocabulary with nothing behind it, which that gate exists to
// refuse. They arrive when a layout migration needs them, and a Workspace can already reach those amounts by
// remapping an existing step.
type GapAmount string

const (
	GapAmountHalf  GapAmount = "half"
	GapAmountOne   GapAmount = "one"
	GapAmountTwo   GapAmount = "two"
	GapAmountThree GapAmount = "three"
	GapAmountFour  GapAmount = "four"
	GapAmountFive  GapAmount = "five"
)

// KnownGapAmounts is the closed set.
var KnownGapAmounts = map[GapAmount]bool{
	GapAmountHalf: true, GapAmountOne: true, GapAmountTwo: true,
	GapAmountThree: true, GapAmountFour: true, GapAmountFive: true,
}

// KnownGaps is the closed set. An empty Gap means GapDefault, resolved where the layout is rendered.
var KnownGaps = map[Gap]bool{
	GapTight:       true,
	GapDefault:     true,
	GapComfortable: true,
	GapLoose:       true,
}

// StaticKind is the closed set of static content nodes (§12.6): explanatory text and visual material
// combined with structured data, as first-class composable nodes.
//
// Three of §12.6's six. `image`, `link`, `callout` and `divider` have zero measured uses in a page's own
// composed content and are absent for the same reason the four unbuilt layouts are.
//
// `eyebrow` is not in §12.6's list and is this repository's own addition, which is worth flagging rather
// than hiding: it is the small uppercase line naming the Application above a screen's heading, and it
// recurs in eleven screens. It is a *heading-level* static node by any reading of §12.6's "heading", and
// naming it separately is what lets `pageHeader` render it without a second class string.
type StaticKind string

const (
	StaticEyebrow   StaticKind = "eyebrow"
	StaticHeading   StaticKind = "heading"
	StaticParagraph StaticKind = "paragraph"
	// StaticLink is §12.6's own `link`: a text link to an already-resolved destination.
	//
	// **It carries no size, and that is what the measurement decided.** 29 `<a>` sites use the blue link
	// colour, and three estimates of mine put the uniform population at 34, then 23, then 12. Measuring each
	// site's own size *against the size it inherits* settled it: `text-sm` appears inside a `text-xs` parent
	// (larger), inside `text-2xs` (much larger) and inside `text-base` (smaller); `text-xs` appears inside
	// `text-sm` (smaller) and inside `text-3xs` (larger). A link's size in this corpus **overrides its context,
	// in both directions**, so it has no consistent meaning -- and naming kinds `meta`/`body` for it would
	// invent a semantics the data refuses, which is 007 §4.2 cutting the opposite way to the obvious reading.
	//
	// So the primitive is the sites carrying **no size at all**, which inherit their context. Everything with a
	// size is per-site typography and stays hand-written, the same reason seven `row` sites do.
	StaticLink StaticKind = "link"
)

// KnownStaticKinds is the closed set.
var KnownStaticKinds = map[StaticKind]bool{
	StaticEyebrow:   true,
	StaticHeading:   true,
	StaticParagraph: true,
	StaticLink:      true,
}
