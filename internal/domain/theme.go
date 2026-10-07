package domain

// RadiusRole is what a rounded thing *is*, as a closed set — the first slice of 006's Theme.
//
// **Three roles, measured.** Counted across every `.templ` on 2026-10-04, radius has three values carrying
// 185 of 194 uses: `md` 64 (controls — inputs, buttons), `lg` 60 (surfaces — panels, sections, cards), `full`
// 61 (pills — badges, avatars). The remaining nine are a bare `rounded` (7) and two single-side uses, which
// the inventory records as tail.
//
// Named for the job, not the measurement: 007 §4.2 asks semantic over visual, and `rounded-lg` is a class
// while `surface` is a meaning. The class mapping lives in `internal/rendering` alone (§15.2).
type RadiusRole string

const (
	// RadiusControl is an input, a button, a small interactive box.
	RadiusControl RadiusRole = "control"
	// RadiusSurface is a panel, a section, a card — something content sits inside.
	RadiusSurface RadiusRole = "surface"
	// RadiusPill is fully rounded: a badge, an avatar, a chip.
	RadiusPill RadiusRole = "pill"
)

// KnownRadiusRoles is the closed set.
var KnownRadiusRoles = map[RadiusRole]bool{RadiusControl: true, RadiusSurface: true, RadiusPill: true}

// RadiusStep is a step on the radius ladder, named by **amount rather than by framework token**. Three,
// because three are in use — a fourth would be a design space nobody asked for.
type RadiusStep string

const (
	RadiusStepSmall RadiusStep = "small"
	RadiusStepLarge RadiusStep = "large"
	RadiusStepFull  RadiusStep = "full"
)

// KnownRadiusSteps is the closed set.
var KnownRadiusSteps = map[RadiusStep]bool{RadiusStepSmall: true, RadiusStepLarge: true, RadiusStepFull: true}

// WeightRole is how strongly a piece of text is set, as a closed set — the second token category.
//
// **Two roles, and the measurement is why there are not three.** Counted 2026-10-04: `font-medium` 170 uses,
// `font-normal` 11, `font-semibold` 2, `font-bold` 1. The inventory's first verdict called the last two
// accidents to fold into medium — **re-measured per site, that was wrong on both**. `font-bold` sits on the
// brand mark (`bg-brand`, a 26px glyph), which is 006's *branding*, a separate concern from weight; and the
// two `font-semibold` are a role heading and an SLA badge, per-site stronger emphasis with no second
// consistent case. All three stay hand-written, so **this slice moves zero sites** rather than the three the
// inventory predicted.
type WeightRole string

const (
	// WeightBody is ordinary text, explicitly unemphasised — the four sites that reset an inherited weight (three `<th>`, one group header; re-measured 2026-10-06 by grepping `font-normal`).
	WeightBody WeightRole = "body"
	// WeightEmphasis is a heading, a label, a value: the thing a reader should land on. 170 uses, so this is
	// the default a primitive reaches for.
	WeightEmphasis WeightRole = "emphasis"
)

// KnownWeightRoles is the closed set.
var KnownWeightRoles = map[WeightRole]bool{WeightBody: true, WeightEmphasis: true}

// WeightStep is a step on the weight ladder. Two, because two carry 181 of 184 measured uses.
type WeightStep string

const (
	WeightStepNormal WeightStep = "normal"
	WeightStepMedium WeightStep = "medium"
)

// KnownWeightSteps is the closed set.
var KnownWeightSteps = map[WeightStep]bool{WeightStepNormal: true, WeightStepMedium: true}

// TextRole is what a piece of text *is*, as a closed set — the third token category and the largest (497
// uses over 7 values).
//
// **Eight roles, each one a size a primitive already hardcoded (caption shares `2xs` with label).** Measured: `sm` 194, `xs` 108, `2xs` 94,
// `3xs` 64, `xl` 24, `base` 11, `2xl` 2. Six were declared first, without `base`: its 11 sites had **no
// consistent role** *as counted*, so it stayed hand-written — a token needs a meaning, not just a count.
// Reading them (2026-10-06) split the 11: seven are one section's own title (RoleSubheading), two are a
// monogram and a menu label that are not headings, and the count had hidden that. `2xl` is kept despite 2
// uses because both are `metric`'s value, which is one clear role.
//
// Named for the job, not the measurement (007 §4.2): `text-sm` is a measurement, `body` is a meaning.
type TextRole string

