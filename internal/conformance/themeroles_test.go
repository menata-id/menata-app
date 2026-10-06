package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// unreadThemeRoles are the declarable Theme roles that no renderer reads, each with the reason. A role in
// theme.go is vocabulary a Workspace author may write in YAML and the loader validates -- so a role nothing
// reads is a key that loads and changes no pixel (2026-10-06: `heading` and `eyebrow` were exactly this until
// `staticText` started reading them). The list may only shrink: an entry whose role is now read fails, and so
// does one naming a constant that no longer exists. A new role arrives with its reader or lands here with a
// reason, which is the Theme-side form of "a primitive arrives with the uses it replaces".
//
// All three need a Component, not a token (007 §12.3): the elements they style (an input, a table row, a
// run of reset-weight text) are drawn by no primitive today, so there is nowhere for a reader to live that is
// not a hand-written screen. `RadiusControl` and `InkBody` left on 2026-10-06 when Button became the first
// Component to read them. Declaring one without a reader would be the shape-before-need this repo has removed three times.
var unreadThemeRoles = map[string]string{
	"WeightBody":    "text explicitly reset to normal weight; no primitive draws such text",
	"BorderControl": "the outline of an input; Button does not read it because its outline is slate-200 (the mockups), not slate-300 -- owner decision D8, and a Field control would be its first reader",
	"BorderDivider": "the rule between rows of a list or table; a divider Component or a table primitive would read it",
}

var themeRoleLineComment = regexp.MustCompile(`//[^\n]*`)

func themeRoleConstants(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot(), "internal", "domain", "theme.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		typ, ok := spec.Type.(*ast.Ident)
		if !ok || !strings.HasSuffix(typ.Name, "Role") {
			return true
		}
		for _, name := range spec.Names {
			out = append(out, name.Name)
		}
		return true
	})
	sort.Strings(out)
	return out
}

// themeReaderSource is every hand-written .templ and .go under internal/rendering, comments removed, so that a
// comment naming a role does not count as reading it. Generated files and tests are excluded: the first
// duplicates the .templ and the second reads a role to prove it, not to render it.
func themeReaderSource(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot(), "internal", "rendering")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rendering: %v", err)
	}
	var all strings.Builder
	for _, e := range entries {
		n := e.Name()
		isTempl := strings.HasSuffix(n, ".templ")
		isGo := strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") && !strings.HasSuffix(n, "_templ.go")
		if e.IsDir() || !(isTempl || isGo) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		all.WriteString(themeRoleLineComment.ReplaceAllString(string(b), ""))
		all.WriteByte('\n')
	}
	return all.String()
}

func TestEveryThemeRoleHasAReaderOrAReason(t *testing.T) {
	roles := themeRoleConstants(t)
	if len(roles) < 20 {
		t.Fatalf("found %d role constants in theme.go -- this gate would pass by measuring almost nothing", len(roles))
	}
	src := themeReaderSource(t)
	if !strings.Contains(src, "domain.RoleBody") {
		t.Fatal("rendering source does not name domain.RoleBody -- the reader scan is looking at the wrong text")
	}

	isRole := map[string]bool{}
	for _, r := range roles {
		isRole[r] = true
		read := regexp.MustCompile(`domain\.` + r + `\b`).MatchString(src)
		reason, listed := unreadThemeRoles[r]
		switch {
		case !read && !listed:
			t.Errorf("domain.%s is declarable in a Workspace's theme: but no renderer reads it, so a declared value changes nothing. Give it a reader, or list it in unreadThemeRoles with the reason", r)
		case read && listed:
			t.Errorf("unreadThemeRoles still lists %s, which a renderer now reads -- remove the entry (%q)", r, reason)
		}
	}
	for _, name := range sortedKeys(unreadThemeRoles) {
		if !isRole[name] {
			t.Errorf("unreadThemeRoles names %s, which is not a Role constant in theme.go -- remove the entry", name)
		}
		if strings.TrimSpace(unreadThemeRoles[name]) == "" {
			t.Errorf("unreadThemeRoles[%s] has no reason", name)
		}
	}
}
