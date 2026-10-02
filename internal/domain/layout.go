package domain

// LayoutKind is the closed set of generic spatial composition primitives
// (007-composable-runtime-architecture.md §12.2).
//
// **Four of §12.2's eight, and the four are the ones measured in use.** §12.2 lists `stack`, `row`,
// `columns`, `grid`, `split`, `tabs`, `panel` and `section`; counted across the 38 bespoke screens on
// 2026-10-02, `flex flex-col gap-N` (stack) appears **61 times**, `flex flex-wrap items-center gap-N` and
// its justify-between variants 11 (row), `grid` with `grid-cols-*` 25, and the bordered `section` wrapper
// (panel) recurs throughout. `columns`, `split`, `tabs` and §12.2's own `section` have **zero** measured
// uses, so they are deliberately absent: §12.2 is a permitted vocabulary, not a quota, and building the
// rest now would be the shape-before-need 007 §34 forbids.
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
)

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
}

// Gap is the spacing between a layout's children, as an enum rather than a number.
//
// Deliberately not a pixel or rem value: a declaration that carries `gap: 12px` has smuggled a physical
// presentation choice into the logical plane, which is what §15.2 forbids and what would make this a page
// builder rather than a composition vocabulary. Three steps, because the measured corpus uses
// `gap-1`/`gap-2` (tight), `gap-3`/`gap-4` (default) and `gap-5` (loose) and nothing else.
type Gap string

const (
	GapTight   Gap = "tight"
	GapDefault Gap = "default"
)

// KnownGaps is the closed set. An empty Gap means GapDefault, resolved where the layout is rendered.
var KnownGaps = map[Gap]bool{
	GapTight:   true,
	GapDefault: true,
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
