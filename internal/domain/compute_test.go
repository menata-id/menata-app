package domain

import (
	"math"
	"testing"
)

// TestComputeEdgesTheReviewFound (the review of step 3): a value that is not a number is no value (it could not be
// stored); a figure stored as text is still a figure; a month added to the 31st lands on the month's last day; a count
// of months must be whole.
func TestComputeEdgesTheReviewFound(t *testing.T) {
	big := 1e200
	f := func(e ComputeExpr) FieldCompute { return FieldCompute{Op: e.Op, Expr: &e} }
	field := func(id string) ComputeExpr { return ComputeExpr{Field: id} }
	num := func(n float64) ComputeExpr { return ComputeExpr{Number: &n} }
	if v, ok := f(ComputeExpr{Op: ComputeMultiply, Args: []ComputeExpr{field("a"), field("a")}}).Evaluate(map[string]any{"a": big}); ok {
		t.Errorf("an overflowing product gave %v; want no value", v)
	}
	if v, ok := f(ComputeExpr{Op: ComputeSubtract, Args: []ComputeExpr{field("a"), field("b")}}).Evaluate(map[string]any{"a": 500.0, "b": "100"}); !ok || v != 400.0 {
		t.Errorf("a figure stored as text: %v %v, want 400", v, ok)
	}
	months := func(date string, n float64) (any, bool) {
		return f(ComputeExpr{Op: ComputeAddMonths, Args: []ComputeExpr{field("d"), num(n)}}).Evaluate(map[string]any{"d": date})
	}
	for _, c := range []struct {
		date string
		n    float64
		want string
	}{{"2026-01-31", 1, "2026-02-28"}, {"2028-01-31", 1, "2028-02-29"}, {"2026-03-31", -1, "2026-02-28"}, {"2026-01-15", 13, "2027-02-15"}} {
		if v, ok := months(c.date, c.n); !ok || v != c.want {
			t.Errorf("%s + %v months = %v, want %s", c.date, c.n, v, c.want)
		}
	}
	if v, ok := months("2026-01-15", 1.9); ok {
		t.Errorf("1.9 months gave %v; a count of months is whole", v)
	}
	if v, ok := months("2026-01-15", 1e19); ok {
		t.Errorf("1e19 months gave %v", v)
	}
	if n, ok := FigureOf(math.NaN()); ok {
		t.Errorf("NaN read as the figure %v", n)
	}
}
