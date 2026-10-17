package domain

import (
	"fmt"
	"strings"
)

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
	// ComponentProgress is a part of a whole drawn as a bar: a label, how many, out of how many.
	//
	// **It already existed unregistered, twice.** The Dashboard's "Open cards per person" (`personLoadRow`) and the
	// approval card's progress (`approvalProgress`) each drew a `role="progressbar"` filled to value/max with the
	// same words beside it. Registering it is what lets a `page:` draw one per Dimension value, the Dashboard's
	// bars being the first (K15). Both numbers arrive resolved; the Component computes only the fill.
	ComponentProgress ComponentType = "Progress"
	// ComponentCallout is one message about what just happened or what needs attention, drawn as a tinted box:
	// "Password updated.", "Wrong password.".
	//
	// **It already existed unregistered, six times**, on the account screens: the same tinted `<p>` with the
	// tone's fill and ink, written out by hand at each site, two of the six with no `role` and one with
	// `role="status"`. The Design System artifact's similarity report (GS7) found it as the one fingerprint
	// group that was one meaning in two files. Registering it moves the choice of live-region role from the
	// site into the contract, where a tone decides it.
	ComponentCallout ComponentType = "Callout"

	// ComponentEmptyState is the card a list draws when it has nothing in it: a dashed box holding one message,
	// "Nothing pending your approval.", "No groups yet. Create one to grant a role to several people at once.".
	//
	// **It already existed unregistered, five times in two files, byte for byte**: the Approval Inbox's `inboxEmpty`
	// (called five times) and a hand-written copy of its class set on the Groups screen. It is the *card*, not every
	// way a screen says "nothing": a declared `Collection`'s `empty:` draws a plain `static: message`, and
	// whether it should draw this card instead is a visible change and the owner's decision
	// (design-system-decisions.md, "Tombol, rencana 2.3a" was the last such note; this one is "Keadaan kosong").
	ComponentEmptyState ComponentType = "EmptyState"

	// ComponentDisclosure is a panel that opens on a click to show what is behind its summary: "+ New group" over the
	// form that creates one, "+ Invite member" over the invitation form. It is a native `<details>`, so opening and
	// closing needs no script and no request, and it is the first Component to declare an action that is not a
	// write: `toggle`.
	//
	// **It already existed unregistered, twice**: Groups and Members drew the same surface, the same
	// accent summary and a form under a rule. It is the *panel* disclosure only. The tail-folding "Show all N" link
	// (`foldedRest`, the record detail's fold) hides its own summary once open, the card menus open a floating list, and the Archived-workspaces fold is a quiet line on a pre-auth-adjacent screen; each is a different job.
	ComponentDisclosure ComponentType = "Disclosure"

	// ComponentSegmentedControl is a short row of mutually exclusive choices of one page, the selected one marked: the
	// record list's "Board / List / Calendar" switch (`viewSwitcher`), which every Machine with two or more Views draws.
	//
	// **It is the navigation shape only.** Each unselected option is a link to the page's own route with one query
	// parameter changed, and the selected one carries `aria-current`. The Builder mockup's `role="radiogroup"` control
	// looks like it but is another job: a choice held in the page, switched by a script. The 2026-10-11 kajian counted
	// the two as one "second case"; they share a track and a knob, and differ in element, semantics and behaviour
	// (the second needs 007 §12.3's `actions`). The radiogroup is not registered here.
	ComponentSegmentedControl ComponentType = "SegmentedControl"

	// ComponentPopover is a floating panel that opens under a trigger and holds a short form or a short list of actions:
	// the record page's "..." card-actions menu and the board header's "New <machine>" form. It is a native `<details>`
	// positioned under its trigger, so opening and closing needs no script (the same `toggle` Disclosure declares).
	//
	// **It was registered when an Application screen first needed one** (gap plan K-04, 2026-10-15). T-A had closed on
	// "register a popover when an Application screen, not the shell, needs a menu", and the 2026-10-14 audit found two
	// already hand-drawn there: they differ in their trigger's look, the side the panel opens to, its padding, and a few
	// pixels of margin, so those four are the contract's closed inputs. The app shell's own menus are a different thing
	// (`sheetPanel`, the native Popover API with a bottom-sheet form on a phone) and stay outside. The board card's
	// quick-edit and a lane's "Add a card" expand in place; they are not popovers.
	ComponentPopover ComponentType = "Popover"

	// ComponentTable is a records table: a header row from the columns a page declares and one row per record, each cell
	// one of the record's Projection roles. It is `dataTable`, the one `<table>` this tree draws (007 §12.3 `Table`; 006
	// "List View = Collection + Projection + Table renderer", K12), made nameable by a `page:`: the frame, the header cells
	// and the scroll box are the table's, and the cells arrive composed, the way a Collection's items do.
	//
	// **Records only.** A dimension-bound table (several Measures per Dimension row, the Dashboard's shape) is the next
	// case and is refused by name rather than half built; its first consumer is the bespoke Dashboard, which this does not
	// migrate.
	ComponentTable ComponentType = "Table"

	// ComponentNavList is a list of the app's own destinations, grouped under headings, one of them marked as where the
	// viewer is: the settings sidebar and the settings hub are both one. A page names only the section
	// (`section: workspace_settings`) and how it is drawn (`variant: sidebar|page`); the rows are **derived** from the
	// navigation items that declare themselves part of that section (`NavigationItem.SettingsSection`), so the sidebar and
	// the hub cannot disagree about what exists, and a destination is declared once (001 #8). A row for a screen that is
	// not built is drawn as such and links nowhere.
	ComponentNavList ComponentType = "NavList"
)

