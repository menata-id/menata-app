package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// inferenceNavFixture is a ctx with a Workspace on it. nav_inference itself is not declared here on
// purpose: it lives in domain.RuntimeScreens, which declaredNavigation appends, so a page reading its
// own title by id must resolve without any Application declaring it. If that ever stops being true,
// this fixture fails rather than the live screen.
func inferenceNavFixture() context.Context {
	return WithCurrentWorkspace(context.Background(), domain.Workspace{}, "Test Workspace", false)
}

// TestInferencePage_showsTheReviewDefect is about *rendering*, not classification -- internal/action
// already proves ExplainCast labels the /review 404's shape input-unavailable. This proves the page a
// person actually opens says so.
//
// Both halves could hide it independently: the classification could be right while the cell rendered
// blank, or the page could be fine while the four statuses collapsed into one.
func TestInferencePage_showsTheReviewDefect(t *testing.T) {
	view := InferenceView{
		Defects: 1,
		Engines: []InferenceEngine{{
			Engine:  "document_approval",
			Defects: 1,
			Roles: []InferenceRole{{
				Role: "step", MachineID: "mch_langkah", Required: true, Defects: 1,
				Rows: []InferenceRow{
					{Derivation: "decision", Value: "fld_putusan", From: "transitions[action=decide].field", Status: "resolved", Tone: PillGood},
					{
						Derivation: "parent", Value: "", Status: "input unavailable", Tone: PillWarn, IsDefect: true,
						From: "relation to the document Machine -- but no document Machine was supplied",
					},
				},
			}},
		}},
	}

	html := renderInference(t, view)

	if !strings.Contains(html, "input unavailable") {
		t.Error("the page does not render \"input unavailable\" -- the status exists in the data and never " +
			"reaches the person looking at the screen, which is the whole failure mode")
	}
	if !strings.Contains(html, "no document Machine was supplied") {
		t.Error("the page does not name the missing input, so a reader is told something is wrong and not what")
	}
	if !strings.Contains(html, "needs attention") {
		t.Error("the summary does not say the page needs attention, so a reader must find the row themselves")
	}
	// The empty value must be *visibly* empty. Two separate ways this can go wrong, both asserted,
	// because an empty derivation that renders as an ordinary-looking cell is exactly what made the
	// /review 404 invisible for a day: the page must not invent a plausible id, and it must not render
	// nothing at all -- a blank cell reads as "no problem here" to every reader.
	if strings.Contains(html, "fld_document") || strings.Contains(html, "fld_parent") {
		t.Error("the page invented a plausible-looking Field id for a derivation that resolved to nothing")
	}
	if !strings.Contains(html, "&mdash;") && !strings.Contains(html, "\u2014") {
		t.Error("an empty resolved value renders as a blank cell -- it needs a visible marker, or the row " +
			"reads as ordinary next to the ones that did resolve")
	}
}

// TestInferencePage_doesNotShoutAboutCorrectEmpties is the half more easily lost. 52 of 65 rows are
// legitimately empty; a page that flagged them would be abandoned in a day, and two earlier attempts
// at triaging these numbers died exactly there.
func TestInferencePage_doesNotShoutAboutCorrectEmpties(t *testing.T) {
	view := InferenceView{
		Defects: 0,
		Engines: []InferenceEngine{{
			Engine: "document_approval",
			Roles: []InferenceRole{{
				Role: "document", MachineID: "mch_surat", Required: true,
				Rows: []InferenceRow{
					{Derivation: "document_status", Value: "fld_status", From: "transitions[].field", Status: "resolved", Tone: PillGood},
					{Derivation: "decision", Value: "", From: "transitions[action=decide].field", Status: "not applicable", Tone: PillMuted},
				},
			}},
		}},
	}

	html := renderInference(t, view)

	if !strings.Contains(html, "nothing to act on") {
		t.Error("a page with no defects does not say so, so a reader has to establish it by scanning rows")
	}
	if strings.Contains(html, "needs attention") {
		t.Error("a page with no defects claims it needs attention")
	}
	// Present, not omitted: silence about a correct empty is what made Stage E1's broken ones ordinary.
	if !strings.Contains(html, "not applicable") {
		t.Error("the not-applicable row was left off the page entirely -- it must be stated as an answer")
	}
}

// TestInferencePage_statesTheVocabulary: four statuses are only useful if the page explains them. A
// reader who has to look up "undeclared" in the Go source is not being given a diagnostic.
func TestInferencePage_statesTheVocabulary(t *testing.T) {
	html := renderInference(t, InferenceView{})
	for _, want := range []string{"undeclared", "input unavailable", "not applicable"} {
		if !strings.Contains(html, want) {
			t.Errorf("the page never explains what %q means", want)
		}
	}
	// And an empty Workspace says so rather than rendering an empty frame.
	if !strings.Contains(html, "No workflow engine is installed") {
		t.Error("a Workspace with no engine installed renders no explanation of why the page is empty")
	}
}

func renderInference(t *testing.T, v InferenceView) string {
	t.Helper()
	var buf bytes.Buffer
	if err := InferencePage(v, "Test Workspace", Viewer{}, "/switch-workspace").Render(inferenceNavFixture(), &buf); err != nil {
		t.Fatalf("render InferencePage: %v", err)
	}
	return buf.String()
}
