package aiassist

import (
	"slices"
	"testing"

	"menata.app/internal/domain"
)

// TestFieldTypeEnumIsTheCatalogue asserts the schema a generated Field is constrained by offers exactly the
// field types the prompt names, in the same order. The two were separate lists until 2026-10-10 (K02): the
// prompt said "long text" and the Enum, a literal, made long_text impossible to return.
func TestFieldTypeEnumIsTheCatalogue(t *testing.T) {
	got := generatedFieldSchema.Properties["type"].Enum
	var want []string
	for _, ft := range domain.FieldTypes() {
		want = append(want, string(ft))
	}
	if len(want) == 0 {
		t.Fatal("domain.FieldTypes is empty -- this test would pass by asserting nothing")
	}
	if !slices.Equal(got, want) {
		t.Errorf("generated Field type Enum = %v, want domain.FieldTypes %v", got, want)
	}
}
