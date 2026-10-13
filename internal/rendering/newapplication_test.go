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
	if err := NewApplicationReviewPage(change, aiassist.Plan{}, GeneratedFilesView{}, "s1", "Acme", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := resolveColourRoles(t, ctx, buf.String())
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

// TestUpdateReview_aRemovalThatNeedsConfirmationCarriesOneCheckboxForThePublishForm: the publish handler answers
// 422 for a removal whose key is not posted as `confirm_remove`, so without this box a Field removal could not be
// published from the UI. The box sits in the plan and belongs to the form in the aside, by id; a removal that needs
// no confirmation, and an add, draw none.
func TestUpdateReview_aRemovalThatNeedsConfirmationCarriesOneCheckboxForThePublishForm(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Test Workspace", false)
	change := aiassist.GeneratedChange{Kind: aiassist.KindUpdateApplication, TargetAppID: "app_x"}
	key := aiassist.RemovalKey("mch_a", "fld_b")
	plan := aiassist.Plan{Items: []aiassist.PlanItem{
		{Op: "add", What: "Field Added"},
		{Op: "remove", What: "Field Gone", Confirm: key},
		{Op: "remove", What: "Option Gone"},
	}}
	var buf bytes.Buffer
	if err := NewApplicationReviewPage(change, plan, GeneratedFilesView{}, "s1", "Acme", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if got := strings.Count(html, `name="confirm_remove"`); got != 1 {
		t.Fatalf("%d confirm_remove checkboxes, want exactly 1 (only the removal that asks)", got)
	}
	if !strings.Contains(html, `value="`+key+`"`) {
		t.Errorf("the checkbox does not carry the plan item's confirmation key %q", key)
	}
	if !strings.Contains(html, `form="`+publishFormID+`"`) || !strings.Contains(html, `<form id="`+publishFormID+`"`) {
		t.Error("the checkbox is not bound to the publish form by id, so it would never be submitted")
	}
}

// TestGeneratedFilesPanel_showsTheWritersDiffOrItsRefusalAndNothingWhenThereIsNoPreview: the panel under a proposal
// is the YAML the writer would put on disk. Added lines are drawn as additions, a change the writer refused shows
// its reason instead of an empty panel, and a process with no preview (no metadata directory) draws no heading at all.
func TestGeneratedFilesPanel_showsTheWritersDiffOrItsRefusalAndNothingWhenThereIsNoPreview(t *testing.T) {
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Test Workspace", false)
	change := aiassist.GeneratedChange{Kind: aiassist.KindNewApplication}
	render := func(v GeneratedFilesView) string {
		var buf bytes.Buffer
		if err := NewApplicationReviewPage(change, aiassist.Plan{}, v, "s1", "Acme", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	files := render(GeneratedFilesView{Files: []FileDiffView{{Path: "acme/leave.yaml", Changed: true, Diff: "+id: mch_leave\n-id: old\n"}}})
	for _, want := range []string{"The files this would write", "acme/leave.yaml", "bg-green-50 px-3 text-green-800", "bg-red-50 px-3 text-red-800"} {
		if !strings.Contains(files, want) {
			t.Errorf("the panel is missing %q", want)
		}
	}

	refused := render(GeneratedFilesView{Reason: "machine mch_x is already installed <b>"})
	if !strings.Contains(refused, "could not be previewed") || !strings.Contains(refused, "mch_x is already installed &lt;b&gt;") {
		t.Errorf("a refused change must show its reason, escaped; got no such text")
	}

	if none := render(GeneratedFilesView{}); strings.Contains(none, "The files this would write") {
		t.Error("with no preview the page must not draw an empty files panel")
	}
}
