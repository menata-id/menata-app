package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// handWrittenRadii freezes, per file, the literal corner classes left in `internal/rendering` (2026-10-12,
// rencana 0.2b). A corner that is a Theme role reads `radiusClass(ctx, domain.Radius*)`, so `theme.radius` moves it:
// - `control` (small): inputs, buttons, menu rows, and the small boxes inside a surface;
// - `surface` (large): cards, panels, dialogs, empty-state boxes;
// - `pill` (full): badges and chips.
//
// 57 sites moved, and 106 pages plus 12 component renders were byte-identical. The audit counted 98; the measured
// population was 117.
//
// **Read by meaning (menjaga-design-system R2).** Each entry below is a corner the three roles do not describe, or
// one whose move would be a decision:
//   - **circles and bars** (avatars, step circles, dots, progress bars, a switch, the sheet grabber): geometry. A
//     theme moving `pill` would square them;
//   - **icon tiles drawn `rounded-lg`** (`size-8`..`size-10`): the artifact's nearest token is `radius-brand-mark` (10px),
//     which has no Theme role. A small `rounded-md` tile (a file-type tile, a `size-6` app icon) is a box inside a
//     surface and reads `control`, the `tileClasses` precedent;
//   - **menu rows drawn `rounded-lg`**, beside other menu rows drawn `rounded-md`. Folding them is a decision (R1);
//   - **`rounded`** (4px) and **`rounded-xl`**: off the ladder, and owner decision DS-D17 (radius 4px/sheet) is open;
//   - **pre-auth** screens: no Workspace on ctx, so no theme reaches them;
//   - chat bubbles, which have their own shape.
//
// Counted after `joinClassLiterals` (string literals only, so a reader call is not a corner), whole-corner tokens only.
// So the "117" is whole-corner tokens. **Not counted:** side corners and arbitrary values (`rounded-t-2xl` on the
// sheet, `rounded-r-md` and `rounded-[10px]` in the pre-auth shell, `rounded-tl-sm` on a signature marker; four in
// the tree on 2026-10-12), variant-prefixed corners, and a corner returned by a Go function (`avatarClasses`).
var handWrittenRadii = map[string]struct {
	n   int
	why string
}{
	"account.templ":            {2, "the notification switch's track and knob (geometry)"},
	"approvalinbox.templ":      {3, "the progress bar's track and fill, and the approver chip's initials circle (geometry)"},
	"approvalstepper.templ":    {3, "three step circles (geometry)"},
	"appshell.templ":           {8, "four rounded-lg icon tiles and the workspace monogram (brand-mark); the bell and avatar buttons' circles and the sheet grabber (geometry). The five rounded-lg menu rows moved to radius.control (owner decision P1, 2026-10-12)"},
	"authshell.templ":          {3, "pre-auth inputs and submit button: no theme reaches them"},
	"checklist.templ":          {1, "the completion circle (geometry)"},
	"chooseworkspace.templ":    {1, "pre-auth dashed create-workspace link"},
	"detail.templ":             {1, "rounded (4px) signature image: off the ladder, DS-D17"},
	"documentsubmit.templ":     {1, "the approver step-number circle (geometry)"},
	"installapplication.templ": {1, "the Application icon tile (brand-mark)"},
	"login.templ":              {2, "pre-auth inputs: no theme reaches them"},
	"machine.templ":            {7, "rounded (4px) view-switch tab, link, edit summary and tag-ish label (DS-D17); the lane's rounded-xl (the artifact folded lanes to radius-surface in the mockups, a visible change); the tag dot and completion circle (geometry)"},
	"newapplication.templ":     {2, "the assistant chat bubbles (their own shape)"},
	"notifications.templ":      {2, "the unread dot and its placeholder (geometry)"},
	"progress.templ":           {2, "the progress bar's track and fill (geometry)"},
	"reviewdocument.templ":     {6, "rounded (4px) reference code and signature box (DS-D17); four signature-marker and timeline circles (geometry)"},
	"signatureplacement.templ": {5, "rounded (4px) signature markers and width input (DS-D17); two marker circles (geometry)"},
	"workspacehome.templ":      {3, "three Application icon tiles (brand-mark)"},
}

var radiusToken = regexp.MustCompile(`^rounded(?:-(?:sm|md|lg|xl|2xl|full))?$`)

func TestHandWrittenRadiiOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	actual := map[string]int{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") || e.Name() == "layout.templ" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range classAttr.FindAllStringSubmatch(joinClassLiterals(stripTemplComments(string(b))), -1) {
			for _, tok := range strings.Fields(m[1]) {
				if radiusToken.MatchString(tok) {
					actual[e.Name()]++
				}
			}
		}
	}
	if len(actual) == 0 {
		t.Fatal("no literal radius found anywhere -- either every corner reads the Theme (keep this gate, empty) or the pattern stopped matching")
	}
	seen := map[string]bool{}
	for f := range actual {
		seen[f] = true
	}
	for f := range handWrittenRadii {
		seen[f] = true
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		got, want := actual[f], handWrittenRadii[f].n
		switch {
		case got > want:
			t.Errorf("%s: %d literal corner(s), frozen at %d -- a control, surface or pill corner reads radiusClass(ctx, domain.Radius*), "+
				"so theme.radius moves it; a corner no role describes goes in handWrittenRadii with what it is", f, got, want)
		case got < want:
			t.Errorf("%s: %d literal corner(s), down from %d -- lower handWrittenRadii to lock it in", f, got, want)
		}
	}
}

// TestRadiusRewriteMatchesTheDefaultTheme holds `radiusDefaults`, the hand-written table `radiusReadersAsLiterals`
// uses, to the code it stands for: `domain.DefaultTheme`'s step for each role, mapped through `radiusClass`'s own
// arms in layout.templ. Without it a change to the default Theme would leave the shape gates reading the old corner
// (found by the 2026-10-12 independent review).
func TestRadiusRewriteMatchesTheDefaultTheme(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(), "internal", "rendering", "layout.templ"))
	if err != nil {
		t.Fatal(err)
	}
	body := regexp.MustCompile(`(?s)func radiusClass\(.*?\n\}`).FindString(string(b))
	arms := map[string]string{}
	for _, m := range regexp.MustCompile(`case domain\.(RadiusStep\w+):\s*return "([^"]+)"`).FindAllStringSubmatch(body, -1) {
		arms[m[1]] = m[2]
	}
	steps := map[domain.RadiusStep]string{domain.RadiusStepSmall: "RadiusStepSmall", domain.RadiusStepLarge: "RadiusStepLarge", domain.RadiusStepFull: "RadiusStepFull"}
	if len(arms) != len(steps) {
		t.Fatalf("parsed %d radiusClass arms, want %d -- the scan is reading the wrong text", len(arms), len(steps))
	}
	roles := map[string]domain.RadiusRole{"RadiusControl": domain.RadiusControl, "RadiusSurface": domain.RadiusSurface, "RadiusPill": domain.RadiusPill}
	if len(roles) != len(domain.KnownRadiusRoles) || len(radiusDefaults) != len(roles) {
		t.Fatalf("radius roles %d, known %d, radiusDefaults %d -- a role was added; extend radiusDefaults and this map", len(roles), len(domain.KnownRadiusRoles), len(radiusDefaults))
	}
	for name, role := range roles {
		want := arms[steps[domain.DefaultTheme().RadiusFor(role)]]
		if radiusDefaults[name] != want {
			t.Errorf("radiusDefaults[%s] = %q, but the default Theme renders %q", name, radiusDefaults[name], want)
		}
	}
}
