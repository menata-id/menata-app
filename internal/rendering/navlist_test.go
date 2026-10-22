package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
)

func renderNavList(t *testing.T, variant string) string {
	t.Helper()
	rows := strings.Join([]string{
		"People\tMembers\tEveryone here\t/m\t",
		"People\tGroups\tWho holds what\t/g\tcurrent",
		"Security\tAudit log\tPermanent record\t\tplaceholder",
	}, "\n")
	var buf bytes.Buffer
	if err := navList("Workspace settings", variant, "/workspace-settings", rows).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestNavList_sidebarIsHiddenOnAPhoneLeadsBackAndMarksOneRow(t *testing.T) {
	html := renderNavList(t, "sidebar")
	if !strings.Contains(html, `<nav aria-label="Workspace settings" class="`) || !strings.Contains(html, "hidden w-55") || !strings.Contains(html, "sm:flex") {
		t.Errorf("the sidebar is not hidden below sm: %s", html)
	}
	if !strings.Contains(html, `href="/workspace-settings"`) || !strings.Contains(html, "← Workspace settings") {
		t.Error("the sidebar has no way back to the hub")
	}
	if n := strings.Count(html, `aria-current="page"`); n != 1 || !strings.Contains(html, `href="/g" aria-current="page"`) {
		t.Errorf("%d current rows, want exactly the Groups one", n)
	}
	if strings.Contains(html, "Everyone here") {
		t.Error("the sidebar draws a row's description")
	}
	if strings.Contains(html, `href=""`) || !strings.Contains(html, "Not built yet") || !strings.Contains(html, "Audit log") {
		t.Error("a screen that is not built is not drawn as such, or is a link to nowhere")
	}
	for _, heading := range []string{"People", "Security"} {
		if !strings.Contains(html, heading) {
			t.Errorf("heading %q missing", heading)
		}
	}
}

func TestNavList_pageVariantShowsDescriptionsAndIsAlwaysDrawn(t *testing.T) {
	html := renderNavList(t, "page")
	if strings.Contains(html, "hidden w-55") || strings.Contains(html, "← Workspace settings") {
		t.Errorf("the page variant is hidden or leads back to itself: %s", html)
	}
	if !strings.Contains(html, "Everyone here") || !strings.Contains(html, "Who holds what") {
		t.Error("the page variant omits the descriptions")
	}
}

func TestMultiSelectInput_drawsOneCheckboxPerOptionUnderOneNameTickedFromTheValue(t *testing.T) {
	var buf bytes.Buffer
	if err := formInput("ctl", "member", "multiselect", "u1=ann@example.com\nu2=bob@example.com\nplain", "u2\nplain", "", false, false, false).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if n := strings.Count(html, `type="checkbox" name="member"`); n != 3 {
		t.Errorf("%d checkboxes, want one per option: %s", n, html)
	}
	if !strings.Contains(html, `value="u2" checked`) || strings.Contains(html, `value="u1" checked`) || !strings.Contains(html, `value="plain" checked`) {
		t.Errorf("ticked state wrong: %s", html)
	}
	if !strings.Contains(html, ">ann@example.com<") || !strings.Contains(html, ">plain<") {
		t.Errorf("an option's label is not drawn (value=Label, or the value when there is no =): %s", html)
	}
}

func TestCollection_blockDrawsEachItemAsOneColumnAndAPlainListStillWrapsInline(t *testing.T) {
	item := []templ.Component{templ.Raw("<p>x</p>")}
	var plain, block bytes.Buffer
	if err := collection(domain.GapTight, "", false, false, false, false, "", Truncation{}, "", "", item).Render(context.Background(), &plain); err != nil {
		t.Fatal(err)
	}
	if err := collection(domain.GapTight, "", false, false, true, false, "", Truncation{}, "", "", item).Render(context.Background(), &block); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.String(), `<li class="flex flex-wrap items-center`) || strings.Contains(plain.String(), "flex-col\"><p>") {
		t.Errorf("a plain list's item changed: %s", plain.String())
	}
	if !strings.Contains(block.String(), `<li class="flex flex-col`) || strings.Contains(block.String(), "flex-wrap") {
		t.Errorf("a block list's item is not one column: %s", block.String())
	}
}

func TestSearchField_isANamedSearchFormThatSubmitsByGetAndKeepsTheWords(t *testing.T) {
	var buf bytes.Buffer
	if err := searchField("Search members", "q", "Find…", `ri"na`, "", "", "").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`<form method="get" role="search"`, `<label for="search-q" class="sr-only">Search members</label>`, `type="search" name="q"`, `placeholder="Find…"`, `value="ri&#34;na"`} {
		if !strings.Contains(html, want) {
			t.Errorf("search field does not contain %q: %s", want, html)
		}
	}
}

func TestUINode_aLayoutWithAnAnchorIsWrappedInOneElementHoldingTheId(t *testing.T) {
	node := func(props map[string]string) ir.UINode {
		return ir.UINode{Kind: ir.NodeLayout, Type: "stack", Props: props, Children: []ir.UINode{{Kind: ir.NodeStatic, Type: "paragraph", Props: map[string]string{"text": "hi"}}}}
	}
	var with, without bytes.Buffer
	if err := uiNode(node(map[string]string{"gap": "default", "anchor": "danger-zone"})).Render(context.Background(), &with); err != nil {
		t.Fatal(err)
	}
	if err := uiNode(node(map[string]string{"gap": "default"})).Render(context.Background(), &without); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with.String(), `<div id="danger-zone">`) || strings.Contains(without.String(), "id=") {
		t.Errorf("anchored: %s\nplain: %s", with.String(), without.String())
	}
}

func TestSearchField_goingElsewhereDrawsItsActionAndASubmitButton(t *testing.T) {
	var buf bytes.Buffer
	if err := searchField("Describe", "idea", "", "", "/new-application", "Start →", "").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, `action="/new-application"`) || !strings.Contains(html, "Start →") || !strings.Contains(html, `name="idea"`) {
		t.Errorf("a search box that goes elsewhere: %s", html)
	}
	var plain bytes.Buffer
	if err := searchField("Search", "q", "", "", "", "", "").Render(context.Background(), &plain); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "action=") || strings.Contains(plain.String(), "<button") {
		t.Errorf("a plain search box draws an action or a button: %s", plain.String())
	}
}
