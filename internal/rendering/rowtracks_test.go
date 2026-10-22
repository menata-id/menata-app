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
	for _, want := range []string{`class="min-w-0 grow"`, `class="min-w-0 sm:w-42 sm:shrink-0"`, `class="flex sm:w-30 sm:shrink-0 sm:justify-end"`} {
		if !strings.Contains(got, want) {
			t.Errorf("row does not draw %s:\n%s", want, got)
		}
	}
	if n := strings.Count(got, `class="min-w-0 sm:w-42 sm:shrink-0"`); n != 1 {
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
	// And a Collection's divided item is the positioned ancestor it stretches over.
	item := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, "item"); return err })
	var buf bytes.Buffer
	if err := collectionBody(domain.GapDefault, "", false, true, true, []templ.Component{item}).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<li class="relative `) {
		t.Errorf("a divided item is not positioned, so a row link would stretch over something else:\n%s", buf.String())
	}
}
