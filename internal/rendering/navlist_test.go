package rendering

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"menata.app/internal/domain"
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
	if err := formInput("ctl", "member", "multiselect", "u1=ann@example.com\nu2=bob@example.com\nplain", "u2\nplain", false).Render(context.Background(), &buf); err != nil {
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
	if err := collection(domain.GapTight, "", false, false, false, Truncation{}, "", "", item).Render(context.Background(), &plain); err != nil {
		t.Fatal(err)
	}
	if err := collection(domain.GapTight, "", false, false, true, Truncation{}, "", "", item).Render(context.Background(), &block); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.String(), `<li class="flex flex-wrap items-center`) || strings.Contains(plain.String(), "flex-col\"><p>") {
		t.Errorf("a plain list's item changed: %s", plain.String())
	}
	if !strings.Contains(block.String(), `<li class="flex flex-col`) || strings.Contains(block.String(), "flex-wrap") {
		t.Errorf("a block list's item is not one column: %s", block.String())
	}
}
