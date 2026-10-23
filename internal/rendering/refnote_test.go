package rendering

import (
	"strings"
	"testing"

	"menata.app/internal/domain"
)

func TestEmptyReferenceNote(t *testing.T) {
	ctx := WithCurrentWorkspace(t.Context(), domain.Workspace{Machines: []*domain.Machine{{ID: "mch_list", Name: "List"}}}, "W", false)
	f := domain.Field{ID: "fld_l", Type: domain.FieldTypeRelation, RelatedMachine: "mch_list"}
	if got := emptyReferenceNote(ctx, f, RelationOptions{}, true); !strings.Contains(got, "No List exists yet") {
		t.Fatalf("required + empty: %q", got)
	}
	if got := emptyReferenceNote(ctx, f, RelationOptions{}, false); got != "" {
		t.Fatalf("optional: %q", got)
	}
	if got := emptyReferenceNote(ctx, f, RelationOptions{"mch_list": {{ID: "a", Label: "A"}}}, true); got != "" {
		t.Fatalf("has options: %q", got)
	}
}
