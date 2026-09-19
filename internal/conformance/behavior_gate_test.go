package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// Package behavior_gate_test.go holds the executable half of two disciplines this session
// established in prose (CLAUDE.md, writing-guide.md §8/§10, capabilities.md) -- the same posture
// metadata_alignment_test.go already states for Metadata First: prose says it, these tests are
// what actually holds it, so the next edit (human or AI) can't silently drift from it without a
// failing `go test`.

// mutatingWriteGuards names, for each generic mutating handler in internal/web, the guard
// function call its body must still contain. Before allowsRecordEdit/deleteAllowed existed, there
// was no conformance guard preventing a Machine-level edit/delete Permission from being silently
// ungoverned (capabilities.md's own "Not built" note on that gap, closed 2026-09-19) -- this test
// is what stops the same gap reopening by accident: a future refactor of record.go/api.go that
// drops one of these calls (e.g. while restructuring the guard chain) fails here instead of
// shipping an authorization regression nobody notices until it's reported.
var mutatingWriteGuards = map[string]map[string]string{
	"record.go": {
		"updateRecordForm": "allowsRecordEdit",
		"deleteRecord":     "deleteAllowed",
	},
	"api.go": {
		"updateRecord":    "allowsRecordEdit",
		"deleteRecordAPI": "deleteAllowed",
	},
}

func TestMutatingHandlersCallTheirAuthorizationGuard(t *testing.T) {
	fset := token.NewFileSet()
	for file, handlers := range mutatingWriteGuards {
		path := filepath.Join(repoRoot(), "internal", "web", file)
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for handlerName, guard := range handlers {
			if !funcBodyCallsIdent(f, handlerName, guard) {
				t.Errorf("internal/web/%s: %s no longer calls %s -- this Machine-level authorization guard must not be silently dropped from a generic mutating route", file, handlerName, guard)
			}
		}
	}
}

// funcBodyCallsIdent reports whether the top-level function funcName in f references the
// identifier ident anywhere in its body, including inside a nested closure (updateRecordForm's
// own shape is `func(...) http.HandlerFunc { return func(w, req) {...} }` -- ast.Inspect walks
// the whole subtree regardless of nesting, so this needs no special handling for that).
func funcBodyCallsIdent(f *ast.File, funcName, ident string) bool {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName {
			continue
		}
		found := false
		ast.Inspect(fn, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == ident {
				found = true
			}
			return true
		})
		return found
	}
	return false
}

// TestCapabilitiesDocumentsKnownActionsAndServices keeps capabilities.md honest against
// domain.KnownActions/domain.KnownServices the same way TestCapabilitiesComponentsTableMatchesTempl
// already does for Shared rendering components: both are closed, static seams (007 §14) --
// declaring a new Action or Service is a real Go change, not a metadata one, so it's exactly the
// kind of addition a commit can make without anyone remembering to also write it down. This only
// checks the doc still names each one somewhere (not a structured table row, matching
// TestCapabilitiesComponentsTableCitesPromotionGuide's simpler substring posture for the same
// reason): capabilities.md's own prose around these primitives already cites specific identifiers
// inline rather than only in table cells.
func TestCapabilitiesDocumentsKnownActionsAndServices(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(repoRoot(), "capabilities.md"))
	if err != nil {
		t.Fatalf("read capabilities.md: %v", err)
	}
	text := string(doc)

	for action := range domain.KnownActions {
		if !strings.Contains(text, action) {
			t.Errorf("domain.KnownActions declares %q, but capabilities.md never mentions it -- document the new Action or its removal", action)
		}
	}
	for service := range domain.KnownServices {
		if !strings.Contains(text, service) {
			t.Errorf("domain.KnownServices declares %q, but capabilities.md never mentions it -- document the new Service or its removal", service)
		}
	}
}
