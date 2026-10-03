package domain

// LayoutKind is the closed set of generic spatial composition primitives
// (007-composable-runtime-architecture.md §12.2).
//
// **Four of §12.2's eight are built, and each arrived with the uses it replaces.** §12.2 lists `stack`,
// `row`, `columns`, `grid`, `split`, `tabs`, `panel` and `section`. Counted by *meaning* across the 38
// bespoke screens (see the retraction below for why the first two counts were wrong): vertical flow with a
// uniform gap appears **61 times** (`stack`), the bordered padded surface twelve times across six screens
// (`panel`), a responsive card grid **five** times (`grid`) and a main-plus-aside split **five** more
// (`split`). `row` (33 sites) is measured and next; `columns`, `tabs` and §12.2's own `section` have
// **zero** measured uses and are deliberately absent -- §12.2 is a permitted vocabulary, not a quota, and building the rest now would be
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
)

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
	LayoutStack: true,
	LayoutPanel: true,
	LayoutGrid:  true,
	LayoutSplit: true,
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
// **The spacings still hand-written are not a fifth step waiting to be declared.** The corpus also uses two
// *half*-steps between tight and default, across six `row` sites. Those are almost certainly accidental, and
// the `row` migration folds them into the two neighbours this ladder already has, which moves six sites by
// at most two pixels. That is a decision about the corpus, not about this ladder.
type Gap string

const (
	GapTight       Gap = "tight"
	GapDefault     Gap = "default"
	GapComfortable Gap = "comfortable"
	GapLoose       Gap = "loose"
)

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
)

// KnownStaticKinds is the closed set.
var KnownStaticKinds = map[StaticKind]bool{
	StaticEyebrow:   true,
	StaticHeading:   true,
	StaticParagraph: true,
}
