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
	// ComponentAvatar is §12.3's own `Avatar`: one person as a circle of initials.
	ComponentAvatar ComponentType = "Avatar"
	// ComponentMetric is §12.3's own `Metric`: one headline number with its label.
	//
	// **Registered by building its case rather than by waiting for one** (owner, 2026-10-03: *"buatkan case
	// agar case nya jadi ada. ini kan umum ada di aplikasi bukan?"*). The second-case rule governs invented
	// *semantics*; a metric tile is structural vocabulary §12.3 closes by name and every application has, so
	// the question is not whether it recurs but whether this runtime can compose one. It now can, and four
	// screens do.
	ComponentMetric ComponentType = "Metric"
	// ComponentCollection is §12.3's own `Collection`: an ordered list of already-composed items.
	//
	// **The first Component with a non-empty `Slots`**, which is what makes it §12.5's first real consumer.
	// Its items arrive as rendered children rather than as records, so the Component never selects, sorts or
	// formats anything -- a collection that took `[]*data.Record` would be taking the data layer with it.
	ComponentCollection ComponentType = "Collection"
	// ComponentField is §12.3's own `Field`: a labelled form control.
	//
	// **It already existed unregistered.** `rendering.wizardField(id, label)` was exactly this -- a wrapper,
	// a `<label for>`, and `{ children... }` for the control -- and `authField` carried the identical wrapper
	// and label with one hardcoded `<input>` after them. So registering it is naming what was there, and both
	// migrate byte-identically.
	//
	// Second Component with a slot, after `Collection`. The slot is why this is two inputs rather than the six
	// `authField` took: a Field labels a control, it does not know what kind of control.
	ComponentField ComponentType = "Field"
	// ComponentButton is §12.3's own `Button`: one action a person takes, drawn as a control.
	//
	// **The vocabulary existed as three strings.** `controlPrimary`, `controlSecondary` and `controlDanger` were
	// read by 36 sites in 16 files, so the *look* of a button was already one decision; what was missing was
	// that nothing a Workspace declared could reach it (five Theme keys were valid YAML with no reader) and that
	// a screen could not compose one without hand-writing the tag. It takes the plain case only -- label,
	// variant, and the native `name`/`value` pair a submit button posts -- and leaves behaviour (`hx-*`), the
	// choice of element and per-site spacing to the sites that need them, which still draw their own tag but
	// read the same classes through `rendering.buttonClasses`.
	ComponentButton ComponentType = "Button"
	// ComponentTag is §12.3's own `Tag`: a name set against a record, drawn as a pill with a colour dot.
	//
	// **It already existed unregistered, five times.** `rendering.tagChip(CardTag)` drew a board card's labels,
	// the calendar's, the record detail's, My Tasks' and Board Settings' list, every one passing the same
	// resolved `{Label, Color}`. Registering it names what was there. The colour is an entry of the closed
	// palette (`TagColor`, 007 §15.2: a Workspace picks a name, never a value), and the label is always drawn,
	// so the colour is never the only thing that says which tag this is.
	ComponentTag ComponentType = "Tag"
	// ComponentForm is §12.3's own `Form` and 007 §11.3's write side: one create form over a Dataset's Machine.
	//
	// **An author names a Dataset and a submit label, and nothing else.** The route it posts to, the Fields it
	// asks for and the kind of control each takes are all derived from the Machine (`Machine.CreateFormInputs`),
	// so a page holds no `name=` and no route, and the generic create route -- permission, validation, defaults,
	// events -- is the only thing that writes. The slot is `field`; lowering fills it, an author does not.
	ComponentForm ComponentType = "Form"
	// ComponentFormInput is the control a `Field` labels inside a `Form`. **Lowering emits it; an author may not
	// write it**, because an author-typed control is a hand-typed `name=`, which is exactly what §11.3's Binding
	// exists to remove (`ir.dimensionRoute` refuses a typed `href` for the same reason). Named `FormInput` and
	// not `Input` because `ComponentInput` is already a contract's declared input.
	ComponentFormInput ComponentType = "Input"
)

// InputKind is the shape of control an `Input` draws, as a closed set. It is a semantic ("a date"), never an
// HTML attribute: the renderer decides the element, so a Theme can change a control without a page noticing.
type InputKind string

