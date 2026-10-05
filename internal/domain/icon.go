package domain

// KnownIcons is the closed set of icon names the runtime can actually draw -- the same
// static-seam posture as KnownFieldTypes, KnownNavigationBadges and KnownApplicationColors
// (007 §14), and closed for the identical reason KnownApplicationColors is: the renderer maps
// each name to a *drawing* (internal/rendering.icon's own switch), so a name nothing draws
// would render an empty box. Failing at load beats rendering nothing.
//
// This replaced `icon: "▣"` -- a single literal character per item -- on 2026-09-24. That
// convention was never meant to last: metadata/applications/document-approval.yaml's own comment
// called the glyphs "placeholders standing in for a real SVG icon set ... swapping to one is a
// separate, not-yet-scoped piece of work", and the Flow 2 mockup scoped it by drawing every icon
// as inline stroke SVG (owner request, 2026-09-24: "pilihkan juga icon yang style nya cocok dan
// bisa dipakai"). A character also cannot carry the one property that set needs -- a consistent
// stroke weight and cap style across every icon on a screen -- because it belongs to whichever
// font happened to have that codepoint.
//
// A *name* rather than a path: metadata declares which icon, never how it is drawn. A path in
// YAML would put rendering in the Domain Plane and make every icon un-restyleable at once, which
// is the same mistake as writing a Tailwind class into a manifest.
//
// Two groups, one set. Most names are declarable on an Application or a NavigationItem
// (`icon:`, validated in internal/metadata). The rest -- home, grid, more, chevron-right,
// chevron-down, switch -- are the runtime's own chrome, drawn by appShell and never named by any
// manifest; they live here anyway so one gate (internal/conformance.TestKnownIconsAreAllDrawn)
// covers every name the renderer must handle, rather than two lists that can disagree.
//
// The drawings follow the Flow 2 mockup's own style, which is what makes them one set rather than
// a collection: 24x24 viewBox, no fill, stroke: currentColor at 1.8, round caps and joins.
// DeclarableIcons is the subset of KnownIcons a manifest -- or, since 2026-09-27, the AI Metadata
// Assistant's own generated Application -- may actually name (Application.Icon,
// NavigationItem.Icon). Split out from KnownIcons's own remaining chrome-only names so
// aiassist/schema.go can build a Gemini Enum from exactly this set: an Enum is a structural
// guarantee (the API cannot return a value outside it), unlike a value merely *described* as
// closed, which is what icon's own schema field said until this line existed and Gemini could not
// actually see the list -- found 2026-09-27 watching a real conversation invent "Palette",
// "Sparkles" then "FileText" in three straight turns, each rejected, none from this set.
var DeclarableIcons = map[string]bool{
	"check":        true, // Document Approval's own mark
	"board":        true, // Project Management: a column board
	"inbox":        true,
	"file-text":    true,
	"check-square": true,
	"dashboard":    true,
	"calendar":     true,
	"timer":        true,
	"bar-chart":    true,
	"list":         true,
	"bolt":         true,
	"settings":     true,
	// user-check: Assigned to me (2026-09-24) -- a person with a check mark, distinct from
	// "check" alone (Document Approval's own mark) the way the two screens themselves are
	// distinct: one is the Application, the other is "approval steps naming you".
	"user-check": true,
}

// chromeOnlyIcons are this runtime's own chrome, drawn by appShell -- no manifest, and nothing
// generated, may declare one.
var chromeOnlyIcons = []string{
	"home", "grid", "more", "chevron-right", "chevron-down", "switch", "trash",
	// sparkle: the Workspace menu's own "New application" link (AI Metadata Assistant, Flow 2 gap
	// study Tahap 8) -- chrome, not a declarable Application/nav icon, the same posture "switch"
	// already has.
	"sparkle",
	// bell: the header's own notification badge button (Flow 2 gap study Tahap 6) -- chrome, the
	// same posture "switch"/"sparkle" already have.
	"bell",
}

var KnownIcons = func() map[string]bool {
	out := make(map[string]bool, len(DeclarableIcons)+len(chromeOnlyIcons))
	for name := range DeclarableIcons {
		out[name] = true
	}
	for _, name := range chromeOnlyIcons {
		out[name] = true
	}
	return out
}()