const (
	// RoleDisplay is a headline number — the one thing on a tile a reader sees first.
	RoleDisplay TextRole = "display"
	// RoleHeading is a screen's own title.
	RoleHeading TextRole = "heading"
	// RoleSubheading is the title of one section inside a screen, below the screen's own RoleHeading: "Lists",
	// "Comments and activity", "History". It is the `base` size, which sat unclaimed for as long as the type
	// ladder existed; reading the seven `<h2 text-base font-medium>` sites (2026-10-06) found one job, and it
	// is not the panel title -- the 23 `text-sm font-medium` headings are body-sized and stay RoleBody.
	RoleSubheading TextRole = "subheading"
	// RoleBody is ordinary prose and row content. The most common by far (194 uses).
	RoleBody TextRole = "body"
	// RoleMeta is secondary information beside something else — a date, a count, a project name.
	RoleMeta TextRole = "meta"
	// RoleLabel is the text inside a badge or a pill: short, never a sentence.
	RoleLabel TextRole = "label"
	// RoleEyebrow is the small uppercase line naming what a screen belongs to.
	RoleEyebrow TextRole = "eyebrow"
	// RoleCaption is the help line under a block's title or a control: one sentence saying what the thing above it
	// does ("Choose a strong password you don't use elsewhere."). Same default size as RoleLabel and a different
	// job -- a label is the text *inside* a badge, a caption is prose *beside* a block -- so a Workspace can size
	// them apart. Eight sites moved to it (2026-10-06).
	RoleCaption TextRole = "caption"
)

// KnownTextRoles is the closed set.
var KnownTextRoles = map[TextRole]bool{
	RoleDisplay: true, RoleHeading: true, RoleSubheading: true, RoleBody: true,
	RoleMeta: true, RoleLabel: true, RoleEyebrow: true, RoleCaption: true,
}

// TextScale is a step on the type ladder, named by **amount in plain words** rather than by framework token —
// 007 §15.2 forbids the logical plane embedding a class, and `2xs` is Tailwind's spelling. Seven steps,
// because seven sizes are in use; `medium` is claimed by RoleSubheading alone.
type TextScale string

const (
	TextMicro  TextScale = "micro"
	TextTiny   TextScale = "tiny"
	TextSmall  TextScale = "small"
	TextNormal TextScale = "normal"
	TextMedium TextScale = "medium"
	TextLarge  TextScale = "large"
	TextHuge   TextScale = "huge"
)

// KnownTextScales is the closed set.
var KnownTextScales = map[TextScale]bool{
	TextMicro: true, TextTiny: true, TextSmall: true, TextNormal: true,
	TextMedium: true, TextLarge: true, TextHuge: true,
}

// BorderRole is what a border separates, as a closed set — the fifth token category, and the first slice of
// colour.
//
// **Three roles, each with a visibly distinct job**, which is what made border the right colour sub-category
// to take first: `slate-200` (70 uses, 24 files) outlines a **surface** — a panel, a section, a card, a table;
// `slate-300` (24) outlines a **control** — an input's own box, and the dashed circle of a pending avatar;
// `slate-100` (21) is a **divider** between rows. 115 of 155 border-colour uses, and no overlap between the
// three.
//
// Taken before text and background colour deliberately: those are 19 and 16 values where the semantic naming
// is the hard part (`slate-500` at 145 uses against `slate-400` at 103 — two shades of "secondary" with no
// measured line between them). Border has three jobs and three shades, one each.
type BorderRole string

const (
	// BorderSurface outlines something content sits inside. Read through `rendering.surfaceClasses`, so a hand-written
	// large-radius surface and `panelLayout`/`sectionLayout` move together; the thirteen rules that bound a region (a header, a table's head, the page's top bar and bottom nav) read it through
	// `borderClass` too, as does the edge of the eleven small boxes inside a surface (`tileClasses`, 2026-10-07); a button's outline does not.
	BorderSurface BorderRole = "surface"
	// BorderControl outlines something a person types into or interacts with.
	BorderControl BorderRole = "control"
	// BorderDivider separates rows in a list or a table.
	BorderDivider BorderRole = "divider"
)

// KnownBorderRoles is the closed set.
var KnownBorderRoles = map[BorderRole]bool{BorderSurface: true, BorderControl: true, BorderDivider: true}

// BorderShade is how visible a border is, named by presence rather than by palette number — `slate-300` is a
// Tailwind token and 007 §15.2 keeps those out of the logical plane. Three, because three are in use.
type BorderShade string

