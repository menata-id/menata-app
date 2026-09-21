package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"menata.app/internal/authorization"
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

// transitionGuards names the handlers that must hold a Machine's own declared state model against
// the write they perform (Case 03 Fase 7), for the same reason the map above exists: what these
// calls enforce used to be hand-written Go (allowsDecisionChange, naming one Machine and one
// Field), and dropping the declared replacement would reopen the bypass silently -- a Document
// could be written straight to `approved` through the generic route with no step decided, exactly
// as it could before this phase.
var transitionGuards = map[string]map[string]string{
	"record.go":   {"updateRecordForm": "allowsTransition"},
	"api.go":      {"updateRecord": "allowsTransition"},
	"approval.go": {"decideStep": "declaredDecision"},
}

func TestMutatingHandlersCheckDeclaredTransitions(t *testing.T) {
	fset := token.NewFileSet()
	for file, handlers := range transitionGuards {
		path := filepath.Join(repoRoot(), "internal", "web", file)
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for handlerName, guard := range handlers {
			if !funcBodyCallsIdent(f, handlerName, guard) {
				t.Errorf("internal/web/%s: %s no longer calls %s -- a declared transition model that no route enforces is a state machine drawn on a screen and nowhere else", file, handlerName, guard)
			}
		}
	}
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

// TestWorkspaceAdminIsNotAPermissionBypass holds a decision, not an implementation detail, and it
// is here because the decision is invisible in the code: there is no "if admin { return true }"
// to read, so nothing marks its absence as deliberate, and adding one would look like a fix.
//
// The question is real and the owner's own board 06 draws the other answer -- an ADMIN column
// ticked for every transition. The answer taken (2026-09-21) is that holding the Workspace admin
// role satisfies a Permission that *asks* for it (`workspace_role: admin`) and changes nothing
// about any Permission that does not. An administrator who can approve a document they are not an
// approver of is precisely the audit hole an approval flow exists to close, and the owner's own
// member-role-detail.html mockup says the same thing in its own caption: the Workspace role
// "controls workspace administration, independent of application permissions."
//
// What this test can check mechanically is that authorization's evaluator never reads the actor's
// Workspace role except to compare it against a Permission that named one. Any other use of
// Actor.WorkspaceRole in that package is, by construction, a bypass being introduced.
func TestWorkspaceAdminIsNotAPermissionBypass(t *testing.T) {
	path := filepath.Join(repoRoot(), "internal", "authorization", "permission.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	const onlyAllowedUse = "if p.WorkspaceRole != \"\" && actor.WorkspaceRole != p.WorkspaceRole {"
	body := string(src)
	if !strings.Contains(body, onlyAllowedUse) {
		t.Fatalf("internal/authorization/permission.go no longer compares the actor's workspace role against a permission that asked for one -- the workspace_role arm is gone or changed shape")
	}
	if n := strings.Count(body, "actor.WorkspaceRole"); n != 1 {
		t.Errorf("actor.WorkspaceRole is read %d times in internal/authorization/permission.go, want exactly 1 (the comparison above) -- a second read is a bypass: holding admin must never grant what a permission has not asked for", n)
	}

	// The same decision, asserted behaviourally rather than by reading source: an admin who is not
	// the record's own actor is still refused.
	m := &domain.Machine{
		ID:          "mch_approval_step",
		Fields:      []domain.Field{{ID: "fld_assignee", Name: "Assignee", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID}},
		Permissions: []domain.Permission{{ID: "prm_decide_own_step", Action: domain.ActionDecide, ActorField: "fld_assignee"}},
	}
	step := map[string]any{"fld_assignee": "usr_rina"}
	admin := domain.Actor{ID: "usr_ana", WorkspaceRole: domain.WorkspaceRoleAdmin}
	if authorization.AllowsAction(m, domain.ActionDecide, step, admin) {
		t.Error("a Workspace admin who is not this step's own approver must still be refused -- admin is not a bypass")
	}
}
