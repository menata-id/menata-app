package composition

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// Each loader gets its own Machine id: cleanup runs at test end, so two loaders sharing an id would
// see each other's seeded rows. Caught by the memo test itself reporting 60 rows where it wanted 30.
func selectTestMachineID(t *testing.T, suffix string) string {
	t.Helper()
	return "mch_select_test_" + suffix
}

// selectTestLoader is a Loader over a real database, following internal/data/store_test.go's own
// pattern: skip without DATABASE_URL rather than fail, so `make test` stays DB-free by default.
func selectTestLoader(t *testing.T, suffix string, rows int) (*Loader, context.Context) {
	t.Helper()
	machineID := selectTestMachineID(t, suffix)
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run the select-dataset integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	store := data.NewStore(pool)
	ctx := data.WithWorkspaceScope(context.Background(), "ws_select_test")
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id = $1`, machineID); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	for i := range rows {
		if _, err := store.CreateRecord(ctx, machineID, map[string]any{"fld_name": fmt.Sprintf("row-%02d", i)}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	machine := &domain.Machine{
		ID:     machineID,
		Fields: []domain.Field{{ID: "fld_name", Type: domain.FieldTypeText}},
		Datasets: []domain.Dataset{{
			ID: "ds_select_test", Source: machineID, Select: domain.SelectRecords,
			Sort:  []domain.SortKey{{Field: "sort_order", Direction: domain.SortDescending}},
			Limit: 10,
		}},
	}
	return NewLoader(store, map[string]*domain.Machine{machineID: machine}), ctx
}

// TestSelectDataset_doesNotPoisonTheWholeMachineMemo is the most important test in this slice, and
// the reason the memo keys on the Dataset id rather than the Machine id.
//
// A `select: records` result is a *different set of rows for the same Machine* -- ordered and
// bounded. Memoising it under machineID would make the next whole-Machine reader in the same request
// receive ten rows where it needs all thirty. **The failure would be silent and would break a
// different screen than the one being changed**: /dashboard reads ds_recent_activity (10 rows) and
// would then hand every later mch_activity reader that same truncated slice.
func TestSelectDataset_doesNotPoisonTheWholeMachineMemo(t *testing.T) {
	l, ctx := selectTestLoader(t, "memo", 30)
	machineID := selectTestMachineID(t, "memo")

	selected, err := l.SelectDataset(ctx, "ds_select_test")
	if err != nil {
		t.Fatalf("SelectDataset: %v", err)
	}
	if len(selected) != 10 {
		t.Fatalf("SelectDataset returned %d rows, want the declared limit of 10", len(selected))
	}
	// The declared *direction* has to reach the query too, and until this assertion existed nothing
	// checked it: internal/data's own test injects a data.SortKey directly, so the mapping from a
	// declared `direction: desc` through domain.SortKey.Descending into the ORDER BY was covered by
	// nothing. Found by mutation -- making Descending() always return false failed no test.
	if got := selected[0].Values["fld_name"]; got != "row-29" {
		t.Errorf("first selected row is %v, want row-29: `direction: desc` did not reach the query", got)
	}

	// Same Machine, same request, whole-Machine read. It must see everything.
	all, err := l.ListRecords(ctx, machineID)
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(all) != 30 {
		t.Errorf("ListRecords returned %d rows after a selection of 10 -- the selection was memoised "+
			"under the Machine id and poisoned the whole-Machine memo. A screen reading the same "+
			"Machine in the same request now silently sees a truncated set.", len(all))
	}

	// And the other order, because a memo can be poisoned from either direction.
	l2, ctx2 := selectTestLoader(t, "memo2", 30)
	machineID2 := selectTestMachineID(t, "memo2")
	if first, err := l2.ListRecords(ctx2, machineID2); err != nil || len(first) != 30 {
		t.Fatalf("ListRecords first = %d rows (err %v), want 30", len(first), err)
	}
	after, err := l2.SelectDataset(ctx2, "ds_select_test")
	if err != nil {
		t.Fatalf("SelectDataset after ListRecords: %v", err)
	}
	if len(after) != 10 {
		t.Errorf("SelectDataset returned %d rows after a whole-Machine read -- it was served the "+
			"Machine's memo instead of issuing its own bounded query", len(after))
	}
}

// TestSelectDataset_readsEachDatasetOnce: the memo's ordinary job. Two selections of the same Dataset
// in one request are one read, the same guarantee ListRecords gives, and what keeps the GET sweep's
// repeated==0 invariant true for a screen that composes the same feed twice.
func TestSelectDataset_readsEachDatasetOnce(t *testing.T) {
	l, ctx := selectTestLoader(t, "once", 12)

	for range 3 {
		if _, err := l.SelectDataset(ctx, "ds_select_test"); err != nil {
			t.Fatalf("SelectDataset: %v", err)
		}
	}
	if l.reads != 1 {
		t.Errorf("three selections of one Dataset issued %d reads, want 1", l.reads)
	}
	if l.served != 2 {
		t.Errorf("served-from-memo = %d, want 2", l.served)
	}
}

// TestSelectDataset_refusesAnAggregatingDataset: the two modes answer different questions, and a
// caller that asked for rows and silently received every row of the Machine is the unbounded
// retrieval this whole path exists to replace.
func TestSelectDataset_refusesAnAggregatingDataset(t *testing.T) {
	l, ctx := selectTestLoader(t, "refuse", 1)
	machineID := selectTestMachineID(t, "refuse")
	l.machines[machineID].Datasets = append(l.machines[machineID].Datasets, domain.Dataset{
		ID: "ds_counts", Source: machineID, Measures: []domain.Measure{{ID: "msr_total", Aggregate: domain.AggregateCount}},
	})

	if _, err := l.SelectDataset(ctx, "ds_counts"); err == nil {
		t.Error("SelectDataset accepted an aggregating Dataset -- it must refuse rather than fall back " +
			"to reading the Machine whole")
	}
}