const (
	BorderFaint   BorderShade = "faint"
	BorderSoft    BorderShade = "soft"
	BorderDefined BorderShade = "defined"
)

// KnownBorderShades is the closed set.
var KnownBorderShades = map[BorderShade]bool{BorderFaint: true, BorderSoft: true, BorderDefined: true}

// InkRole is how much a piece of text matters, as a closed set — the seventh token category: **text colour**.
//
// **Four roles, and the fourth was the finding.** The decision (D2) first said three — strong / secondary /
// faint — folding `slate-600` (41) and `slate-700` (27) into their neighbours. Reading all 80 sites found
// they do not have one job. `slate-600` as running text is the **twin of `slate-500`**: `workspacemembers`
// renders the identical `{app.Name} · {role}` line at 600 on a member row and at 500 on an invitation row,
// and `rolematrix` renders the same `text-xs leading-5` paragraph at both — an accident, folded. But
// `slate-700` is the text of things you **read or act on** — menu items, a secondary button, a form label,
// the `<code>` values of the inference screen, a comment's body — one step above secondary, and folding it
// to 900 would darken 22 sites for no reason a reader could name. So it is `body`.
//
// **Excluded on purpose**: text *on a tinted chip* (23 sites at 600/700) is the `grey` TonePalette's job, not
// an ink role — a chip's colour is the bg+text pair its component owns; and a hover target is interaction
// state, not a role.
//
// **`faint` is for text that may be missed**: placeholders, hints, disabled and decorative text. `slate-400`
// on white is about 2.6:1, below WCAG AA's 4.5:1, so a Workspace mapping real content to `faint` is
// declaring text some readers cannot read. Whether the 100+ existing `slate-400` sites all qualify is a
// review nobody has done.
type InkRole string

const (
	// InkStrong is the text a screen is about: titles and the figures people scan for.
	InkStrong InkRole = "strong"
	// InkBody is the text of content and controls: what a person reads or clicks.
	InkBody InkRole = "body"
	// InkSecondary supports the content: captions, descriptions, meta lines.
	InkSecondary InkRole = "secondary"
	// InkFaint is text that may be missed without loss: hints, placeholders, decoration.
	InkFaint InkRole = "faint"
)

// KnownInkRoles is the closed set.
var KnownInkRoles = map[InkRole]bool{InkStrong: true, InkBody: true, InkSecondary: true, InkFaint: true}

// InkShade is how dark a text colour is, named by weight of presence rather than by palette number —
// `slate-700` is a Tailwind token and 007 §15.2 keeps those out of the logical plane.
type InkShade string

const (
	InkShadeDarkest InkShade = "darkest"
	InkShadeDark    InkShade = "dark"
	InkShadeMedium  InkShade = "medium"
	InkShadeLight   InkShade = "light"
)

// KnownInkShades is the closed set.
var KnownInkShades = map[InkShade]bool{InkShadeDarkest: true, InkShadeDark: true, InkShadeMedium: true, InkShadeLight: true}

// BackgroundRole is what a surface is *for*, as a closed set — the eighth token category: **background
// colour**, light surfaces only.
//
// Three roles, by elevation: `raised` is a card, a panel or a bar sitting on the page; `base` is the page
// itself; `sunken` is a lane or well recessed into it.
//
// **Read per site (2026-10-05) before building, and the reading both kept and qualified the decision.** The
// 153 uses the inventory counted are 78 white + 54 slate-50 + 38 slate-100 as plain backgrounds, and what is
// *not* a surface role takes most of the count away: 39 are `hover:` targets (interaction state), 21 are a
// grey chip's own fill (the `grey` TonePalette's pair), and 9 are `h-px` menu dividers (a rule, not a
// surface). `raised` is the clean one — 69 of 78 white sites are the same idiom, a rounded bordered card.
// `base` and `sunken` are thinner than their colours suggest: the page canvas is **two** sites, and the
// slate-50 wells (table header bands, inset blocks, icon wells — about 16) sit *on* a raised card, so by
// elevation they are `sunken` while by colour they are `base`. They stay hand-written: a default Theme that
// gave `sunken` slate-50 could not draw the one slate-100 lane, and one that gave it slate-100 would move 16
// screens, so the honest slice is the roles with a primitive or shared component to read them.
//
// **Dark surfaces are not a role** (D3): 17 uses that are three different jobs — the primary button, the
// scrim, a menu — and a role needs one job. Likewise the `-50` tints belong to the BadgeTone palettes.
type BackgroundRole string

