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

func TestValidateCollection_blockIsBooleanAndNotAlsoOrdered(t *testing.T) {
	with := func(k, v string) map[string]string {
		m := map[string]string{"gap": "tight", "block": "true"}
		m[k] = v
		return m
	}
	if issues := validateCollection(map[string]string{"gap": "tight", "block": "true"}); len(issues) != 0 {
		t.Errorf("a block list: %v", issues)
	}
	if issues := validateCollection(with("divided", "true")); len(issues) != 0 {
		t.Errorf("block with divided: %v", issues)
	}
	for name, in := range map[string]map[string]string{"yes": with("block", "yes"), "ordered": with("ordered", "true")} {
		if issues := validateCollection(in); len(issues) == 0 {
			t.Errorf("block with %s was accepted", name)
		}
	}
}

func TestValidateSearchBox(t *testing.T) {
	ok := map[string]string{"label": "Search members", "query": "q"}
	if issues := validateSearchBox(ok); len(issues) != 0 {
		t.Errorf("a valid box: %v", issues)
	}
	for name, in := range map[string]map[string]string{
		"a blank label":    {"label": " ", "query": "q"},
		"a reserved name":  {"label": "S", "query": "week"},
		"not a plain word": {"label": "S", "query": "Q?x"},
		"an undeclared":    {"label": "S", "query": "q", "class": "x"},
	} {
		if issues := validateSearchBox(in); len(issues) == 0 {
			t.Errorf("%s was accepted", name)
		}
	}
}

// `inline` lays a Collection's items in one wrapping run (a row of chips): a boolean, and not also ordered, divided or block.
func TestValidateCollection_inlineIsBooleanAndExcludesTheOtherShapes(t *testing.T) {
	if issues := validateCollection(map[string]string{"gap": "tight", "inline": "true"}); len(issues) != 0 {
		t.Errorf("an inline list: %v", issues)
	}
	if issues := validateCollection(map[string]string{"gap": "tight", "inline": "false", "divided": "true"}); len(issues) != 0 {
		t.Errorf("inline: false beside divided: %v", issues)
	}
	for name, in := range map[string]map[string]string{
		"yes":     {"gap": "tight", "inline": "yes"},
		"ordered": {"gap": "tight", "inline": "true", "ordered": "true"},
		"divided": {"gap": "tight", "inline": "true", "divided": "true"},
		"block":   {"gap": "tight", "inline": "true", "block": "true"},
	} {
		if issues := validateCollection(in); len(issues) == 0 {
			t.Errorf("inline with %s was accepted", name)
		}
	}
}
