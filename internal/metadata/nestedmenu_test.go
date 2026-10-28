package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

// A menu has one level under an item: a parent is another item of the list that has no parent itself (which also rules out a cycle), never the item itself,
// and a settings hub is not nested.
func TestValidateNavigationAllowsOneLevelUnderAnItem(t *testing.T) {
	item := func(id, parent string) domain.NavigationItem {
		return domain.NavigationItem{ID: id, Label: id, Route: "/" + id, Parent: parent}
	}
	if issues := validateNavigation([]domain.NavigationItem{item("nav_a", ""), item("nav_b", "nav_a"), item("nav_c", "nav_a")}); len(issues) != 0 {
		t.Errorf("one level was refused: %v", issues)
	}
	hub := item("nav_hub", "")
	hub.SettingsHub = true
	for name, tc := range map[string]struct {
		items []domain.NavigationItem
		want  string
	}{
		"the item itself":     {[]domain.NavigationItem{item("nav_a", "nav_a")}, "names the item itself"},
		"an item not there":   {[]domain.NavigationItem{item("nav_a", "nav_gone")}, "is not an item of this list"},
		"two levels":          {[]domain.NavigationItem{item("nav_a", ""), item("nav_b", "nav_a"), item("nav_c", "nav_b")}, "one level under an item"},
		"a cycle":             {[]domain.NavigationItem{item("nav_a", "nav_b"), item("nav_b", "nav_a")}, "one level under an item"},
		"a hub as the parent": {[]domain.NavigationItem{hub, item("nav_a", "nav_hub")}, "settings hub"},
	} {
		issues := strings.Join(validateNavigation(tc.items), "\n")
		if !strings.Contains(issues, tc.want) {
			t.Errorf("%s: issues %q lack %q", name, issues, tc.want)
		}
	}
}

// show_when holds a Field to what its form assumes: it watches another status Field of its Machine and one of that Field's options, one step and no chain,
// and it is not beside required_when.
func TestValidateShowWhen(t *testing.T) {
	status := func(id string, sw *domain.FieldCondition) domain.Field {
		return domain.Field{ID: id, Name: id, Type: domain.FieldTypeStatus, Options: []string{"A", "B"}, ShowWhen: sw}
	}
	byID := func(fs ...domain.Field) map[string]domain.Field {
		out := map[string]domain.Field{}
		for _, f := range fs {
			out[f.ID] = f
		}
		return out
	}
	watched := status("fld_kind", nil)
	ok := domain.Field{ID: "fld_ref", Type: domain.FieldTypeText, ShowWhen: &domain.FieldCondition{Field: "fld_kind", Equals: "A"}}
	if issues := validateShowWhen(ok, byID(watched, ok)); len(issues) != 0 {
		t.Errorf("a sound show_when was refused: %v", issues)
	}
	chained := status("fld_mid", &domain.FieldCondition{Field: "fld_kind", Equals: "A"})
	for name, tc := range map[string]struct {
		f    domain.Field
		all  map[string]domain.Field
		want string
	}{
		"itself":              {domain.Field{ID: "fld_x", ShowWhen: &domain.FieldCondition{Field: "fld_x", Equals: "A"}}, byID(), "names itself"},
		"a Field not there":   {domain.Field{ID: "fld_x", ShowWhen: &domain.FieldCondition{Field: "fld_gone", Equals: "A"}}, byID(), "is not a field of this machine"},
		"a text Field":        {domain.Field{ID: "fld_x", ShowWhen: &domain.FieldCondition{Field: "fld_t", Equals: "A"}}, byID(domain.Field{ID: "fld_t", Type: domain.FieldTypeText}), "must be a status field"},
		"an option not there": {domain.Field{ID: "fld_x", ShowWhen: &domain.FieldCondition{Field: "fld_kind", Equals: "Z"}}, byID(watched), "would never be shown"},
		"a chain":             {domain.Field{ID: "fld_x", ShowWhen: &domain.FieldCondition{Field: "fld_mid", Equals: "A"}}, byID(watched, chained), "one step, no chains"},
		"with required_when":  {domain.Field{ID: "fld_x", RequiredWhen: &domain.FieldCondition{Field: "fld_kind", Equals: "A"}, ShowWhen: &domain.FieldCondition{Field: "fld_kind", Equals: "A"}}, byID(watched), "show_when and required_when"},
	} {
		got := strings.Join(validateShowWhen(tc.f, tc.all), "\n")
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: issues %q lack %q", name, got, tc.want)
		}
	}
}