const (
	// BackgroundRaised is a card, panel or bar sitting on the page.
	BackgroundRaised BackgroundRole = "raised"
	// BackgroundBase is the page behind everything else.
	BackgroundBase BackgroundRole = "base"
	// BackgroundSunken is a lane or well recessed into the page.
	BackgroundSunken BackgroundRole = "sunken"
)

// KnownBackgroundRoles is the closed set.
var KnownBackgroundRoles = map[BackgroundRole]bool{BackgroundRaised: true, BackgroundBase: true, BackgroundSunken: true}

// BackgroundShade is how far a light surface is from white, named by presence rather than by palette number
// (007 §15.2 keeps `slate-50` out of the logical plane).
type BackgroundShade string

const (
	BackgroundShadeLightest BackgroundShade = "lightest"
	BackgroundShadeLighter  BackgroundShade = "lighter"
	BackgroundShadeLight    BackgroundShade = "light"
)

// KnownBackgroundShades is the closed set.
var KnownBackgroundShades = map[BackgroundShade]bool{BackgroundShadeLightest: true, BackgroundShadeLighter: true, BackgroundShadeLight: true}

// TonePalette is the colour family a semantic tone renders in — the sixth token category, and the one that
// **moves an existing enum into the Theme rather than adding a new vocabulary**.
//
// `BadgeTone` already was a role set (neutral/info/good/bad/warn/muted) and each of its six members was
// already a **bg+text pair** hardcoded in `statusBadge`: `bg-amber-50 text-amber-800`,
// `bg-red-50 text-red-700`, and so on. So this is the same shape `Gap` has -- the role exists, only the
// amount was fixed -- and the entry maps tone straight to palette with no second role vocabulary.
//
// **Why this had to come before text and background colour** (decision D1 in `menata-app-document`'s
// `guides/design-system-decisions.md`): those six pairs own 42 of the `-50` tint uses and ~36 of the semantic
// text-colour uses. Defining colour roles first would have produced roles overlapping these tones, then
// required unpicking. And without it a Workspace could remap its surface backgrounds but **not** its badge
// colours, a split nobody could be told.
//
// Six palettes, 1:1 with the six tones by default. Two are grey at different intensities, which is why the
// palette is named rather than being a bare colour: `grey` and `grey-faint` are one family, two jobs.
type TonePalette string

const (
	PaletteGrey      TonePalette = "grey"
	PaletteGreyFaint TonePalette = "grey-faint"
	PaletteBlue      TonePalette = "blue"
	PaletteGreen     TonePalette = "green"
	PaletteRed       TonePalette = "red"
	PaletteAmber     TonePalette = "amber"
)

// KnownTonePalettes is the closed set.
var KnownTonePalettes = map[TonePalette]bool{
	PaletteGrey: true, PaletteGreyFaint: true, PaletteBlue: true,
	PaletteGreen: true, PaletteRed: true, PaletteAmber: true,
}

// PaddingRole names a place a primitive pads itself -- the ninth and last Theme category.
//
// **Roles carry an axis, which is how D4's two ladders are held.** `panel` and `section` pad all four sides;
// a badge pads its two axes separately because they are chosen separately (a regular badge is wider than it is
// tall by a factor the compact one does not share). Four badge roles rather than one role with two numbers keeps
// the Theme a flat role-to-amount map like its other eight, at the price of a longer list.
//
// **Only what a primitive draws is a role.** 416 padding sites over 67 values exist in `internal/rendering`;
// the ones drawn by `panelLayout`, `sectionLayout` and `statusBadge` have a single
// owner, so they are the ones that can follow a Workspace's declaration. The rest are hand-written and do not,
// the limit every category above carries. Three groups of those are not padding in the sense a role would mean,
// and are excluded on purpose rather than left as an unexplained tail (D5): `pt-18`/`sm:pt-28` on the auth shell
// is a vertical offset that places a card, `pb-24` on the main column is clearance for the fixed mobile bar
// (tied to that bar's own height), and `pr-11` on a password input is room for its trailing button. Each is
// sized by *another element*, so none is a design-system step.
type PaddingRole string

