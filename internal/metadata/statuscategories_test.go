package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func categorisedMachine(sc *domain.StatusCategories, c *domain.Completion) *domain.Machine {
	return Normalize(&domain.Machine{
		ID: "mch_x", Name: "X",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Options: []string{"draft", "open", "done", "dropped"}},
		},
		Completion:       c,
		StatusCategories: sc,
	})
}

func TestStatusCategories_validation(t *testing.T) {
	ok := map[string]domain.StatusCategory{"draft": domain.StatusStart, "open": domain.StatusActive, "done": domain.StatusDone, "dropped": domain.StatusStopped}
	cases := map[string]struct {
		sc   *domain.StatusCategories
		c    *domain.Completion
		want string // "" = valid
	}{
		"a full declaration loads":    {&domain.StatusCategories{Field: "fld_status", Values: ok}, nil, ""},
		"a partial declaration loads": {&domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{"done": domain.StatusDone}}, nil, ""},
		"none loads":                  {nil, nil, ""},
		"unknown field":               {&domain.StatusCategories{Field: "fld_nope", Values: ok}, nil, "is not a field"},
		"not a status field":          {&domain.StatusCategories{Field: "fld_title", Values: ok}, nil, "must be a status field"},
		"empty":                       {&domain.StatusCategories{Field: "fld_status"}, nil, "declares no values"},
		"option that does not exist":  {&domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{"ghost": domain.StatusDone}}, nil, "is not one of"},
		"category outside the set":    {&domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{"done": "finished"}}, nil, "unknown category"},
		"completion agrees":           {&domain.StatusCategories{Field: "fld_status", Values: ok}, &domain.Completion{Field: "fld_status", Done: "done"}, ""},
		"completion value not done":   {&domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{"done": domain.StatusActive}}, &domain.Completion{Field: "fld_status", Done: "done"}, "must be categorised done"},
		"a second option marked done": {&domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{"done": domain.StatusDone, "dropped": domain.StatusDone}}, &domain.Completion{Field: "fld_status", Done: "done"}, "completion says"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(categorisedMachine(tc.sc, tc.c))
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("want valid, got %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestStatusCategories_everyTransitionEndpointIsCategorised(t *testing.T) {
	m := categorisedMachine(&domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{"draft": domain.StatusStart, "open": domain.StatusActive}}, nil)
	m.Transitions = []domain.Transition{{ID: "trn_finish", Name: "Finish", Field: "fld_status", From: "open", To: "done"}}
	err := Validate(m)
	if err == nil || !strings.Contains(err.Error(), `reaches "done"`) {
		t.Fatalf("want an uncategorised endpoint refused, got %v", err)
	}
	m.StatusCategories.Values["done"] = domain.StatusDone
	if err := Validate(m); err != nil {
		t.Fatalf("categorising the endpoint should load: %v", err)
	}
}

func TestStatusCategories_parsedFromYAML(t *testing.T) {
	m, err := Parse([]byte(`id: mch_x
name: X
fields:
  - id: fld_status
    name: Status
    type: status
    options: [a, b]
status_categories:
  field: fld_status
  values: { a: start, b: done }
`))
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := m.StatusCategories.CategoryOf("b"); !ok || c != domain.StatusDone {
		t.Fatalf("b = %q, %v", c, ok)
	}
	if _, ok := m.StatusCategories.CategoryOf("zzz"); ok {
		t.Fatal("an undeclared option must not be categorised")
	}
	if got := strings.Join(m.DeclaredBlocks(), ","); !strings.Contains(got, "status_categories") {
		t.Fatalf("DeclaredBlocks = %s", got)
	}
}
