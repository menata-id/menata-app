package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

func renderSegments(t *testing.T, ctx context.Context, label, query, options, selected string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := segmentedControl(label, query, options, selected).Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// `viewSwitcher` drew this markup by hand. The expected string is what it rendered at 8aad1a7, captured before the
// migration; the one difference is the link, which was `/machines/mch_document?view=...` and is the same query on the
// page's own route now (the control holds no route).
func TestSegmentedControl_reproducesThePreMigrationMarkup(t *testing.T) {
	const want = `<nav class="inline-flex gap-0.5 rounded-md border border-border-surface bg-bg-raised p-0.5" aria-label="View"><span class="rounded bg-bg-sunken px-3 py-1 text-xs font-medium text-ink-strong" aria-current="true">Table</span><a class="rounded px-3 py-1 text-xs text-ink-secondary hover:text-ink-strong" href="?view=vw_document_cards">Cards</a></nav>`
	m := docMachine()
	var buf bytes.Buffer
	if err := viewSwitcher(m, m.Views[0]).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// R9: the track, knob, corner and inks follow the Workspace's theme, and the label and every value are escaped.
func TestSegmentedControl_followsTheThemeAndEscapes(t *testing.T) {
	th := domain.DefaultTheme()
	th.Radius[domain.RadiusControl] = domain.RadiusStepFull
	th.Border[domain.BorderSurface] = domain.BorderDefined
	th.Background[domain.BackgroundRaised] = "#fffdf5"
	th.Background[domain.BackgroundSunken] = "#e8e4d8"
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "m", Theme: th}, "m", false)
	got := resolveColourRoles(t, ctx, renderSegments(t, ctx, `a <b> & "c"`, "v", "x=One\ny z=<Two>", "x"))
	for _, want := range []string{"rounded-full", "border-slate-300", "bg-[#fffdf5]", "bg-[#e8e4d8]", `aria-label="a &lt;b&gt; &amp; &#34;c&#34;"`, `href="?v=y+z"`, ">&lt;Two&gt;<"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing: %s", want, got)
		}
	}
}

// The selected option's label is escaped too (the review found only the unselected one was tested).
func TestSegmentedControl_escapesTheSelectedLabel(t *testing.T) {
	got := renderSegments(t, context.Background(), "L", "p", "a=<A>\nb=B", "a")
	if !strings.Contains(got, `aria-current="true">&lt;A&gt;</span>`) {
		t.Errorf("the selected label is not escaped: %s", got)
	}
}

// Exactly the selected option is marked, and it is not a link.
func TestSegmentedControl_marksOnlyTheSelectedOption(t *testing.T) {
	got := renderSegments(t, context.Background(), "L", "p", "a=A\nb=B\nc=C", "b")
	if strings.Count(got, `aria-current="true"`) != 1 || strings.Count(got, "<a ") != 2 || !strings.Contains(got, `aria-current="true">B</span>`) {
		t.Errorf("one span marked, two links expected: %s", got)
	}
}

// A declared page reaches the Component through `uiNode`.
func TestSegmentedControl_isDrawnWhenAPageDeclaresIt(t *testing.T) {
	var buf bytes.Buffer
	n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentSegmentedControl), Props: map[string]string{"label": "Range", "query": "range", "options": "week=Week\nmonth=Month", "selected": "month"}}
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `aria-label="Range"`) || !strings.Contains(got, `href="?range=week"`) || !strings.Contains(got, `aria-current="true">Month</span>`) {
		t.Errorf("the declared control is not drawn: %s", got)
	}
}