const (
	// PaddingPanel is `panelLayout`'s padding on all sides.
	PaddingPanel PaddingRole = "panel"
	// PaddingSection is `sectionLayout`'s. It is a separate role from PaddingPanel because the corpus holds
	// two values (D6): merging them is one line in a Workspace manifest, but doing it as the default would
	// move 12 or 21 screens, and that is an owner decision rather than a side effect of this slice.
	PaddingSection PaddingRole = "section"
	// PaddingBadgeX / PaddingBadgeY are a regular badge's horizontal and vertical room.
	PaddingBadgeX PaddingRole = "badge-x"
	PaddingBadgeY PaddingRole = "badge-y"
	// PaddingBadgeCompactX / PaddingBadgeCompactY are the compact badge's.
	PaddingBadgeCompactX PaddingRole = "badge-compact-x"
	PaddingBadgeCompactY PaddingRole = "badge-compact-y"
)

// KnownPaddingRoles is the closed set.
var KnownPaddingRoles = map[PaddingRole]bool{
	PaddingPanel: true, PaddingSection: true,
	PaddingBadgeX: true, PaddingBadgeY: true,
	PaddingBadgeCompactX: true, PaddingBadgeCompactY: true,
}

// PaddingAxis is which sides a role pads.
type PaddingAxis string

const (
	PaddingAxisAll PaddingAxis = "all"
	PaddingAxisX   PaddingAxis = "x"
	PaddingAxisY   PaddingAxis = "y"
)

// Axis is the sides this role pads.
func (r PaddingRole) Axis() PaddingAxis {
	switch r {
	case PaddingBadgeX, PaddingBadgeCompactX:
		return PaddingAxisX
	case PaddingBadgeY, PaddingBadgeCompactY:
		return PaddingAxisY
	default:
		return PaddingAxisAll
	}
}

// PaddingAmount is how much room a padding role renders -- on the same terms as GapAmount: names, not Tailwind
// classes (007 §15.2), and every one already present in the shipped CSS bundle from hand-written use, so
// remapping a role emits nothing new.
type PaddingAmount string

const (
	PaddingAmountHalf    PaddingAmount = "half"
	PaddingAmountOne     PaddingAmount = "one"
	PaddingAmountOneHalf PaddingAmount = "one-half"
	PaddingAmountTwo     PaddingAmount = "two"
	PaddingAmountTwoHalf PaddingAmount = "two-half"
	PaddingAmountThree   PaddingAmount = "three"
	PaddingAmountFour    PaddingAmount = "four"
	PaddingAmountFive    PaddingAmount = "five"
)

// PaddingAmountAllowed reports whether an amount is on the ladder for an axis.
//
// **Two ladders, and the second one is the finding.** D4 asked for `px` and `py` to be separate ladders because
// their distributions differ (`px-3` 50 and `px-4` 45 against `py-2` 24, `py-1.5` 19 and `py-2.5` 16). Counted
// again on 2026-10-06 over the amounts a primitive can reach, the *membership* of the two is identical -- eight
// amounts, half to five, all present on both -- so the distribution difference lives in each role's default and
// not in a different set. What genuinely differs is the all-sides ladder: `p-0.5`, `p-1` and `p-1.5` have no
// site anywhere, and admitting them would put three classes in the bundle that nothing uses, which is exactly
// what `GapAmount` refuses. The same reason holds the top of it at five: `p-6` is hand-written only behind a
// breakpoint, so a base `p-6` would be the one new class this slice emitted, and the first build showed exactly
// that before it was removed. So x and y share a case here on purpose, and a divergence is one case away.
func PaddingAmountAllowed(axis PaddingAxis, a PaddingAmount) bool {
	switch axis {
	case PaddingAxisX, PaddingAxisY:
		switch a {
		case PaddingAmountHalf, PaddingAmountOne, PaddingAmountOneHalf, PaddingAmountTwo,
			PaddingAmountTwoHalf, PaddingAmountThree, PaddingAmountFour, PaddingAmountFive:
			return true
		}
	case PaddingAxisAll:
		switch a {
		case PaddingAmountTwo, PaddingAmountTwoHalf, PaddingAmountThree, PaddingAmountFour,
			PaddingAmountFive:
			return true
		}
	}
	return false
}

