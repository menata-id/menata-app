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

// TestSchemaEnumsAreTheCatalogues holds every enum the schema offers to the registry it restricts (K16): the
// colours to domain.KnownApplicationColors, and the Permission actions to a subset of domain.KnownActions --
// a subset on purpose (the prompt explains why `decide` and `revise` are not generable), so the test asserts
// membership rather than equality and names the excluded ones.
func TestSchemaEnumsAreTheCatalogues(t *testing.T) {
	colors := generatedApplicationSchema.Properties["color"].Enum
	if len(colors) != len(domain.KnownApplicationColors) {
		t.Errorf("color enum %v does not match domain.KnownApplicationColors (%d)", colors, len(domain.KnownApplicationColors))
	}
	for _, c := range colors {
		if !domain.KnownApplicationColors[c] {
			t.Errorf("color %q is offered to the model but is not a known application color", c)
		}
	}

	offered := generatedPermissionSchema.Properties["action"].Enum
	for _, a := range offered {
		if !domain.KnownActions[a] {
			t.Errorf("permission action %q is offered to the model but is not in domain.KnownActions", a)
		}
	}
	for _, withheld := range []string{domain.ActionDecide, domain.ActionRevise} {
		if slices.Contains(offered, withheld) {
			t.Errorf("%q is a hardcoded workflow action and must not be offered to the model", withheld)
		}
	}
}
