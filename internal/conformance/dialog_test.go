package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var dialogTagStart = regexp.MustCompile(`(?i)<dialog[\s>]`)

// TestEveryDialogReadsTheShell is a zero-floor gate beside `dialogClasses` (rencana 2.3d, 2026-10-14; build, migrate,
// then gate). A modal `<dialog>` in `internal/rendering` draws its surface, padding, ink, elevation and scrim through that
// one reader, so a Workspace's `theme:` moves every dialog together; width stays per site (440px and 448px differ, and
// folding them is a decision nobody took).
//
// The slice measured two dialogs (`signatureModal`, `leaveWorkspaceGuard`) and registered nothing: what makes a dialog a
// dialog in this tree is when it opens and closes (a dirty-form check; a canvas with its own submit handling), which is
// 007 §12.3's `actions`, with no declaration yet. The shell, which is only class, is what could be shared, as a reader.
//
// The `<dialog>` tag is read with the quote- and brace-aware scanner the button gate uses, and its `class` attribute must
// call `dialogClasses(`. **Blind spots, stated:** a dialog assembled in Go, and a `popover` (the menus use `popover`, not
// `<dialog>`, and are another job).
func TestEveryDialogReadsTheShell(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	total := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := stripTemplComments(string(b))
		for i := 0; i < len(src); {
			loc := dialogTagStart.FindStringIndex(src[i:])
			if loc == nil {
				break
			}
			start := i + loc[0]
			end := scanTagEnd(src, i+loc[1]-1)
			tag := src[start:end]
			i = end
			total++
			if m := classAttr.FindStringSubmatch(tag); m == nil || !strings.Contains(m[1], "dialogClasses(") {
				t.Errorf("%s: a <dialog> whose class does not call dialogClasses(ctx) -- its surface, padding, ink, elevation and scrim "+
					"would stop following theme:; add the reader and keep only the width at the site", e.Name())
			}
		}
	}
	if total < 2 {
		t.Fatalf("found %d <dialog> tags, want at least the 2 rencana 2.3d measured -- the scan is reading the wrong text", total)
	}
}
