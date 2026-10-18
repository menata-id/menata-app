package composition

import (
	"fmt"
	"testing"

	"menata.app/internal/domain"
)

// TestAQuestionCarriesOnlyABoundedListOfChoices: a relation's related records' labels go to the model as the allowed answers,
// so a relation with more records than the bound is asked as words (and matched to a record by label afterwards).
func TestAQuestionCarriesOnlyABoundedListOfChoices(t *testing.T) {
	choices := func(n int) []domain.FormChoice {
		out := make([]domain.FormChoice, n)
		for i := range out {
			out[i] = domain.FormChoice{Value: fmt.Sprint("rec_", i), Label: fmt.Sprint("Label ", i)}
		}
		return out
	}
	small := question(domain.FormInput{FieldID: "fld_c", Label: "Category", Kind: domain.InputSelect, Reference: "mch_c", Choices: choices(maxChoicesAsked)})
	if small.Type != "choice" || len(small.Options) != maxChoicesAsked {
		t.Errorf("a relation within the bound: %+v", small.Type)
	}
	big := question(domain.FormInput{FieldID: "fld_c", Label: "Category", Kind: domain.InputSelect, Reference: "mch_c", Choices: choices(maxChoicesAsked + 1)})
	if big.Type != "text" || len(big.Options) != 0 {
		t.Errorf("a relation over the bound sent %d labels as %q", len(big.Options), big.Type)
	}
	if q := question(domain.FormInput{FieldID: "fld_k", Kind: domain.InputSelect, Options: []string{"A", "B"}}); q.Type != "choice" || len(q.Options) != 2 {
		t.Errorf("a status: %+v", q)
	}
	if q := question(domain.FormInput{FieldID: "fld_n", Kind: domain.InputNumber}); q.Type != "number" {
		t.Errorf("a number: %+v", q)
	}
}
