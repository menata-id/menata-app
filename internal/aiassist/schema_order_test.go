package aiassist

import (
	"slices"
	"sort"
	"testing"
)

// TestEveryObjectSchemaDeclaresItsPropertyOrder: without propertyOrdering Gemini writes an object's
// properties alphabetically and cannot go back, which is how an extend_application change came out
// with its additions stuffed into the target_app_id string (2026-09-30). Every object with more than
// one property declares the order, and the order names exactly its properties.
func TestEveryObjectSchemaDeclaresItsPropertyOrder(t *testing.T) {
	var walk func(path string, s geminiSchema)
	walk = func(path string, s geminiSchema) {
		if s.Items != nil {
			walk(path+"[]", *s.Items)
		}
		if len(s.Properties) == 0 {
			return
		}
		keys := make([]string, 0, len(s.Properties))
		for k := range s.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ordered := slices.Clone(s.PropertyOrdering)
		sort.Strings(ordered)
		if len(keys) > 1 && !slices.Equal(keys, ordered) {
			t.Errorf("%s: propertyOrdering %v does not name exactly its properties %v", path, s.PropertyOrdering, keys)
		}
		for k, child := range s.Properties {
			walk(path+"."+k, child)
		}
	}
	walk("reply", replySchema)
}

// TestReplyWritesItsMessageFirstAndItsTargetBeforeItsAdditions pins the two orders the incident was
// about, so a later edit cannot satisfy the gate above with the wrong order.
func TestReplyWritesItsMessageFirstAndItsTargetBeforeItsAdditions(t *testing.T) {
	if replySchema.PropertyOrdering[0] != "message" {
		t.Errorf("reply order = %v, want message first", replySchema.PropertyOrdering)
	}
	o := generatedChangeSchema.PropertyOrdering
	if slices.Index(o, "kind") > slices.Index(o, "additions") || slices.Index(o, "target_app_id") > slices.Index(o, "additions") {
		t.Errorf("change order = %v, want kind and target_app_id before additions", o)
	}
}
