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

// Theme is a Workspace's declared token set (006 "Theme": typography, spacing, icons, branding, tokens).
//
// **It holds no CSS and no business data.** 006's own warning is that a Theme "must not become a hidden
// business or data dependency", so this is a mapping between two closed vocabularies and nothing else.
//
// **Radius only, and that is a measured boundary rather than a staged rollout.** The full token surface is
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
}

// DefaultTheme is what the corpus renders today, so adopting Theme changes nothing until a Workspace declares
// otherwise. Each pair is the class the primitive hardcoded before this file existed.
func DefaultTheme() Theme {
	return Theme{Radius: map[RadiusRole]RadiusStep{
		RadiusControl: RadiusStepSmall, // was rounded-md
		RadiusSurface: RadiusStepLarge, // was rounded-lg
		RadiusPill:    RadiusStepFull,  // was rounded-full
	}}
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
