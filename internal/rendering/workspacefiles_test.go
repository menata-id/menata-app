package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func renderWorkspaceFiles(t *testing.T, view WorkspaceFilesView) string {
	t.Helper()
	ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Tallyville", true)
	var buf bytes.Buffer
	if err := WorkspaceFilesPage(view, "Tallyville", Viewer{Initials: "AN"}, "").Render(ctx, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return buf.String()
}

// Pasted YAML is attacker-shaped text as far as the page is concerned: it comes back inside a textarea and a
// hidden input, and must not be able to close either.
func TestWorkspaceFilesPage_escapesPastedContentInTheEditorAndTheApplyForm(t *testing.T) {
	const evil = "name: \"x\"</textarea><script>alert(1)</script>"
	html := renderWorkspaceFiles(t, WorkspaceFilesView{
		Files: []string{"a.yaml"}, Path: "a.yaml", YAML: evil,
		Review: &FileReview{VerdictLabel: "Loads", Tone: domain.ToneGood, CanApply: true,
			Diffs: []FileDiffView{{Path: "a.yaml", Changed: true, Diff: "-old\n+<b>new</b>\n"}}},
	})
	if strings.Contains(html, "<script>alert(1)</script>") || strings.Contains(html, "<b>new</b>") {
		t.Error("pasted content or a diff line reached the page unescaped")
	}
	if !strings.Contains(html, "&lt;/textarea&gt;") {
		t.Error("the editor did not escape a closing textarea tag")
	}
}

// Apply is drawn only for a review that said CanApply; the page cannot offer a button the handler withheld.
func TestWorkspaceFilesPage_drawsApplyOnlyWhenTheReviewAllowsIt(t *testing.T) {
	review := func(can bool) string {
		return renderWorkspaceFiles(t, WorkspaceFilesView{Files: []string{"a.yaml"}, Path: "a.yaml", YAML: "id: a\n",
			Review: &FileReview{VerdictLabel: "Rejected", Tone: domain.ToneBad, CanApply: can}})
	}
	if !strings.Contains(review(true), `action="/workspace-settings/apply"`) {
		t.Error("CanApply did not draw the Apply form")
	}
	if strings.Contains(review(false), `action="/workspace-settings/apply"`) {
		t.Error("the Apply form was drawn for a review that did not allow it")
	}
}

// A verdict with no chosen file (a rejected path) still shows its reason: dropping it would leave the admin with
// a picker and no explanation.
func TestWorkspaceFilesPage_showsAReviewEvenWithNoFileChosen(t *testing.T) {
	html := renderWorkspaceFiles(t, WorkspaceFilesView{Files: []string{"a.yaml"},
		Review: &FileReview{VerdictLabel: "Rejected", Tone: domain.ToneBad, Reason: "\"x.yaml\" is not a file this workspace has"}})
	if !strings.Contains(html, "not a file this workspace has") {
		t.Error("the reason for a rejected path is not on the page")
	}
}
