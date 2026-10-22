package rendering

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

func renderUINode(t *testing.T, n ir.UINode) string {
	t.Helper()
	var buf bytes.Buffer
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

func trackedRowNode() ir.UINode {
	text := func(s string, track domain.RowTrack) ir.UINode {
		return ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticCaption), Props: map[string]string{"text": s}, Track: track}
	}
	return ir.UINode{Kind: ir.NodeLayout, Type: string(domain.LayoutRow), Props: map[string]string{"gap": "default", "mark": "chevron"},
		Children: []ir.UINode{text("name", domain.TrackFill), text("", domain.TrackField), text("2 pending", domain.TrackBadge)}}
}

// A column is a box around its child: every step draws its own whole literal classes, the two fixed ones only from `sm` up, and a child
// in no column is drawn with no box at all (the byte-identical case every existing screen is).
func TestRowTracks_eachStepDrawsItsOwnBox(t *testing.T) {
	got := renderUINode(t, trackedRowNode())
	for _, want := range []string{`class="min-w-0 grow"`, `class="min-w-0 sm:ml-2 sm:w-42 sm:shrink-0"`, `class="flex shrink-0 sm:w-30 sm:justify-end"`} {
		if !strings.Contains(got, want) {
			t.Errorf("row does not draw %s:\n%s", want, got)
		}
	}
	if n := strings.Count(got, `class="min-w-0 sm:ml-2 sm:w-42 sm:shrink-0"`); n != 1 {
		t.Errorf("the field column is drawn %d times, want once (an empty child still holds it)", n)
	}
	plain := trackedRowNode()
	for i := range plain.Children {
		plain.Children[i].Track = ""
	}
	if out := renderUINode(t, plain); strings.Contains(out, "sm:w-42") || strings.Contains(out, "min-w-0 grow") {
		t.Errorf("a child in no column was boxed:\n%s", out)
	}
}

// `mark: chevron` is the row's last child, aria-hidden, and a row without a mark draws none.
func TestRowMark_chevronEndsTheRowAndIsHiddenFromAssistiveTechnology(t *testing.T) {
	got := renderUINode(t, trackedRowNode())
	i := strings.Index(got, "<svg")
	if i < 0 || !strings.Contains(got[:i], `aria-hidden="true"`) || strings.Index(got, "2 pending") > i {
		t.Errorf("the chevron is not after the last child, or not hidden:\n%s", got)
	}
	bare := trackedRowNode()
	bare.Props = map[string]string{"gap": "default"}
	if strings.Contains(renderUINode(t, bare), "<svg") {
		t.Error("a row with no mark drew one")
	}
}

// `variant: row` is the strong ink and a hit area that is the row: the `after:` box stretches over the nearest positioned ancestor.
func TestLinkVariantRow_stretchesOverTheRow(t *testing.T) {
	link := ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticLink), Props: map[string]string{"href": "/x", "text": "Open", "variant": "row"}}
	got := renderUINode(t, link)
	if !strings.Contains(got, `href="/x"`) || !strings.Contains(got, "after:absolute after:inset-0") || !strings.Contains(got, ">Open</a>") {
		t.Errorf("row link = %s", got)
	}
	// An item's title in a list is the item role (15px), emphasised, in the strong ink.
	if want := textClass(context.Background(), domain.RoleItem); !strings.Contains(got, want) || !strings.Contains(got, "font-medium") || !strings.Contains(got, "text-ink-strong") {
		t.Errorf("row link does not read the item role %q, medium weight and strong ink: %s", want, got)
	}
	// And a Collection's divided item is the positioned ancestor it stretches over.
	item := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, "item"); return err })
	var buf bytes.Buffer
	if err := collectionBody(domain.GapDefault, "", false, true, true, false, "", []templ.Component{item}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<li class="relative `) {
		t.Errorf("a divided item is not positioned, so a row link would stretch over something else:\n%s", buf.String())
	}
}

// `inline: true` is a run of short items side by side and wrapping (a row of chips), and a Collection without it stays the column it was.
func TestCollectionInline_isARunNotAColumn(t *testing.T) {
	item := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, "chip"); return err })
	render := func(inline bool) string {
		var buf bytes.Buffer
		if err := collectionBody(domain.GapTight, "", false, false, false, inline, "", []templ.Component{item, item}).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(buf.String())
	}
	if got := render(true); !strings.HasPrefix(got, `<ul class="m-0 flex list-none flex-wrap`) || strings.Contains(got, "flex-col") {
		t.Errorf("an inline Collection is not a wrapping run:\n%s", got)
	}
	if got := render(false); !strings.HasPrefix(got, `<ul class="flex flex-col`) || strings.Contains(got, "list-none") {
		t.Errorf("a Collection without inline stopped being a column:\n%s", got)
	}
}

