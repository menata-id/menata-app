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

// **`row` and `grid` are not primitives in this corpus, and the measurement that settled it corrected an
// earlier claim of mine.** The Experience Plane kajian said all four of these were "measured in use",
// counting *frequency*. Counting distinct **shapes** instead (2026-10-02):
//
//   - `panel`: 12 sites, **one** class string. A primitive, and it is here.
//   - `row`: 21 sites, **six** shapes -- `items-center gap-2` (7), `gap-1.5` (5),
//     `items-center justify-between gap-3` (4), `gap-2` (3), plus two one-offs carrying `items-baseline`,
//     `gap-x`/`gap-y`, or a bottom border.
//   - `grid`: 6 sites, **six** shapes, every one an arbitrary track list (`grid-cols-[6rem_1fr]`,
//     `lg:grid-cols-[1fr_360px]`, `lg:grid-cols-[1fr_340px]`). There is no closed column vocabulary to
//     declare.
//
// Forcing `row`'s six shapes through one primitive either changes what renders or gives the primitive
// enough knobs to become the unbounded `GenericComponent` §12.3 forbids by name. `grid`'s tracks are a
// layout *language*, which is §15.2's "CSS framework classes" with a different syntax.
//
// So they stay hand-written, and this comment is the reason rather than an omission. What would change it:
// a second case that genuinely shares one of `row`'s shapes -- the 7 `items-center gap-2` sites are the
// most promising -- audited for whether they mean the same thing or merely look alike.

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
