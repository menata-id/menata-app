package web

import (
	"regexp"
	"strings"
	"testing"

	"menata.app/internal/execution"
)

// TestTimeCardListsEveryInstalmentOfOneContractWithItsPaymentState is the red case drawn from mockup K01h (Time Card): one
// contract of five instalments, two of them paid (each by a plan with a payment date), one invoiced (a plan still awaiting
// proof) and two still scheduled. The page `/pages/nav_timecard?contract=<id>` must show the contract's identity and all
// five instalments, the payment date of the paid ones and the number of the plan of the one in billing, and nothing of
// another contract. The contract's page and the plan's page both offer it, the plan's through the contract the plan's
// instalment belongs to, so a person opening a plan gets the Time Card of the whole contract and not of one instalment.
func TestTimeCardListsEveryInstalmentOfOneContractWithItsPaymentState(t *testing.T) {
	h, cookie, ctx, store, _, ws, _ := routerSetupFor(t, "timecard", "sample-contracts")

	cat, err := store.CreateRecord(ctx, "mch_category", map[string]any{"fld_cat_name": "Production Crew", "fld_cat_kind": "Department"})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	newContract := func(no, party string) string {
		rec, err := store.CreateRecord(ctx, "mch_contract", map[string]any{
			"fld_contract_no": no, "fld_party_name": party, "fld_contract_kind": "Crew", "fld_category": cat.ID,
			"fld_position": "Production Accountant", "fld_value": "144000000", "fld_start_date": "2026-06-17", "fld_status": "Active",
		})
		if err != nil {
			t.Fatalf("contract %s: %v", no, err)
		}
		return rec.ID
	}
	mine := newContract("CTR/K/914", "Nadia Rahma")
	other := newContract("CTR/K/915", "Someone Else")

	months := []string{"2026-06-24", "2026-07-24", "2026-08-25", "2026-09-24", "2026-10-23"}
	status := []string{"Paid", "Paid", "Invoiced", "Scheduled", "Scheduled"}
	terms := make([]string, len(months))
	for i := range months {
		rec, err := store.CreateRecord(ctx, "mch_contract_term", map[string]any{
			"fld_contract": mine, "fld_term_label": "Fee " + string(rune('A'+i)), "fld_gross": "16000000", "fld_wht_rate": "2",
			"fld_due_date": months[i], "fld_term_status": status[i],
		})
		if err != nil {
			t.Fatalf("term %d: %v", i, err)
		}
		terms[i] = rec.ID
	}
	if _, err := store.CreateRecord(ctx, "mch_contract_term", map[string]any{
		"fld_contract": other, "fld_term_label": "Foreign fee", "fld_gross": "1000000", "fld_due_date": "2026-11-01",
	}); err != nil {
		t.Fatalf("other term: %v", err)
	}
	newPlan := func(term, no, planStatus, paid string) string {
		v := map[string]any{"fld_term": term, "fld_plan_no": no, "fld_plan_amount": "15680000", "fld_plan_date": "2026-11-01", "fld_plan_status": planStatus}
		if paid != "" {
			v["fld_paid_date"] = paid
		}
		rec, err := store.CreateRecord(ctx, "mch_payment_plan", v)
		if err != nil {
			t.Fatalf("plan %s: %v", no, err)
		}
		return rec.ID
	}
	newPlan(terms[0], "PP-9001", "Paid", "2026-06-24")
	newPlan(terms[1], "PP-9002", "Paid", "2026-07-24")
	billing := newPlan(terms[2], "PP-9003", "Awaiting proof", "")

	// What a route would have written when each record was made: the inherited and added-up Fields (the plan's contract, the
	// instalments' net, the contract's paid figure), through the same recompute a Workspace load runs over stored records.
	if _, err := execution.RecomputeAll(ctx, store, ws.Machines); err != nil {
		t.Fatalf("RecomputeAll: %v", err)
	}

	card := getPage(t, h, cookie, "/pages/nav_timecard?contract="+mine)
	for _, want := range []string{"CTR/K/914", "Nadia Rahma", "Production Accountant", "Fee A", "Fee B", "Fee C", "Fee D", "Fee E", "Paid", "Invoiced", "Scheduled"} {
		if !strings.Contains(card, want) {
			t.Errorf("the Time Card does not show %q", want)
		}
	}
	if !strings.Contains(card, `class="sheet"`) || !strings.Contains(card, "TIMECARD") {
		t.Errorf("the Time Card is not drawn as a printed sheet (layout: sheet) with its TIMECARD plate")
	}
	// What each state adds up to, read from the contract's own figures: two paid, one invoiced, two scheduled of five.
	for _, want := range []string{">2<", ">1<", ">5<"} {
		if !strings.Contains(card, want) {
			t.Errorf("the summary of states lacks a figure %q", want)
		}
	}
	for _, leaked := range []string{"CTR/K/915", "Someone Else", "Foreign fee"} {
		if strings.Contains(card, leaked) {
			t.Errorf("the Time Card of one contract shows %q of another", leaked)
		}
	}
	if !regexp.MustCompile(`24 Jun`).MatchString(card) && !strings.Contains(card, "2026-06-24") {
		t.Errorf("the payment date of the first paid instalment (24 June 2026) is not on the page")
	}

	want := "/pages/nav_timecard?contract=" + mine
	if page := getPage(t, h, cookie, "/pages/nav_contract/"+mine); !strings.Contains(page, want) {
		t.Errorf("the contract's page does not link to its Time Card (%s)", want)
	}
	if page := getPage(t, h, cookie, "/pages/nav_plan/"+billing); !strings.Contains(page, want) {
		t.Errorf("the payment plan's page does not link to the Time Card of its instalment's contract (%s)", want)
	}
}
