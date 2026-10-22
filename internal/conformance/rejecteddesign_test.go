package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestRejectedDesignValuesStayOut is GS2 of menata-app-document's guides/menjaga-design-system.md (rule R5: one meaning,
// one value; a rejected value is marked rejected, not left silent). The Design System artifact lists what it rejected
// for a meaning in a `rejected` token family. This gate reads that family from the repo copy,
// `design-system/tokens.json`, and refuses the matching Tailwind classes in any hand-written `.templ`, except where
// `rejectedDesignExempt` names a site and why it is not that meaning.
//
// It exists because four of the guide's eleven inconsistencies were a second value for one meaning, kept silently:
// emerald-500 as a finished mark (2.46:1) beside mark-good (3.67:1), 13px between meta and body, 15→14px, and green
// beside emerald. A value someone once rejected is the easiest one to bring back, because it looks like it already
// belongs.
//
// **Classification is required, as in GS1.** Every `rejected` entry must have a pattern in `rejectedDesignClasses`, or
// the gate fails and says so. A pattern for an entry the artifact no longer rejects fails too, and so does an
// exemption whose count no longer matches. `TestNoHandWrittenMarkGood` also refuses emerald-500. The overlap is
// deliberate: that gate owns the mark-good readers, this one owns the artifact's list.
func TestRejectedDesignValuesStayOut(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(), "design-system", "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tokens struct {
		Rejected struct {
			Tokens []struct{ Name string } `json:"tokens"`
		} `json:"rejected"`
	}
	if err := json.Unmarshal(b, &tokens); err != nil {
		t.Fatal(err)
	}
	if len(tokens.Rejected.Tokens) == 0 {
		t.Fatal("design-system/tokens.json has no `rejected` family -- the artifact's list moved or the copy is stale")
	}
	listed := map[string]bool{}
	for _, tk := range tokens.Rejected.Tokens {
		listed[tk.Name] = true
		if _, ok := rejectedDesignClasses[tk.Name]; !ok {
			t.Errorf("the Design System rejects %q and rejectedDesignClasses has no pattern for it -- add the Tailwind classes that would bring it back", tk.Name)
		}
	}
	for name := range rejectedDesignClasses {
		if !listed[name] {
			t.Errorf("rejectedDesignClasses has %q, which the Design System copy no longer rejects -- remove it", name)
		}
	}

	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	comment := regexp.MustCompile(`//[^\n]*`)
	got := map[string]int{} // "name|file" -> count
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".templ") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		text := comment.ReplaceAllString(string(src), "")
		for name, re := range rejectedDesignClasses {
			if n := len(re.FindAllString(text, -1)); n > 0 {
				got[name+"|"+e.Name()] = n
			}
		}
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		want := rejectedDesignExempt[k].count
		if got[k] != want {
			parts := strings.SplitN(k, "|", 2)
			t.Errorf("%s: %d use(s) of the Design System's rejected %q (exempt: %d) -- use the value it was replaced with, or, if this site is not that meaning, add it to rejectedDesignExempt with the reason", parts[1], got[k], parts[0], want)
		}
	}
	for k, ex := range rejectedDesignExempt {
		if got[k] != ex.count {
			t.Errorf("rejectedDesignExempt[%q] = %d (%s) but the tree has %d -- a closed exemption must match exactly", k, ex.count, ex.reason, got[k])
		}
	}
}

// rejectedDesignClasses maps each artifact `rejected` entry to the Tailwind classes that would reintroduce it.
var rejectedDesignClasses = map[string]*regexp.Regexp{
	"body-compact":  regexp.MustCompile(`(?:^|[\s"{:])text-\[13px\](?:$|[\s"}])`),
	"mark-good-500": regexp.MustCompile(`(?:^|[\s"{])(?:hover:)?(?:bg|border)-emerald-500(?:$|[\s"}])`),
}

// rejectedDesignExempt is the closed list of sites that use a rejected value for a different meaning, keyed
// "<rejected name>|<file>", with the exact count.
var rejectedDesignExempt = map[string]struct {
	count  int
	reason string
}{}
