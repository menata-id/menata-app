package metadata

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
)

func viewerMachine(ds domain.Dataset) *domain.Machine {
	ds.ID, ds.Source = "ds_mine", "mch_x"
	return Normalize(&domain.Machine{
		ID: "mch_x", Name: "X",
		Fields: []domain.Field{
			{ID: "fld_title", Name: "Title", Type: domain.FieldTypeText},
			{ID: "fld_owner", Name: "Owner", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
			{ID: "fld_other", Name: "Other", Type: domain.FieldTypePerson, RelatedMachine: domain.UserMachineID},
		},
		Datasets: []domain.Dataset{ds},
	})
}

func mineWhere(field string, op expression.Op, value string) *expression.Predicate {
	return &expression.Predicate{All: []expression.Comparison{{Field: field, Op: op, Value: value}}}
}

// TestViewerScoped_theLoaderMakesTheClaimTrue: a Dataset that says it lists the viewer's own records must carry
// a top-level `equals $current_user` on a person field, or it does not load. W5-0, 2026-10-15.
func TestViewerScoped_theLoaderMakesTheClaimTrue(t *testing.T) {
	cases := map[string]struct {
		ds   domain.Dataset
		want string // "" = loads
	}{
		"flagged with the predicate loads": {domain.Dataset{Select: domain.SelectRecords, Limit: 50, ViewerScoped: true,
			Where: mineWhere("fld_owner", expression.OpEquals, expression.SentinelCurrentUser)}, ""},
		"unflagged with no predicate loads": {domain.Dataset{Select: domain.SelectRecords, Limit: 50}, ""},
		"unflagged with the predicate loads": {domain.Dataset{Select: domain.SelectRecords, Limit: 50,
			Where: mineWhere("fld_owner", expression.OpEquals, expression.SentinelCurrentUser)}, ""},
		"flagged with no where": {domain.Dataset{Select: domain.SelectRecords, Limit: 50, ViewerScoped: true}, "no top-level"},
		"flagged with a literal": {domain.Dataset{Select: domain.SelectRecords, Limit: 50, ViewerScoped: true,
			Where: mineWhere("fld_owner", expression.OpEquals, "usr_1")}, "no top-level"},
		"flagged but inverted": {domain.Dataset{Select: domain.SelectRecords, Limit: 50, ViewerScoped: true,
			Where: mineWhere("fld_owner", expression.OpNotEquals, expression.SentinelCurrentUser)}, "no top-level"},
		"flagged but only inside any": {domain.Dataset{Select: domain.SelectRecords, Limit: 50, ViewerScoped: true,
			Where: &expression.Predicate{Any: []expression.Comparison{{Field: "fld_owner", Op: expression.OpEquals, Value: expression.SentinelCurrentUser}, {Field: "fld_other", Op: expression.OpEquals, Value: expression.SentinelCurrentUser}}}}, "no top-level"},
		"flagged over a text field": {domain.Dataset{Select: domain.SelectRecords, Limit: 50, ViewerScoped: true,
			Where: mineWhere("fld_title", expression.OpEquals, expression.SentinelCurrentUser)}, "must be a person field"},
		"flagged on an aggregate": {domain.Dataset{ViewerScoped: true, Dimension: "fld_owner",
			Measures: []domain.Measure{{ID: "n", Aggregate: domain.AggregateCount}}}, "aggregating Dataset"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(viewerMachine(tc.ds))
			if tc.want == "" {
				if err != nil {
					t.Fatalf("a valid Dataset was refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want a refusal saying %q, got %v", tc.want, err)
			}
		})
	}
}
