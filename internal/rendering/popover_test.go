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
	const wantMenu = `<details class="group relative" data-popover><summary aria-label="Card actions" class="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-border-surface bg-bg-raised px-4 font-sans text-sm text-ink-body hover:bg-hover-wash cursor-pointer list-none">⋯</summary>   <div class="absolute top-full h-0 w-0 left-0"><div class="fixed z-10 mt-1.5 w-72  max-sm:inset-x-4 max-sm:top-auto max-sm:bottom-24 max-sm:z-40 max-sm:mt-0 max-sm:w-auto max-sm:max-h-96 max-sm:translate-x-0 max-sm:overflow-y-auto rounded-lg border border-border-surface bg-bg-raised shadow-lg"><div class="flex flex-col gap-1 p-2">ROWS</div></div></div></details>`
	if got := renderPopover(t, context.Background(), "⋯", "Card actions", domain.PopoverTriggerSecondary, domain.PopoverAlignStart, domain.PopoverDensityCompact, "ROWS"); got != wantMenu {
		t.Errorf("menu:\n got %s\nwant %s", got, wantMenu)
	}
	// A body that is a form is a dialog: a title and a close mark on top, a scrolling body, the page dimmed behind it from `sm` up.
	got := renderPopover(t, context.Background(), "New task", "", domain.PopoverTriggerFilled, domain.PopoverAlignEnd, domain.PopoverDensityRegular, "FORM")
	for _, want := range []string{`<details class="group relative" data-popover>`, `role="dialog" aria-label="New task"`, `data-popover-close aria-label="Close"`,
		`class="min-h-0 overflow-y-auto p-4">FORM</div>`, `sm:group-open:block`, `sm:inset-0 sm:m-auto sm:h-fit`, `inset-x-4 bottom-24`} {
		if !strings.Contains(got, want) {
			t.Errorf("the dialog does not carry %q: %s", want, got)
		}
	}
	// ...and no tap outside closes it: nothing in the markup is a light dismiss.
	if strings.Contains(got, "popover=") || strings.Contains(got, "onclick") {
		t.Errorf("a dialog must close only on purpose: %s", got)
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
		Props:    map[string]string{"label": "More", "trigger": "secondary", "align": "end", "density": "compact"},
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

// A menu's panel is `fixed`, so the scrolling box of a table cannot cut it, and below `sm` it gets offsets of its own: a gutter in from
// each side above the bottom bar, wherever its trigger sits. Anchored to the trigger it ran off the left edge of a phone and was cut
// by the table it opened from (the Categories screen, 2026-10-19).
func TestPopover_isFixedToTheViewportOnAPhone(t *testing.T) {
	for _, al := range []domain.PopoverAlign{domain.PopoverAlignStart, domain.PopoverAlignEnd} {
		got := renderPopover(t, context.Background(), "...", "Actions", domain.PopoverTriggerSecondary, al, domain.PopoverDensityCompact, "ROWS")
		for _, want := range []string{`class="fixed `, "max-sm:inset-x-4", "max-sm:bottom-24", "max-sm:translate-x-0", "max-sm:overflow-y-auto", "max-sm:max-h-96"} {
			if !strings.Contains(got, want) {
				t.Errorf("align %s: the panel does not carry %q: %s", al, want, got)
			}
		}
	}
}

// A Form inside a dialog draws Cancel beside its submit, in a row pinned under the scrolling body; a Form elsewhere is the submit alone.
func TestForm_drawsCancelAndPinsItsRowOnlyInsideADialog(t *testing.T) {
	render := func(ctx context.Context) string {
		var buf bytes.Buffer
		if err := form("Save", "/x", "", false, "", "").Render(templ.WithChildren(ctx, templ.Raw("<i>controls</i>")), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	inside := render(withPopover(context.Background()))
	for _, want := range []string{`data-popover-close aria-label="Cancel"`, ">Cancel</button>", "sticky -bottom-4", ">Save</button>"} {
		if !strings.Contains(inside, want) {
			t.Errorf("a Form in a dialog does not carry %q: %s", want, inside)
		}
	}
	if strings.Index(inside, ">Cancel</button>") > strings.Index(inside, ">Save</button>") {
		t.Errorf("Cancel must come before the submit: %s", inside)
	}
	if outside := render(context.Background()); strings.Contains(outside, "Cancel") || strings.Contains(outside, "sticky") {
		t.Errorf("a Form outside a dialog drew a Cancel or pinned its row: %s", outside)
	}
}

// The dialog hands its body the knowledge that it is inside one: a Form drawn in the slot gets its Cancel.
func TestPopover_dialogBodyKnowsItIsInsideADialog(t *testing.T) {
	var buf bytes.Buffer
	n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentPopover),
		Props: map[string]string{"label": "Edit", "trigger": "secondary", "align": "end", "density": "regular"},
		Children: []ir.UINode{{Kind: ir.NodeComponent, Type: string(domain.ComponentForm), Props: map[string]string{"submit": "Save", "action": "/x"},
			Children: []ir.UINode{{Kind: ir.NodeStatic, Type: string(domain.StaticMessage), Props: map[string]string{"text": "Inside."}}}}}}
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); strings.Count(got, "data-popover-close") != 2 {
		t.Errorf("want the close mark and the form's Cancel (2 hooks), got %d: %s", strings.Count(got, "data-popover-close"), got)
	}
}

// `trigger: pill` is the trigger's words in a small outlined pill with a chevron after them -- the shared header's "Switch project" -- and
// not a button's height: it is the quieter of the two outlined looks.
func TestPopover_pillTriggerIsWordsAndAChevronInASmallOutline(t *testing.T) {
	got := renderPopover(t, context.Background(), "Switch project", "Switch project", domain.PopoverTriggerPill, domain.PopoverAlignStart, domain.PopoverDensityCompact, "BODY")
	summary := got[strings.Index(got, "<summary"):strings.Index(got, "</summary>")]
	for _, want := range []string{"Switch project", "<svg", "rounded-full", "border", "inline-flex"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the pill trigger lacks %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "h-9") {
		t.Errorf("a pill is not a button's height:\n%s", summary)
	}
}
