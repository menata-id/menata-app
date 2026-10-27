package rendering

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// TestPopoverPanelStacksAboveBoardCardChrome holds the panel of a dropdown Popover above every layer a board card draws.
// Both are positioned, and the card comes later in the page, so at an equal step the card's completion circle painted over an
// open Switch project menu (2026-10-27). It reads the two sources as text, as the other class gates do: the step the panel is
// drawn at, and the highest step any layer in machine.templ asks for (including the card editor's open state).
func TestPopoverPanelStacksAboveBoardCardChrome(t *testing.T) {
	step := regexp.MustCompile(`(?:^|[\s"'])(?:[a-z-]+:)*z-(\d+)\b`)
	read := func(path string) []string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, m := range step.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
		return out
	}
	panel := regexp.MustCompile(`"fixed z-(\d+) mt-1\.5 w-72"`)
	src, err := os.ReadFile("popover.templ")
	if err != nil {
		t.Fatal(err)
	}
	m := panel.FindSubmatch(src)
	if m == nil {
		t.Fatal(`popover.templ no longer draws its dropdown panel as "fixed z-N mt-1.5 w-72"; update this gate with the new shape`)
	}
	panelStep, _ := strconv.Atoi(string(m[1]))
	cardMax := 0
	for _, s := range read("machine.templ") {
		n, _ := strconv.Atoi(s)
		cardMax = max(cardMax, n)
	}
	if cardMax == 0 {
		t.Fatal("found no z-N in machine.templ: the gate would measure nothing")
	}
	if panelStep <= cardMax {
		t.Errorf("a dropdown Popover panel is drawn at z-%d but a board card draws as high as z-%d: the card paints over an open menu", panelStep, cardMax)
	}
}
