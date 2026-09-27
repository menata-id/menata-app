package aiassist

import (
	"sort"
	"testing"

	"menata.app/internal/domain"
)

// TestGeneratedApplicationSchema_iconIsAnEnumOfDeclarableIcons is the regression test for a real
// conversation (2026-09-27) that invented "Palette", "Sparkles" then "FileText" across three
// straight turns -- icon's own schema field described the set as closed in prose ("One of the
// known icon names named in the system prompt") without the prompt ever naming them, so Gemini had
// nothing to go on but guessing. An Enum is a structural guarantee instead: the API is
// contractually unable to return a value outside it.
func TestGeneratedApplicationSchema_iconIsAnEnumOfDeclarableIcons(t *testing.T) {
	icon := generatedApplicationSchema.Properties["icon"]
	if len(icon.Enum) == 0 {
		t.Fatal("icon's schema has no Enum -- Gemini can invent any string again")
	}

	want := make([]string, 0, len(domain.DeclarableIcons))
	for name := range domain.DeclarableIcons {
		want = append(want, name)
	}
	sort.Strings(want)
	got := append([]string(nil), icon.Enum...)
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("icon.Enum = %v, want exactly domain.DeclarableIcons = %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("icon.Enum = %v, want exactly domain.DeclarableIcons = %v", got, want)
		}
	}

	// The specific hallucinations a real conversation produced must never validate.
	for _, invented := range []string{"Palette", "Sparkles", "FileText", "palette", "sparkles"} {
		if domain.KnownIcons[invented] {
			t.Errorf("%q is in domain.KnownIcons -- the invented names this test guards against must stay invalid", invented)
		}
	}
}