// Theme is a Workspace's declared token set (006 "Theme": typography, spacing, icons, branding, tokens).
//
// **It holds no CSS and no business data.** 006's own warning is that a Theme "must not become a hidden
// business or data dependency", so this is a mapping between two closed vocabularies and nothing else.
//
// **All nine categories are declarable (radius, weight, text size, gap, border colour, semantic tone, text colour, background colour, padding), and each is read only by the primitives that draw it -- a measured boundary rather than a staged rollout.** The full token surface is
// 2,507 usages over 104 values in nine categories (`menata-app-document`'s
// `audits/2026-10-04-inventaris-token-design-system.md`). Radius is first because its ladder is the cleanest
// — three values carry 95% of its uses — so this slice proves the whole mechanism, declaration included,
// while moving the fewest sites. The other categories arrive one slice each, in the order that inventory
// sets, with padding and colour last because both certainly move pixels.
//
// **A Theme only Go can set is not a Theme** — that already exists under the names `Gap` and `BadgeTone`.
// This one is declared in a Workspace's own manifest (`theme:`) and resolved at load.
type Theme struct {
	// Radius maps every role to the step it renders at. A theme omitting a role inherits DefaultTheme's,
	// resolved by RadiusFor rather than at load, so the zero Theme — a pre-auth screen, or a Workspace with
	// no manifest — is a working answer rather than a missing one.
	Radius map[RadiusRole]RadiusStep
	// Weight maps every weight role to its step, on the same terms as Radius: an omitted role inherits the
	// default, resolved by WeightFor rather than at load.
	Weight map[WeightRole]WeightStep
	// Text maps every text role to its step, on the same terms as the two above.
	Text map[TextRole]TextScale
	// Gap maps each spacing step to the amount it renders. Unlike the three above this needs no role
	// vocabulary -- Gap already is the role. See domain.GapAmount.
	Gap map[Gap]GapAmount
	// Border maps each border role to its shade.
	Border map[BorderRole]BorderShade
	// Tone maps each semantic badge tone to the palette it renders in. Like Gap, this needs no role
	// vocabulary: BadgeTone already is the role.
	Tone map[BadgeTone]TonePalette
	// Ink maps each text-colour role to its shade.
	Ink map[InkRole]InkShade
	// Background maps each surface role to its shade.
	Background map[BackgroundRole]BackgroundShade
	// Padding maps each padding role to the amount it renders; validity is per axis (PaddingAmountAllowed).
	Padding map[PaddingRole]PaddingAmount
}

// DefaultTheme is what the corpus renders today, so adopting Theme changes nothing until a Workspace declares
// otherwise. Each pair is the class the primitive hardcoded before this file existed.
func DefaultTheme() Theme {
	return Theme{
		Radius: map[RadiusRole]RadiusStep{
			RadiusControl: RadiusStepSmall, // was rounded-md
			RadiusSurface: RadiusStepLarge, // was rounded-lg
			RadiusPill:    RadiusStepFull,  // was rounded-full
		},
		Weight: map[WeightRole]WeightStep{
			WeightBody:     WeightStepNormal, // was font-normal
			WeightEmphasis: WeightStepMedium, // was font-medium
		},
		Text: map[TextRole]TextScale{
			RoleDisplay:    TextHuge,   // was text-2xl
			RoleHeading:    TextLarge,  // was text-xl
			RoleSubheading: TextMedium, // was text-base
			RoleBody:       TextNormal, // was text-sm
			RoleMeta:       TextSmall,  // was text-xs
			RoleLabel:      TextTiny,   // was text-2xs
			RoleEyebrow:    TextMicro,  // was text-3xs
			RoleCaption:    TextTiny,   // was text-2xs, beside RoleLabel
		},
		Gap: map[Gap]GapAmount{
			GapTight:       GapAmountTwo,   // was gap-2
			GapDefault:     GapAmountThree, // was gap-3
			GapComfortable: GapAmountFour,  // was gap-4
			GapLoose:       GapAmountFive,  // was gap-5
		},
		Border: map[BorderRole]BorderShade{
			BorderSurface: BorderSoft,    // was border-slate-200
			BorderControl: BorderDefined, // was border-slate-300
			BorderDivider: BorderFaint,   // was border-slate-100
		},
		Tone: map[BadgeTone]TonePalette{
			ToneNeutral: PaletteGrey,      // was bg-slate-100 text-slate-600
			ToneInfo:    PaletteBlue,      // was bg-blue-50 text-blue-700
			ToneGood:    PaletteGreen,     // was bg-emerald-50 text-emerald-700
			ToneBad:     PaletteRed,       // was bg-red-50 text-red-700
			ToneWarn:    PaletteAmber,     // was bg-amber-50 text-amber-800
			ToneMuted:   PaletteGreyFaint, // was bg-slate-50 text-slate-400
		},
		Ink: map[InkRole]InkShade{
			InkStrong:    InkShadeDarkest, // was text-slate-900
			InkBody:      InkShadeDark,    // was text-slate-700
			InkSecondary: InkShadeMedium,  // was text-slate-500 (and slate-600 as running text, folded)
			InkFaint:     InkShadeLight,   // was text-slate-400
		},
		Background: map[BackgroundRole]BackgroundShade{
			BackgroundRaised: BackgroundShadeLightest, // was bg-white
			BackgroundBase:   BackgroundShadeLighter,  // was bg-slate-50
			BackgroundSunken: BackgroundShadeLight,    // was bg-slate-100
		},
		Padding: map[PaddingRole]PaddingAmount{
			PaddingPanel:         PaddingAmountFour,    // was p-4
			PaddingSection:       PaddingAmountFive,    // was p-5
			PaddingBadgeX:        PaddingAmountTwoHalf, // was px-2.5
			PaddingBadgeY:        PaddingAmountOne,     // was py-1
			PaddingBadgeCompactX: PaddingAmountTwo,     // was px-2
			PaddingBadgeCompactY: PaddingAmountHalf,    // was py-0.5
		},
	}
}

