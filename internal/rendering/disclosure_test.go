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

func renderDisclosure(t *testing.T, ctx context.Context, summary, body string) string {
	t.Helper()
	var buf bytes.Buffer
	inner := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := w.Write([]byte(body)); return err })
	if err := disclosure(summary).Render(templ.WithChildren(ctx, inner), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// Groups' "+ New group" and Members' "+ Invite member" drew this markup by hand. The expected string is what Groups
// rendered at b7819df, captured before the migration (the summary and the panel, not written from the new template; the body wrapper
// is the one addition, with the rule and padding the forms carried for themselves); the second case moves the
// surface corner and escapes the summary.
func TestDisclosure_reproducesThePreMigrationMarkup(t *testing.T) {
	const wantDefault = `<details class="rounded-lg border border-border-surface bg-bg-raised"><summary class="cursor-pointer list-none px-5 py-3 text-sm font-medium text-accent [&amp;::-webkit-details-marker]:hidden">+ New group</summary><div class="border-t border-border-surface px-5 py-4"><form></form></div></details>`
	if got := renderDisclosure(t, context.Background(), "+ New group", "<form></form>"); got != wantDefault {
		t.Errorf("default theme:\n got %s\nwant %s", got, wantDefault)
	}
	moved := domain.DefaultTheme()
	moved.Radius[domain.RadiusSurface] = domain.RadiusStepFull
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "m", Theme: moved}, "m", false)
	got := renderDisclosure(t, ctx, `a <b> & "c"`, "")
	if !strings.Contains(got, `<details class="rounded-full `) || !strings.Contains(got, `>a &lt;b&gt; &amp; &#34;c&#34;</summary>`) {
		t.Errorf("moved radius, escaped summary: %s", got)
	}
}

// R9: the panel's corner, edge, fill, summary size, weight and ink all follow the Workspace's theme.
func TestDisclosure_followsTheTheme(t *testing.T) {
	th := domain.DefaultTheme()
	th.Radius[domain.RadiusSurface] = domain.RadiusStepFull
	th.Border[domain.BorderSurface] = domain.BorderDefined
	th.Background[domain.BackgroundRaised] = "#fffdf5"
	th.Text[domain.RoleBody] = domain.TextMedium
	th.Weight[domain.WeightEmphasis] = domain.WeightStepNormal
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "m", Theme: th}, "m", false)
	got := resolveColourRoles(t, ctx, renderDisclosure(t, ctx, "x", ""))
	if n := strings.Count(got, "border-slate-300"); n != 2 {
		t.Errorf("the panel edge and the rule above the body should both follow border.surface, got %d: %s", n, got)
	}
	for _, want := range []string{"rounded-full", "border-slate-300", "bg-[#fffdf5]", "text-base", "font-normal"} {
		if !strings.Contains(got, want) {
			t.Errorf("a declared theme did not move the disclosure (%q missing): %s", want, got)
		}
	}
}

// A declared page reaches the Component through `uiNode`, and its children land inside the panel.
func TestDisclosure_isDrawnWhenAPageDeclaresItWithItsBody(t *testing.T) {
	var buf bytes.Buffer
	n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentDisclosure), Props: map[string]string{"summary": "More"},
		Children: []ir.UINode{{Kind: ir.NodeStatic, Type: string(domain.StaticMessage), Props: map[string]string{"text": "Inside."}}}}
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "<details") || !strings.Contains(got, ">More</summary>") || strings.Index(got, "Inside.") < strings.Index(got, "</summary>") || strings.Index(got, "Inside.") > strings.Index(got, "</details>") {
		t.Errorf("the declared disclosure is not drawn with its body inside: %s", got)
	}
}
