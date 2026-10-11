package installer

import (
	"strings"
	"testing"
)

func TestUnifiedDiff_marksOnlyWhatChangedAndKeepsContext(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	b := "1\n2\n3\n4\n5\nsix\n7\n8\n9\n10\n11\n12\n"
	got := UnifiedDiff(a, b)
	if !strings.Contains(got, "-6\n+six\n") || strings.Contains(got, "-1\n") || strings.Contains(got, "+1\n") {
		t.Errorf("diff = %q", got)
	}
	if strings.Contains(got, " 12\n") || strings.Contains(got, " 1\n") {
		t.Errorf("lines more than three away from a change are shown: %q", got)
	}
	if UnifiedDiff("x\n", "x\n") != " x\n" && UnifiedDiff("x\n", "x\n") != "" {
		t.Errorf("an unchanged file is not an empty diff")
	}
}