// InkFor resolves a text-colour role, falling back to the default for the same reason RadiusFor does.
func (t Theme) InkFor(role InkRole) InkShade {
	if s, ok := t.Ink[role]; ok && KnownInkShades[s] {
		return s
	}
	return DefaultTheme().Ink[role]
}

// BackgroundFor resolves a surface role, falling back to the default for the same reason RadiusFor does.
func (t Theme) BackgroundFor(role BackgroundRole) BackgroundShade {
	if s, ok := t.Background[role]; ok && KnownBackgroundShades[s] {
		return s
	}
	return DefaultTheme().Background[role]
}

// ToneFor resolves a semantic tone to its palette, falling back to the default for the same reason RadiusFor
// does.
func (t Theme) ToneFor(tone BadgeTone) TonePalette {
	if p, ok := t.Tone[tone]; ok && KnownTonePalettes[p] {
		return p
	}
	return DefaultTheme().Tone[tone]
}

// BorderFor resolves a border role, falling back to the default for the same reason RadiusFor does.
func (t Theme) BorderFor(role BorderRole) BorderShade {
	if b, ok := t.Border[role]; ok && KnownBorderShades[b] {
		return b
	}
	return DefaultTheme().Border[role]
}

// GapFor resolves a spacing step, falling back to the default for the same reason RadiusFor does.
func (t Theme) GapFor(g Gap) GapAmount {
	if a, ok := t.Gap[g]; ok && KnownGapAmounts[a] {
		return a
	}
	return DefaultTheme().Gap[g]
}

// TextFor resolves a text role, falling back to the default for the same reason RadiusFor does.
func (t Theme) TextFor(role TextRole) TextScale {
	if s, ok := t.Text[role]; ok && KnownTextScales[s] {
		return s
	}
	return DefaultTheme().Text[role]
}

// WeightFor resolves a weight role, falling back to the default for the same reason RadiusFor does: a role
// with no mapping would otherwise render with no weight class, which is a silent defect.
func (t Theme) WeightFor(role WeightRole) WeightStep {
	if s, ok := t.Weight[role]; ok && KnownWeightSteps[s] {
		return s
	}
	return DefaultTheme().Weight[role]
}

// RadiusFor resolves a role, falling back to the default rather than to the empty string: a role with no
// mapping would otherwise render with no radius class at all, which is a silent defect of exactly the kind
// this repository keeps finding.
func (t Theme) RadiusFor(role RadiusRole) RadiusStep {
	if s, ok := t.Radius[role]; ok && KnownRadiusSteps[s] {
		return s
	}
	return DefaultTheme().Radius[role]
}

// PaddingFor resolves a padding role, falling back to the default for the same reason RadiusFor does. An amount
// off the role's axis ladder falls back too: the loader refuses one, so reaching here with it means a Theme built
// in Go, and an unknown amount would otherwise render no padding class at all.
func (t Theme) PaddingFor(role PaddingRole) PaddingAmount {
	if a, ok := t.Padding[role]; ok && PaddingAmountAllowed(role.Axis(), a) {
		return a
	}
	return DefaultTheme().Padding[role]
}
