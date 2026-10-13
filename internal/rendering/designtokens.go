package rendering

import (
	"context"
	"strings"

	"menata.app/internal/domain"
)

// designTokenMap is the Design System artifact's token names, each mapped to the class the reader that draws it answers
// under ctx's Workspace. It is the one list two things read:
//   - GS1 (`TestDesignSystemTokensMatchTheCode`) holds the artifact's `tokens.json` to it under the default Theme;
//   - ExportDesignTokens (rencana 3.4) writes it as W3C DTCG tokens under any Workspace's Theme.
//
// Before 3.4 it lived inside GS1's test body. An exporter with its own copy would have been 001 #8: two lists of
// which reader answers for which token, free to drift apart.
//
// A token the artifact has and no reader answers for (brand, focus-ring, the shadows, control-height, ...) is not here;
// GS1's `dsUnmapped` gives each one's reason, and the export leaves it out for the same reason.
type designTokenMap struct {
	colour map[string]string
	length map[string]string
	styles map[string]designTypeStyle
}

// designTypeStyle is one of the artifact's type styles: a text role's size class and a weight role's class. tracking
// is the letter-spacing class the renderer draws beside it, or empty.
type designTypeStyle struct {
	size, weight, tracking string
}

// designTagHues are the artifact's names for the Tag palette; `TagPurple` draws the `violet` property.
var designTagHues = map[domain.TagColor]string{domain.TagBlue: "blue", domain.TagPurple: "violet", domain.TagAmber: "amber",
	domain.TagEmerald: "emerald", domain.TagCyan: "cyan", domain.TagRose: "rose", domain.TagSlate: "slate"}

func designTokenClasses(ctx context.Context) designTokenMap {
	colour := map[string]string{
		"ink-strong": inkClass(ctx, domain.InkStrong), "ink-body": inkClass(ctx, domain.InkBody),
		"ink-secondary": inkClass(ctx, domain.InkSecondary), "ink-faint": inkClass(ctx, domain.InkFaint),
		// DS-D10 (2026-10-12): the accent is a role now. Its hover is a state of the same key; compared as its colour.
		// DS-D16 (2026-10-12): the tone edge is part of the tone's palette now.
		"tone-warn-border": toneEdgeClass(ctx, domain.ToneWarn), "danger-border": toneEdgeClass(ctx, domain.ToneBad),
		"hover-wash": strings.TrimPrefix(hoverWashClass(ctx), "hover:"), // DS-D11: an alias of bg-base, read by the hover recipe
		"accent":     inkClass(ctx, domain.InkAccent), "accent-hover": strings.TrimPrefix(accentHoverClass(ctx), "hover:"),
		"bg-raised": backgroundClass(ctx, domain.BackgroundRaised), "bg-base": backgroundClass(ctx, domain.BackgroundBase),
		"bg-sunken":      backgroundClass(ctx, domain.BackgroundSunken),
		"border-divider": borderClass(ctx, domain.BorderDivider), "border-surface": borderClass(ctx, domain.BorderSurface),
		"border-control": borderClass(ctx, domain.BorderControl),
		"mark-good":      markGoodFill(),
		// Aliases of a role the artifact names for a component (checked 2026-10-13; they were listed unmapped with
		// reasons that predate the readers).
		"primary": inkFillClass(ctx, domain.InkStrong), "avatar-fill": borderFillClass(ctx, domain.BorderSurface),
		"danger-ink": toneInkClass(ctx, domain.ToneBad), "danger-wash": toneFillClass(ctx, domain.ToneBad),
		// DK8 (2026-10-13): the categorical colours, light and dark.
		"file-pdf-bg": strings.Fields(fileTypeTileClasses())[0], "file-pdf-ink": strings.Fields(fileTypeTileClasses())[1],
		"draft-edge": draftTileEdgeClass(), "draft-ink": draftTileInkClass(),
	}
	for tc, name := range designTagHues {
		chip, _ := tagClasses(tc)
		colour["tag-"+name] = strings.Fields(chip)[2] // the ink; fill and edge are it at 10% and 20%
	}
	for _, c := range []string{"blue", "emerald", "amber", "slate"} {
		f := strings.Fields(appIconClasses(c))
		colour["app-"+c+"-bg"], colour["app-"+c+"-ink"] = f[0], f[1]
	}
	for tone := range domain.KnownBadgeTones {
		colour["tone-"+string(tone)+"-bg"] = toneFillClass(ctx, tone)
		colour["tone-"+string(tone)+"-ink"] = toneInkClass(ctx, tone)
	}
	length := map[string]string{
		"gap-xtight": layoutGap(ctx, domain.GapXtight), "gap-tight": layoutGap(ctx, domain.GapTight),
		"gap-micro": layoutGap(ctx, domain.GapMicro), "gap-hair": layoutGap(ctx, domain.GapHair), // G1, 2026-10-12
		"gap-default": layoutGap(ctx, domain.GapDefault), "gap-comfortable": layoutGap(ctx, domain.GapComfortable),
		"gap-loose":              layoutGap(ctx, domain.GapLoose),
		"gap-region":             layoutGap(ctx, domain.GapRegion),
		"control-height-compact": strings.Fields(controlHeightCompact())[0], // the mouse height; the touch half is pointer-coarse
		"pad-panel":              paddingClass(ctx, domain.PaddingPanel), "pad-section": paddingClass(ctx, domain.PaddingSection),
		"pad-badge-x": paddingClass(ctx, domain.PaddingBadgeX), "pad-badge-y": paddingClass(ctx, domain.PaddingBadgeY),
		"pad-badge-compact-x": paddingClass(ctx, domain.PaddingBadgeCompactX),
		"pad-badge-compact-y": paddingClass(ctx, domain.PaddingBadgeCompactY),
		"radius-control":      radiusClass(ctx, domain.RadiusControl), "radius-surface": radiusClass(ctx, domain.RadiusSurface),
		"radius-pill": radiusClass(ctx, domain.RadiusPill),
	}
	emphasis, body := weightClass(ctx, domain.WeightEmphasis), weightClass(ctx, domain.WeightBody)
	styles := map[string]designTypeStyle{
		"display":       {size: textClass(ctx, domain.RoleDisplay), weight: emphasis},
		"heading":       {size: textClass(ctx, domain.RoleHeading), weight: emphasis},
		"subheading":    {size: textClass(ctx, domain.RoleSubheading), weight: emphasis},
		"panel-heading": {size: textClass(ctx, domain.RoleBody), weight: emphasis},
		"body":          {size: textClass(ctx, domain.RoleBody), weight: body},
		"meta":          {size: textClass(ctx, domain.RoleMeta), weight: body},
		"label":         {size: textClass(ctx, domain.RoleLabel), weight: body},
		"caption":       {size: textClass(ctx, domain.RoleCaption), weight: body},
		"item":          {size: textClass(ctx, domain.RoleItem), weight: emphasis},
		// staticText draws the eyebrow (and the overline) with `tracking-wide` beside the role; it is not a Theme role,
		// so it is named here as the renderer writes it.
		"eyebrow": {size: textClass(ctx, domain.RoleEyebrow), weight: body, tracking: "tracking-wide"},
	}
	return designTokenMap{colour: colour, length: length, styles: styles}
}
