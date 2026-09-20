package behavior

import (
	"testing"

	"menata.app/internal/domain"
)

// approvalRollup is the shape metadata/approval_step.yaml declares: any rejection decides the
// Document immediately, every step approved approves it, anything else leaves it in review.
func approvalRollup() domain.Rollup {
	return domain.Rollup{
		ParentField: "fld_document",
		TargetField: "fld_status",
		AnyValue:    "rejected", AnySet: "rejected",
		AllValue: "approved", AllSet: "approved",
		Default: "in_review",
	}
}

func TestRollupValue(t *testing.T) {
	for _, tc := range []struct {
		name     string
		children []string
		want     string
	}{
		{"no children yet", nil, "in_review"},
		{"all still pending", []string{"pending", "pending"}, "in_review"},
		{"some approved, some pending", []string{"approved", "pending"}, "in_review"},
		{"every step approved", []string{"approved", "approved"}, "approved"},
		{"one rejection", []string{"pending", "rejected", "pending"}, "rejected"},
		// The priority rule, stated as a test: a rejection decides the parent even while other
		// steps are still approved, and even if it is the last one read.
		{"rejection wins over approvals", []string{"approved", "approved", "rejected"}, "rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RollupValue(approvalRollup(), tc.children); got != tc.want {
				t.Errorf("RollupValue(%v) = %q, want %q", tc.children, got, tc.want)
			}
		})
	}
}

// TestRollupValue_oneArmOnly pins the "empty arm is disabled, not matching-the-empty-value"
// contract: a rollup declaring only `all` must never fire the `any` branch just because a child
// holds no value for the watched field.
func TestRollupValue_oneArmOnly(t *testing.T) {
	r := domain.Rollup{AllValue: "done", AllSet: "closed", Default: "open"}

	if got := RollupValue(r, []string{"", ""}); got != "open" {
		t.Errorf("children with no value = %q, want the default %q", got, "open")
	}
	if got := RollupValue(r, []string{"done", "done"}); got != "closed" {
		t.Errorf("every child done = %q, want %q", got, "closed")
	}
}
