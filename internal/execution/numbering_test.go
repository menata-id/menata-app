package execution

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// TestAssignNumbersGivesConcurrentCreatesDistinctNumbers: two creates of a numbered Field that run together queue on its lock,
// so each reads what the other committed -- twenty at once come out as 1..20, none twice.
func TestAssignNumbersGivesConcurrentCreatesDistinctNumbers(t *testing.T) {
	pool := testPool(t)
	const machineID = "mch_numbering_test"
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id = $1`, machineID) })
	store := data.NewStore(pool)
	ctx := data.WithWorkspaceScope(context.Background(), "ws_numbering_test")
	m := &domain.Machine{ID: machineID, Fields: []domain.Field{
		{ID: "fld_kind", Type: domain.FieldTypeText},
		{ID: "fld_no", Type: domain.FieldTypeText, Numbering: &domain.FieldNumbering{Pattern: "N/{fld_kind}/{seq:3}"}},
	}}
	const n = 20
	var wg sync.WaitGroup
	numbers := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- store.InTx(ctx, func(ctx context.Context) error {
				values := map[string]any{"fld_kind": "A", "fld_no": "typed-by-a-client"}
				if err := AssignNumbers(ctx, store, m, values, time.Now()); err != nil {
					return err
				}
				if _, err := store.CreateRecord(ctx, machineID, values); err != nil {
					return err
				}
				numbers <- fmt.Sprint(values["fld_no"])
				return nil
			})
		}()
	}
	wg.Wait()
	close(numbers)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for no := range numbers {
		got = append(got, no)
	}
	sort.Strings(got)
	for i, no := range got {
		if want := fmt.Sprintf("N/A/%03d", i+1); no != want {
			t.Fatalf("number %d is %q, want %q (all: %v)", i+1, no, want, got)
		}
	}
	if len(got) != n {
		t.Fatalf("%d numbers for %d creates", len(got), n)
	}
}
