package rendering

import (
	"bytes"
	"context"
	"menata.app/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A form the browser holds back for an empty required control sends no request, so neither the refusal notice nor the
// server can say why. The shell says it, on every page, from the one place that already shows a refused write -- the
// Dokter Kecil contract read from a document sat unsaveable and silent because the reader left a required date empty.
func TestShellSaysWhyAFormWasHeldBackForARequiredField(t *testing.T) {
	html := renderLeaveGuardShell(t, "admin", "/switch-workspace")
	for _, want := range []string{`htmx:validation:halted`, `required field`, `scrollIntoView`} {
		if !strings.Contains(html, want) {
			t.Errorf("the shell does not carry %q: a form held back for an empty required field would stay silent", want)
		}
	}
}

// A control under a column header has no visible label of its own: it carries a hidden one, so a screen reader still hears
// which cell it is -- and one with no `label` draws none.
func TestFormInputUnderAColumnHeaderCarriesAHiddenLabel(t *testing.T) {
	var with, without bytes.Buffer
	if err := formInput("c1", "rows.0.0.fld_gross", "number", "", "54", "Instalments 1 · Gross", false, false, false).Render(context.Background(), &with); err != nil {
		t.Fatal(err)
	}
	if err := formInput("c1", "rows.0.0.fld_gross", "number", "", "54", "", false, false, false).Render(context.Background(), &without); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with.String(), `<label for="c1" class="sr-only">Instalments 1 · Gross</label>`) {
		t.Errorf("no hidden label: %s", with.String())
	}
	if strings.Contains(without.String(), "<label") {
		t.Errorf("a control with no label drew one: %s", without.String())
	}
}

// A money control carries the Workspace's marks for the shell's script (grouping as it is typed, two decimals, the plain
// decimal sent); a plain number stays a number control.
func TestMoneyControlCarriesTheWorkspacesMarks(t *testing.T) {
	render := func(nf domain.NumberFormat, money bool) string {
		ctx := WithCurrentWorkspace(context.Background(), domain.Workspace{Number: nf}, "W", false)
		var buf bytes.Buffer
		if err := formInput("c1", "fld_gross", "number", "", "54000000", "", false, false, money).Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if got := render(domain.NumberFormatInternational, true); !strings.Contains(got, `data-money`) || !strings.Contains(got, `data-thousands=","`) || !strings.Contains(got, `data-decimal="."`) || !strings.Contains(got, `value="54000000"`) {
		t.Errorf("international money control: %s", got)
	}
	if got := render(domain.NumberFormatIndonesian, true); !strings.Contains(got, `data-thousands="."`) || !strings.Contains(got, `data-decimal=","`) {
		t.Errorf("indonesian money control: %s", got)
	}
	if got := render(domain.NumberFormat{}, true); !strings.Contains(got, `data-thousands=","`) {
		t.Errorf("a Workspace that declares nothing must read international: %s", got)
	}
	if got := render(domain.NumberFormatIndonesian, false); strings.Contains(got, "data-money") || !strings.Contains(got, `type="number"`) {
		t.Errorf("a plain number became a money control: %s", got)
	}
}

// Script is the exception, and the writing guide lists each one (its "Script is the exception" table): a template that carries an
// inline <script> must be named there, and the money control's script lives in the shell and nowhere else. A new script with
// no row in the guide fails here, so the list cannot fall behind.
func TestEveryInlineScriptIsAListedException(t *testing.T) {
	guide, err := os.ReadFile("../../writing-guide.md")
	if err != nil {
		t.Fatal(err)
	}
	g := string(guide)
	start := strings.Index(g, "### Script is the exception")
	if start < 0 {
		t.Fatal("writing-guide.md has no \"Script is the exception\" section")
	}
	section := g[start:]
	if end := strings.Index(section[10:], "\n## "); end > 0 {
		section = section[:end+10]
	}
	files, _ := filepath.Glob("*.templ")
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		inline := false
		for _, line := range strings.Split(string(src), "\n") {
			if trimmed := strings.TrimSpace(line); !strings.HasPrefix(trimmed, "//") && strings.HasPrefix(trimmed, "<script>") {
				inline = true
			}
		}
		if base := strings.TrimSuffix(f, ".templ"); inline && !strings.Contains(section, base) {
			t.Errorf("%s carries an inline <script> the writing guide's \"Script is the exception\" table does not name", f)
		}
		if strings.Contains(string(src), "data-money") && f != "controls.templ" && f != "appshell.templ" {
			t.Errorf("%s handles the money control: its script belongs in the shell, and its markup in controls.templ", f)
		}
	}
}