// `variant: chip` is the pill a filter chip is, drawn from the same classes, and `icon:` on a link with no variant puts the mark before
// its words and hides it from assistive technology.
func TestLinkVariantChipAndIconBeforeWords(t *testing.T) {
	chip := renderUINode(t, ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticLink), Props: map[string]string{"href": "/x", "text": "Asset booking", "variant": "chip"}})
	if want := `class="` + chipClasses(context.Background(), false) + `"`; !strings.Contains(chip, want) || !strings.Contains(chip, ">Asset booking</a>") {
		t.Errorf("chip link = %s\nwant the classes of a filter chip: %s", chip, want)
	}
	// Not circular: what a chip is, said once in words, so a change to chipClasses itself is seen (a bordered pill at the meta size, in the
	// control border and the secondary ink; a filter chip draws the identical string).
	for _, part := range []string{"rounded-full", "border px-3 py-1", "text-xs", "border-border-control", "text-ink-secondary"} {
		if !strings.Contains(chip, part) {
			t.Errorf("chip link does not carry %q: %s", part, chip)
		}
	}
	var filter bytes.Buffer
	if err := filterChipLink("Asset booking", "3", "/x", false).Render(context.Background(), &filter); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filter.String(), chipClasses(context.Background(), false)) {
		t.Errorf("a filter chip and a chip link are not one pill:\n%s", filter.String())
	}
	gear := renderUINode(t, ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticLink), Props: map[string]string{"href": "/s", "text": "Workspace settings", "icon": "settings"}})
	if i, j := strings.Index(gear, "<svg"), strings.Index(gear, "Workspace settings"); i < 0 || j < i || !strings.Contains(gear[:i], `aria-hidden="true"`) {
		t.Errorf("the mark is not before the words, or not hidden:\n%s", gear)
	}
}

// A search box with a button beside it takes the width that is left; one alone keeps its fixed width.
func TestSearchField_aBoxWithAButtonFillsTheWidth(t *testing.T) {
	render := func(submit string) string {
		var buf bytes.Buffer
		if err := searchField("Describe it", "idea", "e.g.", "", "/new", submit, "").Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if got := render("Start"); !strings.Contains(got, "[&amp;_button]:shrink-0 [&amp;_button]:whitespace-nowrap") {
		t.Errorf("the button can shrink and wrap its label:\n%s", got)
	}
	if got := render("Start"); !strings.Contains(got, "min-w-0 grow") || strings.Contains(got, "sm:w-70") || !strings.Contains(got, ` w-full"`) {
		t.Errorf("a box with a button does not fill the width:\n%s", got)
	}
	if got := render(""); !strings.Contains(got, "sm:w-70") || strings.Contains(got, "grow") {
		t.Errorf("a box alone lost its fixed width:\n%s", got)
	}
}

// An icon tile is an Application's face on its tint, or -- in a state -- the assistant's: the dashed draft outline and the solid pale
// indigo both read the generated-draft tokens, never an Application colour.
func TestIconTile_statesReadTheDraftTokens(t *testing.T) {
	tile := func(color string, state domain.TileState) string {
		return renderUINode(t, ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentIconTile), Props: map[string]string{"icon": "sparkle", "color": color, "size": "sheet", "state": string(state)}})
	}
	own := tile("blue", "")
	if !strings.Contains(own, "bg-app-blue-bg") || strings.Contains(own, "draft") {
		t.Errorf("an Application's tile does not read its tint:\n%s", own)
	}
	draft := tile("", domain.TileDraft)
	if !strings.Contains(draft, "border-dashed") || !strings.Contains(draft, "border-draft-edge") || !strings.Contains(draft, "text-draft-ink") {
		t.Errorf("the draft tile is not the dashed indigo outline:\n%s", draft)
	}
	assistant := tile("", domain.TileAssistant)
	if !strings.Contains(assistant, "bg-draft-edge/20") || !strings.Contains(assistant, "text-draft-ink") || strings.Contains(assistant, "border-dashed") {
		t.Errorf("the assistant tile is not the solid pale indigo:\n%s", assistant)
	}
	for name, got := range map[string]string{"own": own, "draft": draft, "assistant": assistant} {
		if !strings.Contains(got, `aria-hidden="true"`) {
			t.Errorf("%s tile is not hidden from assistive technology", name)
		}
	}
}

// `static: divider` is a rule and not text: an `<hr>` in the divider edge role, with no words and no margin of its own.
func TestStaticDivider_isAnHrInTheDividerRole(t *testing.T) {
	got := renderUINode(t, ir.UINode{Kind: ir.NodeStatic, Type: string(domain.StaticDivider)})
	if want := `<hr class="m-0 border-0 ` + dividerClasses(context.Background()) + `">`; strings.TrimSpace(got) != want {
		t.Errorf("divider = %s, want %s", got, want)
	}
	if !strings.Contains(got, "h-px") || !strings.Contains(got, "bg-border-divider") {
		t.Errorf("the rule is not 1px in the divider role: %s", got)
	}
}

// `density` sets the padding of a divided Collection's rows: regular is the 16px-by-12px row every list drew, roomy the 20px-by-14px row of
// a list whose rows carry an icon tile.
func TestCollectionDensity_setsADividedRowsPadding(t *testing.T) {
	item := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, "row"); return err })
	render := func(density domain.ListDensity, ordered bool) string {
		var buf bytes.Buffer
		if err := collectionBody(domain.GapDefault, "", ordered, true, false, false, density, []templ.Component{item}).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	for _, ordered := range []bool{false, true} {
		if got := render("", ordered); !strings.Contains(got, "px-4 py-3") || strings.Contains(got, "px-5") {
			t.Errorf("ordered=%v: the default row is not the regular padding:\n%s", ordered, got)
		}
		if got := render(domain.ListRoomy, ordered); !strings.Contains(got, "px-5 py-3.5") || strings.Contains(got, "px-4 py-3 ") {
			t.Errorf("ordered=%v: a roomy row does not carry the roomy padding:\n%s", ordered, got)
		}
	}
}
