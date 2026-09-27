package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Package machine_identity_test.go holds one rule, learned expensively on 2026-09-27: outside
// internal/action itself, no code may decide "is this Machine one of Document Approval's" by
// comparing a bare id.
//
// Machine ids were unique across the process until Workspace isolation shipped that day, so
// `m.ID == action.DocumentMachineID` really did identify *the* Document Approval Document, and 23
// call sites were written on that basis. Isolation made a different Workspace's own, unrelated
// Machine sharing the id legitimate -- that is the point of it -- and a generated "Document
// Tracking" Application took the name within hours. Its detail page then panicked twice inside
// code that had taken the id as proof of identity.
//
// The first attempted fix forbade seven ids outright so those comparisons could stay. That made
// the model give way to the code, and the owner rejected it on exactly those grounds. The real
// fix is action.IsDocument/IsStep, which ask which *Application* claims the Machine
// (domain.Machine.ApplicationID, stamped at load, per-Workspace since Machines moved onto the
// Workspace) as well as which Machine within it. This test is what stops the 24th bare comparison
// from being written, since nothing else can: no metadata validator can see a Go string
// comparison, and every one of those 23 sites looked perfectly reasonable when written.
//
// Deliberately narrow. Only *comparisons* are forbidden. Using the same constants to build a URL
// (fmt.Sprintf("/machines/%s/...", action.DocumentMachineID)), look a Machine up by id
// (machines[action.StepMachineID]), or query records (store.ListRecordsBy(ctx,
// action.StepMachineID, ...)) is untouched: those name a Machine, they do not claim an identity.
var machineIdentityComparison = regexp.MustCompile(
	`(==|!=)\s*action\.(Document|Step|Signature|Template|TemplateStep)MachineID|action\.(Document|Step|Signature|Template|TemplateStep)MachineID\s*(==|!=)`)

// identityCheckedPackages are the planes that render or handle a Machine generically, and
// therefore the ones where mistaking another Application's Machine for Document Approval's has
// consequences. internal/action is excluded as the owner of both the constants and the
// predicates.
var identityCheckedPackages = []string{
	"internal/web",
	"internal/rendering",
	"internal/composition",
	"internal/execution",
}

func TestNoBareMachineIDIdentityChecks(t *testing.T) {
	scanned := 0
	for _, pkg := range identityCheckedPackages {
		dir := filepath.Join(repoRoot(), pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", pkg, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_templ.go") {
				continue
			}
			if !strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".templ") {
				continue
			}
			path := filepath.Join(dir, name)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			scanned++
			for i, line := range strings.Split(string(src), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue // a comment may name the old shape while explaining it
				}
				if machineIdentityComparison.MatchString(line) {
					t.Errorf("%s/%s:%d compares a Machine id directly:\n\t%s\n"+
						"use action.IsDocument/IsStep instead -- they check which Application claims the Machine "+
						"(domain.Machine.ApplicationID) as well as which Machine it is. Since Workspace isolation "+
						"(2026-09-27) another Workspace may legitimately hold its own Machine under this id, and a "+
						"bare comparison would treat it as Document Approval's own",
						pkg, name, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no source files were scanned -- this gate would pass an empty tree silently")
	}
}
