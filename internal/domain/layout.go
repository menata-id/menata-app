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
)

// **`row`, `grid` and `panel` are measured in use and still deliberately absent, and the reason is worth
// reading before adding them.** They were written in the first pass of this slice and removed in the same
// one: all three ended with **zero callers**, because building a primitive is only half of "build the
// primitive, migrate the uses, then gate" -- and the other half is the half that proves it generic. The
// evidence was concrete rather than theoretical: the unused `gridLayout` put `sm:grid-cols-3` into the
// Tailwind bundle, a class no screen in the tree uses, which is a declaration growing the shipped CSS for
// a code path nothing can reach.
//
// Their measured use, so the next slice migrates rather than re-declares: `flex flex-wrap items-center
// gap-N` and its justify-between variants **11 times** (row), `grid` with `grid-cols-*` **25** (grid), the
// bordered `section` wrapper recurring throughout (panel). Add each *with* its callers.

// KnownLayoutKinds is the closed set. A kind absent from it is a load-time error, never a silent skip
// (capability-lifecycle.md §4 rule 3, "Unknown = explicit").
var KnownLayoutKinds = map[LayoutKind]bool{
	LayoutStack: true,
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