// KnownNavListVariants is how a NavList is drawn: a sidebar beside a screen (hidden below the `sm` breakpoint, where the
// hub is the list) or a page-wide list with each row's description.
var KnownNavListVariants = map[string]bool{"sidebar": true, "page": true}

// NavSectionWorkspaceSettings is the one section a NavList can list today.
const NavSectionWorkspaceSettings = "workspace_settings"

// KnownNavSections is the closed set a NavList's `section:` may name.
var KnownNavSections = map[string]bool{NavSectionWorkspaceSettings: true}

// NavSectionHub is the navigation item each section leads back to: the hub whose rows the section lists.
var NavSectionHub = map[string]string{NavSectionWorkspaceSettings: "nav_workspace_settings"}

// KnownCalloutTones is the part of `BadgeTone` a Callout may take. `neutral` and `muted` are labels, not
// messages: a message that is neither news, nor good, nor bad, nor a warning has no reason to be boxed.
var KnownCalloutTones = map[BadgeTone]bool{
	ToneInfo: true,
	ToneGood: true,
	ToneBad:  true,
	ToneWarn: true,
}

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

// ComponentStatus says how far an author may rely on a Component's contract (rencana 2.0, 2026-10-13). Closed
// vocabulary: it is a word an author reads in `writing-guide.md` §12.1a and a gate compares, not a feature flag.
//
// **The three words are tied to a measurement, not to opinion.** `stable` means an installed Workspace's manifest
// depends on the contract: a `page:` declares the Component, or lowering emits it for one that does (every Form
// emits a Field and an Input), so changing an input breaks a manifest that exists. `experimental` means no
// installed manifest depends on it yet, so the contract may still change; promoting it is a decision made when the
// first page uses it. `deprecated` means a successor exists (`ReplacedBy`) and new pages should use that.
// `conformance.TestComponentStatusMatchesInstalledUse` holds the first two to the installed manifests, both ways.
//
// **Nothing at runtime branches on it.** A `deprecated` Component still renders and still loads. The day one is
// deprecated, `metadata.validatePages` is where a new use of it should be refused or warned about, naming
// `ReplacedBy`; that is not built because no Component is deprecated and a branch no input can reach is the
// shape-before-need 007 §34 refuses.
type ComponentStatus string

const (
	ComponentStatusExperimental ComponentStatus = "experimental"
	ComponentStatusStable       ComponentStatus = "stable"
	ComponentStatusDeprecated   ComponentStatus = "deprecated"
)