const (
	InputText     InputKind = "text"
	InputLongText InputKind = "longtext"
	InputNumber   InputKind = "number"
	InputDate     InputKind = "date"
	InputBoolean  InputKind = "boolean"
	InputSelect   InputKind = "select"
)

// KnownInputKinds is the closed set.
var KnownInputKinds = map[InputKind]bool{
	InputText:     true,
	InputLongText: true,
	InputNumber:   true,
	InputDate:     true,
	InputBoolean:  true,
	InputSelect:   true,
}

// InputOptionsSep joins a select's options into the one string property an `Input` carries them in. A newline,
// because an option is a status name and a comma is a character a status may hold.
const InputOptionsSep = "\n"

// ButtonVariant is what a button means to the person reading the screen, as a closed set -- not how it looks,
// which is `rendering.buttonClasses`' job and the Workspace's Theme's to move (§4.2: a semantic declaration
// carries `primary`, never a colour).
type ButtonVariant string

const (
	// ButtonPrimary is the one affirmative action on a screen: Save, Add, Approve, Continue.
	ButtonPrimary ButtonVariant = "primary"
	// ButtonSecondary is every action beside it: Cancel, Edit, Delete, a link styled as a button.
	ButtonSecondary ButtonVariant = "secondary"
	// ButtonDanger is a destructive decision the screen wants read before it is clicked -- today only Reject.
	// Deliberately not used for Delete: a row's Delete is ordinary and reversible by re-creating the record,
	// while Reject ends an approval.
	ButtonDanger ButtonVariant = "danger"
)

// KnownButtonVariants is the closed set.
var KnownButtonVariants = map[ButtonVariant]bool{
	ButtonPrimary:   true,
	ButtonSecondary: true,
	ButtonDanger:    true,
}

// AvatarSize is how large an avatar circle is, as a closed set rather than a number -- the same reason
// `Gap` is a ladder and not a pixel count (§15.2: a logical declaration carrying `size: 40px` has smuggled a
// physical choice into the plane that must not know about them).
//
// Three steps: the four migrated sites used two, and a Board card (Case 19's PM01) is the third. Named for the role the size plays rather than for the
// measurement, so a change of scale is not a change of vocabulary.
type AvatarSize string

const (
	// AvatarInline sits in a list row beside other content.
	AvatarInline AvatarSize = "inline"
	// AvatarLead heads a detail screen, where the person is the subject rather than a row.
	AvatarLead AvatarSize = "lead"
	// AvatarCompact sits in the footer of a dense card, where a row-sized circle would outweigh the title.
	AvatarCompact AvatarSize = "compact"
)

// KnownAvatarSizes is the closed set.
var KnownAvatarSizes = map[AvatarSize]bool{
	AvatarInline:  true,
	AvatarLead:    true,
	AvatarCompact: true,
}

// AvatarPresence distinguishes a person who is here from one who has only been asked.
//
// **It is semantic, not visual** (007 §4.2), and the distinction is real rather than decorative: an
// invitation is not a membership (this repo has a standing rule about exactly that), so the dashed circle on
// the Members screen says "asked, not joined" and must not be reachable by passing a colour.
type AvatarPresence string

const (
	AvatarPresent AvatarPresence = "present"
	AvatarPending AvatarPresence = "pending"
)

// KnownAvatarPresences is the closed set.
var KnownAvatarPresences = map[AvatarPresence]bool{
	AvatarPresent: true,
	AvatarPending: true,
}

// BadgeSize is how much room a `StatusBadge` takes around its label, as a closed set for the reason
// `AvatarSize` is one. Measured over the 15 hand-written grey chips that migrate to it (2026-10-05): six sit in
// a dense row beside other text (`px-2 py-0.5`) and the other nine are the main thing on their line
// (`px-2.5 py-1`), which is two steps, not a count to declare.
//
// Both are literals inside `rendering.statusBadge` today and become Theme padding steps when the padding
// category lands (design-system-decisions D4) -- the size is the semantic that stays.
type BadgeSize string

const (
	// BadgeRegular is a badge that is the subject of its own line or cell. It is also what an unset size means.
	BadgeRegular BadgeSize = "regular"
	// BadgeCompact is a badge sitting in a dense row, where a regular one would outweigh the text beside it.
	BadgeCompact BadgeSize = "compact"
)

// KnownBadgeSizes is the closed set.
var KnownBadgeSizes = map[BadgeSize]bool{
	BadgeRegular: true,
	BadgeCompact: true,
}

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
