package expression

import "testing"

func TestComparison_Evaluate(t *testing.T) {
	cases := []struct {
		name string
		c    Comparison
		vals map[string]any
		want bool
	}{
		{"equals match", Comparison{Field: "fld_status", Op: OpEquals, Value: "done"}, map[string]any{"fld_status": "done"}, true},
		{"equals mismatch", Comparison{Field: "fld_status", Op: OpEquals, Value: "done"}, map[string]any{"fld_status": "todo"}, false},
		{"not_equals match", Comparison{Field: "fld_status", Op: OpNotEquals, Value: "done"}, map[string]any{"fld_status": "todo"}, true},
		{"not_equals mismatch", Comparison{Field: "fld_status", Op: OpNotEquals, Value: "done"}, map[string]any{"fld_status": "done"}, false},
		{"missing field treated as empty string", Comparison{Field: "fld_status", Op: OpNotEquals, Value: "done"}, map[string]any{}, true},
		{"unknown op is always false", Comparison{Field: "fld_status", Op: "greater_than", Value: "1"}, map[string]any{"fld_status": "2"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.c.Evaluate(c.vals); got != c.want {
				t.Errorf("Evaluate() = %v, want %v", got, c.want)
			}
		})
	}
}
