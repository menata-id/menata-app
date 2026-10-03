package domain

// BadgeTone is the semantic urgency of a status badge, as a closed set.
//
// **Semantic, not visual** (007 §4.2): `good` and `bad` say what the state *means*, never that it is green
// or red. The colour mapping lives in `internal/rendering` for the same reason the Layout primitives' class
// strings do -- §15.2 forbids a logical representation embedding framework classes.
//
// This moved out of `internal/rendering` on 2026-10-03 (it was `rendering.PillTone`) because it is a closed
// set whose value is a bare string: vocabulary, which `conformance.TestDomainHoldsVocabularyAndRegistryHoldsDispatch`
// places here rather than in `internal/registry`. The *contract* that uses it is a dispatch seam and lives
// there.
type BadgeTone string

const (
	// ToneNeutral is a state with no urgency of its own -- a label, not a signal.
	ToneNeutral BadgeTone = "neutral"
	// ToneInfo is in progress, or awaiting someone.
	ToneInfo BadgeTone = "info"
	// ToneGood is settled favourably: approved, complete, nothing to act on.
	ToneGood BadgeTone = "good"
	// ToneBad is settled unfavourably, or overdue.
	ToneBad BadgeTone = "bad"
	// ToneWarn needs attention but is not yet a failure.
	ToneWarn BadgeTone = "warn"
	// ToneMuted is absent or not applicable -- the "uncast" case, deliberately distinct from neutral.
	ToneMuted BadgeTone = "muted"
)

// KnownBadgeTones is the closed set. A tone absent from it is a programming error, not a silent default --
// the renderer's own switch names every member, and a conformance gate holds both ends.
var KnownBadgeTones = map[BadgeTone]bool{
	ToneNeutral: true,
	ToneInfo:    true,
	ToneGood:    true,
	ToneBad:     true,
	ToneWarn:    true,
	ToneMuted:   true,
}

// ComponentType is the identity of a registered Component (007 §12.3, §14). Closed and known at compile
// time: `internal/registry.Components` is the catalogue, and a type string absent from it is refused rather
// than resolved, which is what makes the seam static rather than a plugin loader.
type ComponentType string

const (
	// ComponentStatusBadge is §12.3's own `StatusBadge`: one resolved state, rendered as a pill.
	ComponentStatusBadge ComponentType = "StatusBadge"
)

// ComponentContract is what §13 asks a Component to declare, as a Go value rather than as prose.
//
// **The field that carries the weight is `DataRequirements`, and it carries it by being empty.** §12.3's
// normative sentence is *"A Component MUST NOT silently acquire additional business data that is not
// represented by its contract"*, so a contract declaring no data requirements is a Component that may be
// handed resolved inputs and nothing else. That is not decoration: it is what made
// `rendering.slaBadgePill` -- which took `any`, parsed a date out of it, and called
// `experience.EvaluateSLA(due, time.Now())` -- a contract violation rather than a style preference, and
// what moved that evaluation to where `now` is an argument.
//
// `Renderer` names the function `internal/rendering` must provide. It is a **name, not a func value**, for
// the same plane reason `registry.Service` carries a validator and not an executor: this package may import
// `internal/domain` and nothing else, so it cannot hold a templ component. The binding is asserted by
// `conformance.TestComponentRegistryAndRenderersAgree`.
type ComponentContract struct {
	// Type is this Component's identity (§13's own first row).
	Type ComponentType
	// Inputs are the values a caller must supply. Every one is resolved before it arrives: a Component
	// takes a label, never a record to find the label in.
	Inputs []ComponentInput
	// DataRequirements are the Datasets this Component selects through, by id. **Empty is the normal and
	// preferred answer.** A non-empty entry is a promise that the Component reads exactly that and nothing
	// else, and `internal/metadata` can then check the Dataset exists -- but a Component needing one at all
	// is a sign the resolution belongs in `internal/composition` instead.
	DataRequirements []string
	// Slots are the named composition points this Component exposes (§12.5). Empty means a leaf.
	Slots []string
	// Actions are the events this Component can raise. Empty means it is display-only, which every
	// Component registered so far is.
	Actions []string
	// Accessibility records the semantics the renderer must carry, or "" when the element's own text is
	// the whole of it. Deliberately prose rather than an enum: nothing in this corpus has needed more than
	// one sentence, and inventing a vocabulary for one case is the shape-before-need 007 §34 forbids.
	Accessibility string
	// Renderer is the unexported `internal/rendering` function that draws this Component, by name.
	Renderer string
}

// ComponentInput is one declared input. `Kind` is the Go-level shape as a word ("string", "BadgeTone") --
// enough for a reader and for the boundedness gate to check the renderer's signature against, and
// deliberately not a type system of its own.
type ComponentInput struct {
	Name     string
	Kind     string
	Required bool
}
