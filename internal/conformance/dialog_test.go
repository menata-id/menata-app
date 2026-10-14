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
			if m := classAttr.FindStringSubmatch(tag); m == nil || !hasBareClassPart(m[1], "dialogClasses(ctx)") {
				t.Errorf("%s: a <dialog> whose class does not call dialogClasses(ctx) -- its surface, padding, ink, elevation and scrim "+
					"would stop following theme:; add the reader and keep only the width at the site", e.Name())
			}
		}
	}
	if total < 2 {
		t.Fatalf("found %d <dialog> tags, want at least the 2 rencana 2.3d measured -- the scan is reading the wrong text", total)
	}
}

// hasBareClassPart reports whether a class expression `{ a, b, reader(ctx) }` has `part` as one of its top-level parts,
// exactly: a call wrapped in `templ.KV(..., false)`, a string literal naming it and a mention inside a longer expression
// are not the reader being applied (the review of 2.3d drove three of those through a `strings.Contains`).
func hasBareClassPart(expr, part string) bool {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "{") || !strings.HasSuffix(expr, "}") {
		return false
	}
	expr = expr[1 : len(expr)-1]
	depth, start, quote := 0, 0, byte(0)
	parts := []string{}
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch {
		case quote != 0:
			if c == quote && (i == 0 || expr[i-1] != '\\') {
				quote = 0
			}
		case c == '"' || c == '`' || c == '\'':
			quote = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, strings.TrimSpace(expr[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	for _, p := range parts {
		if p == part {
			return true
		}
	}
	return false
}

func TestHasBareClassPart(t *testing.T) {
	for expr, want := range map[string]bool{
		`{ "a", dialogClasses(ctx) }`:                         true,
		`{ dialogClasses(ctx) }`:                              true,
		`{ "a", templ.KV(dialogClasses(ctx), false) }`:        false,
		`{ "a", "dialogClasses(ctx)" }`:                       false,
		`{ "a", x + dialogClasses(ctx) }`:                     false,
		`"dialogClasses(ctx)"`:                                false,
		`{ "a, dialogClasses(ctx)", other(ctx) }`:             false,
		`{ "max-w-md", fn(ctx, "x", y), dialogClasses(ctx) }`: true,
	} {
		if got := hasBareClassPart(expr, "dialogClasses(ctx)"); got != want {
			t.Errorf("hasBareClassPart(%s) = %v, want %v", expr, got, want)
		}
	}
}
