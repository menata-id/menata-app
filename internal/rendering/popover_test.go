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

func renderPopover(t *testing.T, ctx context.Context, label, title string, tr domain.PopoverTrigger, al domain.PopoverAlign, d domain.PopoverDensity, body string) string {
	t.Helper()
	var buf bytes.Buffer
	inner := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := w.Write([]byte(body)); return err })
	if err := popover(label, title, tr, al, d).Render(templ.WithChildren(ctx, inner), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// The record page's "..." menu and the board header's "New <machine>" form drew this markup by hand. The expected
// strings are what the first rendered at 376acf0 (captured before the migration) with the one deliberate change: the
// panel opens 6px under its trigger, where it was 4px and 8px. The body now sits in a wrapper that carries the room.
func TestPopover_reproducesThePreMigrationMarkup(t *testing.T) {
	const wantMenu = `<details class="relative"><summary aria-label="Card actions" class="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-border-surface bg-bg-raised px-4 font-sans text-sm text-ink-body hover:bg-hover-wash cursor-pointer list-none">⋯</summary><div class="absolute top-full h-0 w-0 left-0"><div class="fixed z-10 mt-1.5 w-72  max-sm:inset-x-4 max-sm:top-auto max-sm:bottom-24 max-sm:z-40 max-sm:mt-0 max-sm:w-auto max-sm:max-h-96 max-sm:translate-x-0 max-sm:overflow-y-auto rounded-lg border border-border-surface bg-bg-raised shadow-lg"><div class="flex flex-col gap-1 p-2">ROWS</div></div></div></details>`
	if got := renderPopover(t, context.Background(), "⋯", "Card actions", domain.PopoverTriggerSecondary, domain.PopoverAlignStart, domain.PopoverDensityCompact, "ROWS"); got != wantMenu {
		t.Errorf("menu:\n got %s\nwant %s", got, wantMenu)
	}
	const wantNew = `<details class="relative"><summary class="cursor-pointer list-none rounded-md bg-ink-strong px-3 py-1.5 text-xs font-medium text-bg-raised hover:bg-ink-strong/90">New task</summary><div class="absolute top-full h-0 w-0 right-0"><div class="fixed z-10 mt-1.5 w-72 -translate-x-full max-sm:inset-x-4 max-sm:top-auto max-sm:bottom-24 max-sm:z-40 max-sm:mt-0 max-sm:w-auto max-sm:max-h-96 max-sm:translate-x-0 max-sm:overflow-y-auto rounded-lg border border-border-surface bg-bg-raised shadow-lg"><div class="p-4">FORM</div></div></div></details>`
	if got := renderPopover(t, context.Background(), "New task", "", domain.PopoverTriggerFilled, domain.PopoverAlignEnd, domain.PopoverDensityRegular, "FORM"); got != wantNew {
		t.Errorf("new card:\n got %s\nwant %s", got, wantNew)
	}
}

// R9: the trigger, the corner, the edge, the fill and the room follow the Workspace's theme, and the label escapes.
func TestPopover_followsTheTheme(t *testing.T) {
	th := domain.DefaultTheme()
	th.Radius[domain.RadiusSurface] = domain.RadiusStepFull
	th.Border[domain.BorderSurface] = domain.BorderDefined
	th.Background[domain.BackgroundRaised] = "#fffdf5"
	th.Gap[domain.GapMicro] = domain.GapAmountThree
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "m", Theme: th}, "m", false)
	got := resolveColourRoles(t, ctx, renderPopover(t, ctx, `a <b>`, "", domain.PopoverTriggerSecondary, domain.PopoverAlignStart, domain.PopoverDensityCompact, ""))
	for _, want := range []string{"rounded-full", "border-slate-300", "bg-[#fffdf5]", "gap-3", ">a &lt;b&gt;</summary>"} {
		if !strings.Contains(got, want) {
			t.Errorf("a declared theme did not move the popover (%q missing): %s", want, got)
		}
	}
}

// A declared page reaches the Component through `uiNode`, and its children land in the panel.
func TestPopover_isDrawnWhenAPageDeclaresItWithItsBody(t *testing.T) {
	var buf bytes.Buffer
	n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentPopover),
		Props:    map[string]string{"label": "More", "trigger": "secondary", "align": "end", "density": "regular"},
		Children: []ir.UINode{{Kind: ir.NodeStatic, Type: string(domain.StaticMessage), Props: map[string]string{"text": "Inside."}}}}
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "<details") || !strings.Contains(got, ">More</summary>") || !strings.Contains(got, "right-0") ||
		strings.Index(got, "Inside.") < strings.Index(got, "</summary>") || strings.Index(got, "Inside.") > strings.Index(got, "</details>") {
		t.Errorf("the declared popover is not drawn with its body in the panel: %s", got)
	}
}

// formLabelClasses is one look for every label that wraps its control inside a floating form (K-17).
func TestFormLabelClasses_followTheTheme(t *testing.T) {
	if got := resolveColourRoles(t, context.Background(), formLabelClasses(context.Background())); got != "flex flex-col gap-1 text-xs font-medium text-slate-700" {
		t.Errorf("default look = %q", got)
	}
	th := domain.DefaultTheme()
	th.Text[domain.RoleMeta] = domain.TextMedium
	th.Weight[domain.WeightEmphasis] = domain.WeightStepNormal
	th.Gap[domain.GapMicro] = domain.GapAmountThree
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "m", Theme: th}, "m", false)
	got := resolveColourRoles(t, ctx, formLabelClasses(ctx))
	for _, want := range []string{"text-base", "font-normal", "gap-3"} {
		if !strings.Contains(got, want) {
			t.Errorf("a declared theme did not move the label (%q missing): %s", want, got)
		}
	}
}

// The panel is `fixed`, so the scrolling box of a table cannot cut it, and below `sm` it gets offsets of its own: a gutter in from each
// side above the bottom bar, wherever its trigger sits. Anchored to the trigger it ran off the left edge of a phone and was cut by the
// table it opened from (the Categories screen, 2026-10-19).
func TestPopover_isFixedToTheViewportOnAPhone(t *testing.T) {
	for _, al := range []domain.PopoverAlign{domain.PopoverAlignStart, domain.PopoverAlignEnd} {
		got := renderPopover(t, context.Background(), "Edit", "Edit", domain.PopoverTriggerSecondary, al, domain.PopoverDensityRegular, "FORM")
		for _, want := range []string{`class="fixed `, "max-sm:inset-x-4", "max-sm:bottom-24", "max-sm:translate-x-0", "max-sm:overflow-y-auto", "max-sm:max-h-96"} {
			if !strings.Contains(got, want) {
				t.Errorf("align %s: the panel does not carry %q: %s", al, want, got)
			}
		}
	}
}
