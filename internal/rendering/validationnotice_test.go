package rendering

import (
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
