package aiassist

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// TestComposableSurfaceRendersEveryFieldType is the behavioural half of the prompt's honesty, and it lives
// here rather than in internal/conformance because it has to *call* composableSurface rather than read
// prompt.go as text.
//
// That distinction is not fussiness. The first version of this check was textual and sat in conformance,
// where its condition ("the prompt mentions FieldTypeLabels, or contains this label") could never fail --
// prompt.go names FieldTypeLabels in a comment. Mutation showed it: replacing the whole generated sentence
// with "text, number" left it green. A gate that reads a file containing its own trigger word is a gate
// with a hole exactly where it looks.
//
// What this asserts is the property that makes the generation worth having: a field type added to
// domain.KnownFieldTypes reaches the model with nobody editing prose.
func TestComposableSurfaceRendersEveryFieldType(t *testing.T) {
	surface := composableSurface()
	labels := domain.FieldTypeLabels()
	if len(labels) == 0 {
		t.Fatal("domain.FieldTypeLabels is empty -- this test would pass by asserting nothing")
	}
	for _, label := range labels {
		if !strings.Contains(surface, label) {
			t.Errorf("the grounding prompt does not name field type %q -- it is in domain.KnownFieldTypes, so the model is being told a smaller vocabulary than the runtime accepts", label)
		}
	}
	// Order is asserted too, because 007 §4.6 states determinism as a MUST and a map range would reword
	// the prompt between processes -- which makes two runs of the same request incomparable.
	if !strings.Contains(surface, strings.Join(labels, ", ")) {
		t.Errorf("the field types are not rendered in domain.FieldTypeLabels' own order -- a prompt that reorders between builds cannot be compared run to run")
	}
}