// KnownComponentStatuses is the closed set of ComponentStatus values.
var KnownComponentStatuses = map[ComponentStatus]bool{
	ComponentStatusExperimental: true,
	ComponentStatusStable:       true,
	ComponentStatusDeprecated:   true,
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
	// Status is how far an author may rely on this contract (see ComponentStatus). Required: a contract with no
	// status is refused by StatusProblems.
	Status ComponentStatus
	// ReplacedBy names the Component new pages should use instead. Required when Status is deprecated, and
	// refused otherwise: a successor on a Component that is not deprecated would say two things at once.
	ReplacedBy ComponentType
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

// StatusProblems checks the contract's own status fields, which need no registry: the status is one of the known
// words, and ReplacedBy is present exactly when the Component is deprecated and does not name itself. That the
// successor is a *registered* Component is the registry's to check (`registry.CatalogueProblems`), because only
// it has the catalogue.
func (c ComponentContract) StatusProblems() []string {
	var issues []string
	switch {
	case c.Status == "":
		issues = append(issues, fmt.Sprintf("%s declares no Status (want one of experimental, stable, deprecated)", c.Type))
	case !KnownComponentStatuses[c.Status]:
		issues = append(issues, fmt.Sprintf("%s declares Status %q, which is not experimental, stable or deprecated", c.Type, c.Status))
	}
	switch {
	case c.Status == ComponentStatusDeprecated && c.ReplacedBy == "":
		issues = append(issues, fmt.Sprintf("%s is deprecated and names no ReplacedBy -- an author needs to know what to use instead", c.Type))
	case c.Status != ComponentStatusDeprecated && c.ReplacedBy != "":
		issues = append(issues, fmt.Sprintf("%s names ReplacedBy %q but is %s -- only a deprecated Component has a successor", c.Type, c.ReplacedBy, c.Status))
	case c.ReplacedBy == c.Type && c.ReplacedBy != "":
		issues = append(issues, fmt.Sprintf("%s names itself as its own ReplacedBy", c.Type))
	}
	return issues
}

// SegmentOption is one choice of a SegmentedControl: the value the page's query parameter takes and the words drawn.
type SegmentOption struct {
	Value string
	Label string
}

// SegmentOptionSep separates a SegmentedControl option's value from its label inside the one `options` string property
// (`board=Board`). The first one splits, so a label may hold the character; a value may not.
const SegmentOptionSep = "="

// ParseSegmentOptions reads the `options` property of a SegmentedControl: one `value=Label` per line
// (`InputOptionsSep`). A blank line is skipped, so a YAML `|` block's trailing newline is not read as an empty option
// (the independent review found only `|-` loaded). An entry with no separator reads as a value with an empty label, which the registry's validator
// refuses; the renderer therefore never meets one from a loaded page.
func ParseSegmentOptions(s string) []SegmentOption {
	if s == "" {
		return nil
	}
	var out []SegmentOption
	for _, line := range strings.Split(s, InputOptionsSep) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		v, l, _ := strings.Cut(line, SegmentOptionSep)
		out = append(out, SegmentOption{Value: v, Label: l})
	}
	return out
}

// JoinSegmentOptions is the inverse, for a caller that holds the choices already resolved.
func JoinSegmentOptions(opts []SegmentOption) string {
	parts := make([]string, len(opts))
	for i, o := range opts {
		parts[i] = o.Value + SegmentOptionSep + o.Label
	}
	return strings.Join(parts, InputOptionsSep)
}

// PopoverTrigger is how a Popover's trigger looks: `secondary` is a Button's outline recipe (the record page's "..."),
// `filled` is the strong ink as a fill with the on-fill text, the board header's "New <machine>".
type PopoverTrigger string

const (
	PopoverTriggerSecondary PopoverTrigger = "secondary"
	PopoverTriggerFilled    PopoverTrigger = "filled"
)

// KnownPopoverTriggers is the closed set.
var KnownPopoverTriggers = map[PopoverTrigger]bool{PopoverTriggerSecondary: true, PopoverTriggerFilled: true}

// PopoverAlign is the side of its trigger a Popover's panel lines up with: `start` opens toward the right from the
// trigger's left edge, `end` toward the left from its right edge.
type PopoverAlign string

const (
	PopoverAlignStart PopoverAlign = "start"
	PopoverAlignEnd   PopoverAlign = "end"
)

// KnownPopoverAligns is the closed set.
var KnownPopoverAligns = map[PopoverAlign]bool{PopoverAlignStart: true, PopoverAlignEnd: true}

// PopoverDensity is the room a Popover gives its body: `compact` is a menu of rows (8px padding, 4px between rows),
// `regular` is a form (16px padding, the body lays itself out).
type PopoverDensity string

const (
	PopoverDensityCompact PopoverDensity = "compact"
	PopoverDensityRegular PopoverDensity = "regular"
)

// KnownPopoverDensities is the closed set.
var KnownPopoverDensities = map[PopoverDensity]bool{PopoverDensityCompact: true, PopoverDensityRegular: true}
