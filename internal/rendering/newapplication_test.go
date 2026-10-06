package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/domain"
)

// TestNewApplicationReviewPage_draftBadgeKeepsItsContentWidth pins the one site of the coloured-chip slice
// whose migration was structural. The hand-written pill carried `w-fit` because its parent is a flex column,
// where a child stretches to full width; `statusBadge` is a Component and takes no arbitrary classes
// (007 §12.3), so the badge sits in a one-child row, which sizes it to its content. The dev data has no draft,
// so the screen cannot be diffed and this is the only check.
func TestNewApplicationReviewPage_draftBadgeKeepsItsContentWidth(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Test Workspace", false)
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication, Application: &aiassist.GeneratedApplication{Name: "Acme"}}
	var buf bytes.Buffer
	if err := NewApplicationReviewPage(change, aiassist.Plan{}, "s1", "Acme", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	i := strings.Index(html, ">Draft<")
	if i < 0 {
		t.Fatal("the review page did not draw its Draft badge")
	}
	before := html[:i]
	row := strings.LastIndex(before, `<div class="flex`)
	if row < 0 || strings.Contains(before[row:], "</div>") || strings.Contains(before[row:], "flex-col") {
		t.Errorf("the Draft badge is not the child of a flex row, so a flex-column parent would stretch it; context: %q", before[max(0, len(before)-200):])
	}
	if !strings.Contains(html[:i+10], "bg-amber-50 text-amber-800") {
		t.Error("the Draft badge does not draw the warn palette")
	}
}
