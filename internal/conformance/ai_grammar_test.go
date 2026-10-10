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
}

// aiUnwritableKeys is the measured remainder, per loader struct. Shrink-only: a key that gains a slot
// fails until it leaves this map, and a key that does not exist in the loader fails too.
var aiUnwritableKeys = map[string]map[string]string{
	"machineDoc": {
		"constraints":           "block-if rules relate two Machines; the assistant has no related-record vocabulary yet",
		"actions":               "declares what an Action writes; only the approval engine's Machines use it",
		"datasets":              "selection is composed by Pages; a generated Machine takes the implicit table view",
		"sequencing":            "approval-engine ordering; generated Machines have no step Machine",
		"signature_placement":   "approval-engine role (step); not generatable",
		"signature_store":       "approval-engine role (signature); not generatable",
		"flow_template":         "approval-engine role; not generatable",
		"flow_template_step":    "approval-engine role; not generatable",
		"blocks_member_removal": "no generated Machine is a member-removal blocker yet",
		"sla_field":             "SLA badge; no forcing case in a generated Application",
		"completion":            "done/reopen over a status; no forcing case in a generated Application",
		"card_tags":             "board card tags; no generated board",
		"card_fields":           "board card projection; no generated board",
		"views":                 "every generated Machine gets the implicit table view",
		"append_only":           "no generated Machine is a log",
	},
	"fieldDoc": {
		"default": "a generated Field has no default value slot",
		"stamp":   "stamped Fields (created/updated by) are runtime-set; not generatable",
	},
}

func TestAIGrammarGapIsMeasured(t *testing.T) {
	pairs := []struct {
		doc string
		gen reflect.Type
	}{
		{"machineDoc", reflect.TypeOf(aiassist.GeneratedMachine{})},
		{"fieldDoc", reflect.TypeOf(aiassist.GeneratedField{})},
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
		listed := aiUnwritableKeys[p.doc]
		inLoader := map[string]bool{}
		for _, k := range keys {
			inLoader[k] = true
			slot := k
			if r, ok := aiRenamedKeys[p.doc][k]; ok {
				slot = r
			}
			switch {
			case slots[slot] && listed[k] != "":
				t.Errorf("%s key %q now has an assistant slot but is still listed as unwritable -- remove the entry", p.doc, k)
			case !slots[slot] && listed[k] == "":
				t.Errorf("%s key %q has no slot in %s and no entry in aiUnwritableKeys -- give the assistant a slot (schema.go, spec.go, writer.go) or list it with the reason it has none", p.doc, k, p.gen.Name())
			}
		}
		for k := range listed {
			if !inLoader[k] {
				t.Errorf("aiUnwritableKeys[%s] lists %q, which the loader no longer has -- remove the entry", p.doc, k)
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
