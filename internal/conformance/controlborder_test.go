package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// handWrittenControlBorders freezes, per file, the literal `border-slate-300` tokens left in `internal/rendering`
// (2026-10-12, rencana 0.2d). `border-slate-300` is what `borderClass(ctx, domain.BorderControl)` answers by
// default, the rim of a control, and a literal one does not move when a Workspace declares
// `theme.border.control`.
//
// **Read by meaning, not by value (menjaga-design-system R2).** Six control rims moved byte-identically: Account's
// three button-shaped controls, the Approval Inbox's inactive filter chip, and the two complete-checkbox circles.
// Every entry below is the same grey with another job. Moving one into the control role would make a Theme key for
// controls restyle an empty state or a signature marker.
//
// Counted after `joinClassExpressions`, so a token inside a `templ.KV` string is seen. **Blind spots:** a token
// returned by a Go function (`avatarClasses`' pending ring, a dashed outline for an invitation that has not been
// accepted, is one), a variant-prefixed token, and whether a floor's reason is still true.
var handWrittenControlBorders = map[string]struct {
	n   int
	why string
}{
	"approvalinbox.templ":   {1, "the dashed empty-state box (an EmptyState candidate, rencana 2.3), not a control"},
	"groups.templ":          {1, "the dashed empty-state box, not a control"},
	"workspacehome.templ":   {2, "two dashed empty-state boxes, not controls"},
	"reviewdocument.templ":  {3, "signature markers: a waiting box's dashed outline, a dotted signing line, the dashed drawing canvas"},
	"login.templ":           {2, "pre-auth inputs: no Workspace on ctx, so no theme reaches them (also floors in the field ratchet)"},
	"authshell.templ":       {2, "pre-auth inputs: no Workspace on ctx, so no theme reaches them (also floors in the field ratchet)"},
	"chooseworkspace.templ": {1, "pre-auth dashed \"create a workspace\" link: no theme reaches it, and dashed is an empty-state affordance"},
}

func TestHandWrittenControlBordersOnlyShrink(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	actual := map[string]int{}
	readers := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		src := stripTemplComments(string(b))
		if e.Name() == "layout.templ" {
			readers = strings.Count(src, `return "border-slate-300"`)
			continue
		}
		for _, m := range classAttr.FindAllStringSubmatch(joinClassExpressions(src), -1) {
			for _, tok := range strings.Fields(m[1]) {
				if tok == controlBorderToken {
					actual[e.Name()]++
				}
			}
		}
	}
	if readers != 1 {
		t.Fatalf("expected borderClass's one `border-slate-300` arm in layout.templ, found %d -- the scan is reading the wrong text", readers)
	}
	files := map[string]bool{}
	for f := range actual {
		files[f] = true
	}
	for f := range handWrittenControlBorders {
		files[f] = true
	}
	for f := range files {
		got, want := actual[f], handWrittenControlBorders[f].n
		switch {
		case got > want:
			t.Errorf("%s: %d literal border-slate-300, frozen at %d -- a control's rim reads borderClass(ctx, domain.BorderControl), "+
				"so a Workspace's theme.border.control moves it; if this grey is not a control's rim, say what it is in handWrittenControlBorders", f, got, want)
		case got < want:
			t.Errorf("%s: %d literal border-slate-300, down from %d -- lower handWrittenControlBorders to lock it in", f, got, want)
		}
	}
}
