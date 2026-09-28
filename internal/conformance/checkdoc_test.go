package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestCheckDocsMirrorMetadatasOwnKeys closes a two-lists-that-drift gap the first template install
// walked straight into.
//
// internal/installer writes every metadata file through WriteFileStrict, which re-parses the bytes with
// unknown-key rejection before renaming them into place -- a real guard against a hallucinated or
// mistyped key, and the reason a half-written file can never reach a reload. The strict decode needs a
// target struct, and those targets (installer's FullMachineCheckDoc, FullApplicationCheckDoc,
// WorkspaceManifestCheckDoc) are hand-mirrors of internal/metadata's own unexported machineDoc,
// applicationDoc and workspaceDoc.
//
// So a key added to the real doc and not to the mirror makes the strict check reject a file that
// internal/metadata accepts perfectly well. That is not a hypothetical: `blocks_member_removal` landed
// on machineDoc on 2026-09-27, the mirror was not updated, and copying mch_approval_step verbatim
// failed on 2026-09-28 with "field blocks_member_removal not found" -- a file the loader would have
// been happy with. The same shape would have broken the AI assistant's own extend path the first time
// it edited that Machine.
//
// Read from source rather than by reflection, because metadata's own types are unexported: the yaml
// tags are the declaration, and comparing them is comparing the two lists themselves.
func TestCheckDocsMirrorMetadatasOwnKeys(t *testing.T) {
	metadataDir := filepath.Join(repoRoot(), "internal", "metadata")
	installerDir := filepath.Join(repoRoot(), "internal", "installer")

	pairs := []struct {
		realFile, realType   string
		mirrorFile, mirrorTy string
	}{
		{"parse.go", "machineDoc", "write.go", "FullMachineCheckDoc"},
		{"application.go", "applicationDoc", "write.go", "FullApplicationCheckDoc"},
		{"application.go", "workspaceDoc", "write.go", "WorkspaceManifestCheckDoc"},
	}
	for _, p := range pairs {
		real := yamlKeysOf(t, filepath.Join(metadataDir, p.realFile), p.realType)
		mirror := yamlKeysOf(t, filepath.Join(installerDir, p.mirrorFile), p.mirrorTy)
		if len(real) == 0 {
			t.Fatalf("found no yaml keys on internal/metadata.%s -- this gate is measuring nothing", p.realType)
		}
		var missing []string
		for _, key := range real {
			if !contains(mirror, key) {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			t.Errorf("installer.%s is missing %v, which internal/metadata.%s declares -- installer.WriteFileStrict would reject a file the loader accepts (this is exactly how blocks_member_removal broke the first template install)",
				p.mirrorTy, missing, p.realType)
		}
		// The reverse is a weaker problem -- a mirror key metadata no longer has just makes the strict
		// check laxer than it could be -- but it is still a stale list, and saying so is free.
		var extra []string
		for _, key := range mirror {
			if !contains(real, key) {
				extra = append(extra, key)
			}
		}
		if len(extra) > 0 {
			t.Errorf("installer.%s declares %v, which internal/metadata.%s no longer has -- remove them rather than leaving the mirror describing a shape that is gone", p.mirrorTy, extra, p.realType)
		}
	}
}

// yamlKeysOf returns the yaml tag names of every field on one struct type in one file.
func yamlKeysOf(t *testing.T, path, typeName string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var keys []string
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != typeName {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, f := range st.Fields.List {
			if f.Tag == nil {
				continue
			}
			tag := reflect.StructTag(strings.Trim(f.Tag.Value, "`")).Get("yaml")
			name, _, _ := strings.Cut(tag, ",")
			if name != "" && name != "-" {
				keys = append(keys, name)
			}
		}
		return false
	})
	sort.Strings(keys)
	return keys
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestMetadataDecodesStrictly is the lock on the hole closed 2026-09-28: every YAML this runtime reads
// goes through internal/metadata's own decodeStrict, which rejects a key it has no home for.
//
// A bare yaml.Unmarshal is lenient, and its leniency is invisible -- the file loads, the Machine is
// built, and the capability the misspelled key was meant to declare simply never appears. That was true
// for months and cost a real investigation (a copy of metadata/ using the retired singular `view:` block
// loaded with zero Views and no error). Nothing but this stops a fourth decode site being added the
// lenient way, because the lenient way is the one the yaml package makes easiest.
//
// internal/aiassist and internal/installer decode strictly too, through installer.WriteFileStrict, and
// are checked by TestCheckDocsMirrorMetadatasOwnKeys above -- this is the read side of the same rule.
func TestMetadataDecodesStrictly(t *testing.T) {
	dir := filepath.Join(repoRoot(), "internal", "metadata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read internal/metadata: %v", err)
	}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		for i, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue // a comment may name the old shape while explaining it
			}
			if strings.Contains(line, "yaml.Unmarshal(") {
				t.Errorf("internal/metadata/%s:%d decodes leniently:\n\t%s\n"+
					"use decodeStrict, which rejects a key this runtime has no home for. A lenient decode "+
					"drops a misspelled or retired key in silence, and the file then loads clean while the "+
					"capability it was meant to declare never appears (005 Phase 3)",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no source files were scanned -- this gate would pass an empty tree silently")
	}
}
