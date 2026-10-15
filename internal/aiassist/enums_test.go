package aiassist

import (
	"slices"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/registry"
)

// Every value the assistant's schema lists by hand must be one the loader accepts, or the model is offered a word
// the runtime then refuses (K16: the schema is derived from the loader's closed sets where it can be, and held to
// them where a subset is meant -- the assistant deliberately offers fewer actions, services, view types and card
// roles than the loader knows).
func TestHandListedEnumsAreMembersOfTheLoadersSets(t *testing.T) {
	enum := func(s geminiSchema, prop string) []string {
		t.Helper()
		p, ok := s.Properties[prop]
		if !ok || len(p.Enum) == 0 {
			t.Fatalf("schema has no enum on %q -- this gate would measure nothing", prop)
		}
		return p.Enum
	}
	checks := []struct {
		what   string
		values []string
		known  func(string) bool
	}{
		{"a permission's action", enum(generatedPermissionSchema, "action"), func(v string) bool { return domain.KnownActions[v] }},
		{"an event's service", enum(generatedEventSchema, "service"), func(v string) bool { _, ok := registry.Services[v]; return ok }},
		{"a view's type", GeneratableViewTypes, func(v string) bool { return domain.KnownViewKinds[domain.ViewKind(v)] }},
		{"a card field's role", GeneratableCardRoles, func(v string) bool { return domain.KnownCardFieldRoles[domain.CardFieldRole(v)] }},
		{"a list's direction", enum(generatedListSchema, "direction"), func(v string) bool { return domain.KnownSortDirections[v] }},
	}
	for _, c := range checks {
		for _, v := range c.values {
			if !c.known(v) {
				t.Errorf("%s: the schema offers %q, which the loader does not accept", c.what, v)
			}
		}
	}
}

// A list's direction is read from the loader's set, not retyped: adding a direction there reaches the model.
func TestSortDirectionEnumIsTheLoadersSet(t *testing.T) {
	got := sortDirectionEnum()
	if len(got) != len(domain.KnownSortDirections) || !slices.IsSorted(got) {
		t.Fatalf("sortDirectionEnum = %v, want the loader's %d directions, sorted", got, len(domain.KnownSortDirections))
	}
	for _, d := range got {
		if !domain.KnownSortDirections[d] {
			t.Errorf("%q is not a loader direction", d)
		}
	}
}
