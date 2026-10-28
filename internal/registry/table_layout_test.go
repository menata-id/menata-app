package registry

import (
	"strings"
	"testing"
)

// A Table's density and row tone are closed sets: a word outside them is refused with the words the page may use.
func TestValidateTableLayoutProps(t *testing.T) {
	base := map[string]string{"label": "Jobs", "headers": "Title", "aligns": "start"}
	with := func(k, v string) map[string]string {
		in := map[string]string{}
		for a, b := range base {
			in[a] = b
		}
		in[k] = v
		return in
	}
	for _, v := range []string{"compact", "regular", "roomy"} {
		if issues := validateTable(with("density", v)); len(issues) != 0 {
			t.Errorf("density %q was refused: %v", v, issues)
		}
	}
	if issues := strings.Join(validateTable(with("density", "tight")), "\n"); !strings.Contains(issues, "compact, regular or roomy") {
		t.Errorf("a density outside the set: %q", issues)
	}
	for _, v := range []string{"date:tone", "status:tone"} {
		if issues := validateTable(with("row_tone", v)); len(issues) != 0 {
			t.Errorf("row_tone %q was refused: %v", v, issues)
		}
	}
	if issues := strings.Join(validateTable(with("row_tone", "money:tone")), "\n"); !strings.Contains(issues, "date:tone or status:tone") {
		t.Errorf("a row_tone outside the set: %q", issues)
	}
}
