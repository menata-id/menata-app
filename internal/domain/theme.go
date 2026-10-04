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
	// WeightBody is ordinary text, explicitly unemphasised — the 11 sites that reset an inherited weight.
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

// Theme is a Workspace's declared token set (006 "Theme": typography, spacing, icons, branding, tokens).
//
// **It holds no CSS and no business data.** 006's own warning is that a Theme "must not become a hidden
// business or data dependency", so this is a mapping between two closed vocabularies and nothing else.
//
// **Radius and weight only, and that is a measured boundary rather than a staged rollout.** The full token surface is
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
	}
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
