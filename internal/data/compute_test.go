package data

import (
	"net/url"
	"testing"

	"menata.app/internal/domain"
)

func waterMachine() *domain.Machine {
	return &domain.Machine{ID: "mch_water", Fields: []domain.Field{
		{ID: "fld_pdam", Type: domain.FieldTypeNumber},
		{ID: "fld_well", Type: domain.FieldTypeNumber},
		{ID: "fld_total", Type: domain.FieldTypeNumber, Compute: &domain.FieldCompute{Op: domain.ComputeSum, Fields: []string{"fld_pdam", "fld_well"}}},
	}}
}

func TestApplyComputed(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]any
		want   any
	}{
		{"both", map[string]any{"fld_pdam": 12.5, "fld_well": 3.0}, 15.5},
		{"one empty counts as zero", map[string]any{"fld_pdam": 4.0}, 4.0},
		{"nothing entered stays empty", map[string]any{}, nil},
		{"a submitted total is overwritten", map[string]any{"fld_pdam": 1.0, "fld_well": 1.0, "fld_total": 999.0}, 2.0},
		{"a stale total is removed", map[string]any{"fld_total": 7.0}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ApplyComputed(waterMachine(), tc.values)
			if got := tc.values["fld_total"]; got != tc.want {
				t.Errorf("fld_total = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestValuesFromForm_ignoresComputedFields: a computed Field is never read from a submission.
func TestValuesFromForm_ignoresComputedFields(t *testing.T) {
	values := ValuesFromForm(waterMachine(), url.Values{"fld_pdam": {"2"}, "fld_total": {"100"}})
	if _, ok := values["fld_total"]; ok {
		t.Errorf("a submitted computed value was read: %v", values)
	}
}
