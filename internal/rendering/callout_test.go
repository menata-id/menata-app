package rendering

import (
	"bytes"
	"context"
	"testing"

	"menata.app/internal/domain"
)

func renderCallout(t *testing.T, ctx context.Context, text string, tone domain.BadgeTone) string {
	t.Helper()
	var buf bytes.Buffer
	if err := callout(text, tone).Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// The six hand-written sites rendered these exact classes; a migrated site may differ only by the role the
// contract now decides (bad: alert, everything else: status). Expected strings are written by hand.
func TestCallout_reproducesThePreMigrationMarkupPlusItsRole(t *testing.T) {
	for tone, want := range map[domain.BadgeTone]string{
		domain.ToneBad:  `<p role="alert" class="m-0 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">Wrong password.</p>`,
		domain.ToneGood: `<p role="status" class="m-0 rounded-md bg-emerald-50 px-3 py-2 text-sm text-emerald-700">Wrong password.</p>`,
		domain.ToneWarn: `<p role="status" class="m-0 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">Wrong password.</p>`,
		domain.ToneInfo: `<p role="status" class="m-0 rounded-md bg-blue-50 px-3 py-2 text-sm text-blue-700">Wrong password.</p>`,
	} {
		if got := renderCallout(t, context.Background(), "Wrong password.", tone); got != want {
			t.Errorf("%s:\n got %s\nwant %s", tone, got, want)
		}
	}
}

// A Workspace's theme moves a callout: its tone pair, its corner and its size are Theme reads, not literals.
func TestCallout_followsTheTheme(t *testing.T) {
	th := domain.Theme{
		Tone:   map[domain.BadgeTone]domain.TonePalette{domain.ToneBad: domain.PaletteAmber},
		Radius: map[domain.RadiusRole]domain.RadiusStep{domain.RadiusControl: domain.RadiusStepLarge},
		Text:   map[domain.TextRole]domain.TextScale{domain.RoleBody: domain.TextMedium},
	}
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Theme: th}, "WS", false)
	got := renderCallout(t, ctx, "x", domain.ToneBad)
	want := `<p role="alert" class="m-0 rounded-lg bg-amber-50 px-3 py-2 text-base text-amber-800">x</p>`
	if got != want {
		t.Errorf("a declared theme must move the callout:\n got %s\nwant %s", got, want)
	}
}
