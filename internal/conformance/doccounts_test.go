package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/registry"
)

// TestDocumentedDesignSystemCountsMatchTheCode is GS5 of menata-app-document's guides/menjaga-design-system.md (rule
// R7: a number in prose is read from its source, or dated). It holds the two crisp counts that went stale while the
// code moved, both found by reading, not by any test:
//   - "Six Theme token categories" in writing-guide.md, beside "All nine" a hundred lines later;
//   - "registry.Components holds seven §13 contracts" in 007 §40, with ten registered.
//
// It reads every "<number-word> (Theme) token categories" and every "registry.Components` holds <number-word>" phrase
// in the normative and agent-facing documents. It compares each with the code: the map fields of `domain.Theme`
// (parsed from source, so a new category counts the moment it is declared), and `len(registry.Components)`.
//
// **Deliberately narrow.** CLAUDE.md's ratchet narratives also carried stale counts ("4 remain"), but those sentences
// take a dozen shapes, and a pattern loose enough to read them all would flag dated history as stale (the ~50%-false
// shape this repo has deleted twice). Those were rewritten as dated numbers pointing at the map instead. This gate
// covers only claims with one fixed wording. A third becomes worth adding the day it goes stale too.
func TestDocumentedDesignSystemCountsMatchTheCode(t *testing.T) {
	root := repoRoot()
	docs := []string{"writing-guide.md", "capabilities.md", "CLAUDE.md", "006-runtime-model.md", "007-composable-runtime-architecture.md"}

	categories := themeCategoryCount(t, filepath.Join(root, "internal", "domain", "theme.go"))
	components := len(registry.Components)

	catRe := regexp.MustCompile(`(?i)\b([a-z]+) (?:theme )?token categories\b`)
	regRe := regexp.MustCompile("(?i)registry\\.Components` holds ([a-z]+)\\b")
	seen := 0
	for _, name := range docs {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(b)
		for _, m := range catRe.FindAllStringSubmatch(text, -1) {
			n, ok := numberWords[strings.ToLower(m[1])]
			if !ok {
				continue
			}
			seen++
			if n != categories {
				t.Errorf("%s says %q, but domain.Theme declares %d token categories -- correct the prose, or date it", name, m[0], categories)
			}
		}
		for _, m := range regRe.FindAllStringSubmatch(text, -1) {
			n, ok := numberWords[strings.ToLower(m[1])]
			if !ok {
				continue
			}
			seen++
			if n != components {
				t.Errorf("%s says %q, but registry.Components has %d -- correct the prose, or date it", name, m[0], components)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no count claim was found in any document -- the patterns no longer match the prose, and this gate is checking nothing")
	}
}

var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
}

// themeCategoryCount counts domain.Theme's map-typed fields: one per token category.
func themeCategoryCount(t *testing.T, path string) int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		ts, ok := node.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "Theme" {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, field := range st.Fields.List {
			if _, isMap := field.Type.(*ast.MapType); isMap {
				n += len(field.Names)
			}
		}
		return false
	})
	if n == 0 {
		t.Fatal("domain.Theme has no map fields -- the parse is looking at the wrong type")
	}
	return n
}
