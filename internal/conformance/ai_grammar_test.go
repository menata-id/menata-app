package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"menata.app/internal/aiassist"
	"menata.app/internal/registry"
)

// The assistant's output schema is a hand-written mirror of the loader's grammar (D2 = option G, K16): the
// loader accepts keys the assistant has no slot for, and nothing said which. This gate measures the
// difference and freezes it, so a key added to `machineDoc`/`fieldDoc` is either given a slot or listed here
// with the reason it has none -- the same terms every named population in this package carries. It reads
// the loader's struct tags from source (the docs are unexported) and the assistant's from reflection.
//
// It measures *presence of a slot*, not that the slot is right: a key can be writable and still be written
// wrongly, which `aiassist.Validate` and the loader are for.

// aiRenamedKeys: a key the assistant spells differently from the loader.
var aiRenamedKeys = map[string]map[string]string{
	"fieldDoc": {"machine": "related_machine"},
	// `then` is a block in the loader; the assistant flattens it into service/summary/recipient_field/preference_key.
	"eventDoc": {"then": "service"},
}

// The reasons the assistant has no slot for a key live in the capability catalogue
// (registry.Capabilities, KT1), which replaced the map that used to sit here. This gate holds the catalogue to both
// the loader (every key is catalogued exactly once; a proposed key is not read yet) and the assistant's schema
// (AIWritable is true exactly when a slot exists), so neither can be claimed.

func TestAIGrammarGapIsMeasured(t *testing.T) {
	pairs := []struct {
		doc string
		gen reflect.Type
	}{
		{"machineDoc", reflect.TypeOf(aiassist.GeneratedMachine{})},
		{"fieldDoc", reflect.TypeOf(aiassist.GeneratedField{})},
		{"transitionDoc", reflect.TypeOf(aiassist.GeneratedTransition{})},
		{"permissionDoc", reflect.TypeOf(aiassist.GeneratedPermission{})},
		{"parentActorDoc", reflect.TypeOf(aiassist.GeneratedParentActor{})},
		{"eventDoc", reflect.TypeOf(aiassist.GeneratedEvent{})},
		{"viewDoc", reflect.TypeOf(aiassist.GeneratedView{})},
		{"cardFieldDoc", reflect.TypeOf(aiassist.GeneratedCardField{})},
		{"completionDoc", reflect.TypeOf(aiassist.GeneratedCompletion{})},
		{"computeDoc", reflect.TypeOf(aiassist.GeneratedCompute{})},
	}
	loader := loaderYAMLKeys(t)
	for _, p := range pairs {
		keys, ok := loader[p.doc]
		if !ok || len(keys) == 0 {
			t.Fatalf("found no yaml keys on %s -- this gate would pass by measuring nothing", p.doc)
		}
		slots := map[string]bool{}
		for i := 0; i < p.gen.NumField(); i++ {
			slots[strings.Split(p.gen.Field(i).Tag.Get("json"), ",")[0]] = true
		}
		supported := map[string]registry.Capability{}
		for _, c := range registry.Capabilities {
			if c.Struct != p.doc {
				continue
			}
			for _, k := range c.Keys {
				if c.Stage == registry.StageProposed {
					for _, lk := range keys {
						if lk == k {
							t.Errorf("%s.%s is proposed in the catalogue but the loader reads it now -- mark %s supported", p.doc, k, c.ID)
						}
					}
					continue
				}
				supported[k] = c
			}
		}
		inLoader := map[string]bool{}
		for _, k := range keys {
			inLoader[k] = true
			c, ok := supported[k]
			if !ok {
				t.Errorf("%s key %q is in no catalogue entry -- add one to registry.Capabilities", p.doc, k)
				continue
			}
			slot := k
			if r, ok := aiRenamedKeys[p.doc][k]; ok {
				slot = r
			}
			switch {
			case slots[slot] && !c.AIWritable:
				t.Errorf("%s key %q now has an assistant slot but %s says it has none -- set AIWritable and drop the reason", p.doc, k, c.ID)
			case !slots[slot] && c.AIWritable:
				t.Errorf("%s key %q is catalogued as writable but %s has no slot for it", p.doc, k, p.gen.Name())
			}
		}
		for k, c := range supported {
			if !inLoader[k] {
				t.Errorf("catalogue entry %s lists %q, which the loader no longer has -- remove it", c.ID, k)
			}
		}
	}
}

// loaderYAMLKeys returns the yaml tag names of each struct in internal/metadata/parse.go, by AST.
func loaderYAMLKeys(t *testing.T) map[string][]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repoRoot(), "internal", "metadata", "parse.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range gd.Specs {
			ts, ok := s.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, fl := range st.Fields.List {
				if fl.Tag == nil {
					continue
				}
				tag := reflect.StructTag(strings.Trim(fl.Tag.Value, "`")).Get("yaml")
				if name := strings.Split(tag, ",")[0]; name != "" && name != "-" {
					out[ts.Name.Name] = append(out[ts.Name.Name], name)
				}
			}
		}
	}
	return out
}

// unwritableReason is the catalogue's reason the assistant has no slot for a loader key, or "" when it has one
// (or the key is not catalogued).
func unwritableReason(doc, key string) string {
	for _, c := range registry.Capabilities {
		if c.Struct != doc || c.Stage != registry.StageSupported || c.AIWritable {
			continue
		}
		for _, k := range c.Keys {
			if k == key {
				return c.AIReason
			}
		}
	}
	return ""
}
