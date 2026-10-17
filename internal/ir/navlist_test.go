package ir

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func navResolver(pageID string) Resolver {
	return Resolver{PageID: pageID, Navigation: func(string) (NavSection, error) {
		return NavSection{Label: "Workspace settings", Home: "/workspace-settings", Entries: []NavEntry{
			{ID: "nav_a", Group: "One", Label: "Alpha", Description: "first", Href: "/a"},
			{ID: "nav_b", Group: "One", Label: "Beta", Description: "second", Href: "/b"},
			{ID: "nav_p", Group: "Two", Label: "Pending", Description: "later", Placeholder: true},
			{ID: "nav_s", Group: "Two", Label: "Side", Href: "/s#x", SidebarOnly: true},
		}}, nil
	}}
}

func navNode(props map[string]string) domain.PageNode {
	return formPage(domain.PageNode{Kind: "component", Type: string(domain.ComponentNavList), Props: props})
}

func TestLower_aNavListIsTheNavigationsRowsWithThePagesOwnMarkedCurrent(t *testing.T) {
	got, err := Lower(navNode(map[string]string{"section": "workspace_settings", "variant": "sidebar"}), navResolver("nav_b"))
	if err != nil {
		t.Fatal(err)
	}
	p := got.Children[0].Props
	rows := strings.Split(p["rows"], "\n")
	want := []string{"One\tAlpha\tfirst\t/a\t", "One\tBeta\tsecond\t/b\tcurrent", "Two\tPending\tlater\t\tplaceholder", "Two\tSide\t\t/s#x\t"}
	if strings.Join(rows, "|") != strings.Join(want, "|") {
		t.Errorf("rows = %q, want %q", rows, want)
	}
	if p["label"] != "Workspace settings" || p["home"] != "/workspace-settings" || p["variant"] != "sidebar" || p["section"] != "" {
		t.Errorf("props = %v", p)
	}
	got, err = Lower(navNode(map[string]string{"section": "workspace_settings", "variant": "page"}), navResolver("nav_zzz"))
	if err != nil {
		t.Fatal(err)
	}
	if rows := got.Children[0].Props["rows"]; strings.Contains(rows, "Side") || strings.Contains(rows, "current") || got.Children[0].Props["home"] != "" {
		t.Errorf("the page variant keeps a sidebar-only row, a current mark or the way back: %q", rows)
	}
}

func TestLower_aNavListWritesNothingItDerives(t *testing.T) {
	for name, props := range map[string]map[string]string{
		"a typed label":      {"section": "workspace_settings", "variant": "page", "label": "Mine"},
		"typed rows":         {"section": "workspace_settings", "variant": "page", "rows": "x"},
		"a typed home":       {"section": "workspace_settings", "variant": "sidebar", "home": "/x"},
		"an unknown section": {"section": "nothing", "variant": "page"},
		"an unknown variant": {"section": "workspace_settings", "variant": "wide"},
		"no section":         {"variant": "page"},
	} {
		if _, err := Lower(navNode(props), navResolver("")); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	withChild := navNode(map[string]string{"section": "workspace_settings", "variant": "page"})
	withChild.Children[0].Children = []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}
	if _, err := Lower(withChild, navResolver("")); err == nil {
		t.Error("a NavList holding children was accepted")
	}
}

func searchNode(props map[string]string) domain.PageNode {
	return formPage(domain.PageNode{Kind: "component", Type: string(domain.ComponentSearchBox), Props: props})
}

func TestLower_aSearchBoxCarriesTheWordsTheRequestAlreadyHad(t *testing.T) {
	res := Resolver{Query: func(name string) string {
		if name == "q" {
			return "rina"
		}
		return "other"
	}}
	got, err := Lower(searchNode(map[string]string{"label": "Search", "query": "q", "placeholder": "Find…"}), res)
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Children[0].Props; p["value"] != "rina" || p["query"] != "q" || p["label"] != "Search" {
		t.Errorf("props = %v", p)
	}
	got, err = Lower(searchNode(map[string]string{"label": "Search", "query": "q"}), Resolver{})
	if err != nil || got.Children[0].Props["value"] != "" {
		t.Errorf("no request: err=%v props=%v, want an empty box", err, got.Children[0].Props)
	}
}

func TestLower_aSearchBoxWritesNoValueAndUsesNoQueryTheRuntimeOwns(t *testing.T) {
	for name, props := range map[string]map[string]string{
		"a written value": {"label": "S", "query": "q", "value": "x"},
		"a reserved name": {"label": "S", "query": "application"},
		"the record id":   {"label": "S", "query": "id"},
	} {
		if _, err := Lower(searchNode(props), Resolver{}); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestLower_aLayoutMayCarryAnAnchorThatIsAPlainWord(t *testing.T) {
	for anchor, ok := range map[string]bool{"danger-zone": true, "installation": true, "x1": true, "Danger": false, "a b": false, "1a": false, "a\"b": false, "<b>": false} {
		n := formPage(domain.PageNode{Kind: "layout", Type: "section", Props: map[string]string{"gap": "default", "anchor": anchor},
			Children: []domain.PageNode{{Kind: "static", Type: "paragraph", Props: map[string]string{"text": "x"}}}})
		got, err := Lower(n, Resolver{})
		if err != nil {
			t.Fatal(err)
		}
		issues := Validate(got)
		if (len(issues) == 0) != ok {
			t.Errorf("anchor %q: valid=%v, want %v (%v)", anchor, len(issues) == 0, ok, issues)
		}
	}
}

func TestLower_aSearchBoxThatSendsItsWordsElsewhereNamesItsDestinationAndCarriesNoOldWords(t *testing.T) {
	res := Resolver{
		Query: func(string) string { return "old words" },
		Route: func(id string) (string, string, bool) {
			return "/new-application", "New application", id == "nav_new_application"
		},
	}
	n := searchNode(map[string]string{"label": "Describe", "query": "idea", "submit": "Start"})
	n.Children[0].To = "nav_new_application"
	got, err := Lower(n, res)
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Children[0].Props; p["action"] != "/new-application" || p["value"] != "" || p["submit"] != "Start" {
		t.Errorf("props = %v, want the destination's route, no carried words and the submit label", p)
	}
	for name, mutate := range map[string]func(*domain.PageNode){
		"a typed action":   func(n *domain.PageNode) { n.Props["action"] = "/somewhere" },
		"an unknown route": func(n *domain.PageNode) { n.To = "nav_nobody" },
	} {
		bad := searchNode(map[string]string{"label": "Describe", "query": "idea"})
		bad.Children[0].To = "nav_new_application"
		mutate(&bad.Children[0])
		if _, err := Lower(bad, res); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
