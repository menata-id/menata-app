package rendering

import (
	"bytes"
	"context"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

func renderEmptyState(t *testing.T, ctx context.Context, text string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := emptyState(text).Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// The Approval Inbox's `inboxEmpty` (five calls) and Groups' hand-written copy drew this exact markup. The expected
// strings below are what `inboxEmpty` rendered at e3de0e6, captured before it was deleted, not written from the new
// template: one under the default Theme, one under a Theme moving `radius.surface`, with a message that needs escaping.
func TestEmptyState_reproducesThePreMigrationMarkup(t *testing.T) {
	const wantDefault = `<div class="rounded-lg border border-dashed border-border-control bg-bg-raised p-8 text-center text-sm text-ink-secondary">Nothing pending your approval.</div>`
	if got := renderEmptyState(t, context.Background(), "Nothing pending your approval."); got != wantDefault {
		t.Errorf("default theme:\n got %s\nwant %s", got, wantDefault)
	}

	moved := domain.DefaultTheme()
	moved.Radius[domain.RadiusSurface] = domain.RadiusStepFull
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "m", Theme: moved}, "m", false)
	const wantMoved = `<div class="rounded-full border border-dashed border-border-control bg-bg-raised p-8 text-center text-sm text-ink-secondary">a &lt;b&gt; &amp; &#34;c&#34;</div>`
	if got := renderEmptyState(t, ctx, `a <b> & "c"`); got != wantMoved {
		t.Errorf("moved radius, escaped text:\n got %s\nwant %s", got, wantMoved)
	}
}

// R9: a Workspace's theme moves every part of an empty state -- corner, edge, fill, size and ink -- and the default
// cannot show a collision. Colour roles are properties, so the check is made on what each class computes to.
func TestEmptyState_followsTheTheme(t *testing.T) {
	th := domain.DefaultTheme()
	th.Radius[domain.RadiusSurface] = domain.RadiusStepFull
	th.Border[domain.BorderControl] = domain.BorderFaint
	th.Background[domain.BackgroundRaised] = "#fffdf5"
	th.Text[domain.RoleBody] = domain.TextMedium
	th.Ink[domain.InkSecondary] = domain.InkShadeDarkest
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Slug: "m", Theme: th}, "m", false)
	got := resolveColourRoles(t, ctx, renderEmptyState(t, ctx, "x"))
	want := `<div class="rounded-full border border-dashed border-slate-100 bg-[#fffdf5] p-8 text-center text-base text-slate-900">x</div>`
	if got != want {
		t.Errorf("a declared theme must move the empty state:\n got %s\nwant %s", got, want)
	}
}

// A declared page reaches the Component through `uiNode`. Without this, an arm missing from the walker draws nothing and
// every test above still passes, because they call `emptyState` directly (the 2.3c mutation run found it).
func TestEmptyState_isDrawnWhenAPageDeclaresIt(t *testing.T) {
	var buf bytes.Buffer
	n := ir.UINode{Kind: ir.NodeComponent, Type: string(domain.ComponentEmptyState), Props: map[string]string{"text": "Nothing here."}}
	if err := uiNode(n).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	const want = `<div class="rounded-lg border border-dashed border-border-control bg-bg-raised p-8 text-center text-sm text-ink-secondary">Nothing here.</div>`
	if got := buf.String(); got != want {
		t.Errorf("a declared EmptyState:\n got %s\nwant %s", got, want)
	}
}
