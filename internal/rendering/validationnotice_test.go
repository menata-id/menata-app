package rendering

import (
	"bytes"
	"context"
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
	if err := formInput("c1", "rows.0.0.fld_gross", "number", "", "54", "Instalments 1 · Gross", false, false).Render(context.Background(), &with); err != nil {
		t.Fatal(err)
	}
	if err := formInput("c1", "rows.0.0.fld_gross", "number", "", "54", "", false, false).Render(context.Background(), &without); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with.String(), `<label for="c1" class="sr-only">Instalments 1 · Gross</label>`) {
		t.Errorf("no hidden label: %s", with.String())
	}
	if strings.Contains(without.String(), "<label") {
		t.Errorf("a control with no label drew one: %s", without.String())
	}
}
