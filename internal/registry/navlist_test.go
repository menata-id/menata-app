package registry

import (
	"strings"
	"testing"
)

func TestValidateNavList(t *testing.T) {
	row := func(group, label, desc, href, state string) string {
		return strings.Join([]string{group, label, desc, href, state}, "\t")
	}
	ok := row("G", "A", "", "/a", "") + "\n" + row("G", "B", "", "/b", "current") + "\n" + row("H", "P", "", "", "placeholder")
	for name, tc := range map[string]struct {
		in   map[string]string
		want string // "" = valid
	}{
		"valid":                  {map[string]string{"label": "L", "variant": "sidebar", "rows": ok}, ""},
		"a blank label":          {map[string]string{"label": " ", "variant": "page", "rows": ok}, "label is blank"},
		"an unknown variant":     {map[string]string{"label": "L", "variant": "wide", "rows": ok}, "not sidebar or page"},
		"a short row":            {map[string]string{"label": "L", "variant": "page", "rows": "G\tA"}, "five cells"},
		"two current rows":       {map[string]string{"label": "L", "variant": "page", "rows": row("G", "A", "", "/a", "current") + "\n" + row("G", "B", "", "/b", "current")}, "more than one"},
		"a link-less live row":   {map[string]string{"label": "L", "variant": "page", "rows": row("G", "A", "", "", "")}, "no href"},
		"a placeholder with one": {map[string]string{"label": "L", "variant": "page", "rows": row("G", "A", "", "/a", "placeholder")}, "no href"},
		"an unknown state":       {map[string]string{"label": "L", "variant": "page", "rows": row("G", "A", "", "/a", "hot")}, "not current or placeholder"},
		"an undeclared input":    {map[string]string{"label": "L", "variant": "page", "rows": ok, "class": "x"}, "class"},
	} {
		issues := validateNavList(tc.in)
		switch {
		case tc.want == "" && len(issues) > 0:
			t.Errorf("%s: unexpected %v", name, issues)
		case tc.want != "" && !strings.Contains(strings.Join(issues, "; "), tc.want):
			t.Errorf("%s: %v, want one containing %q", name, issues, tc.want)
		}
	}
}
