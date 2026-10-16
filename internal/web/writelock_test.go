package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

func TestWriteLockKeys(t *testing.T) {
	atMost := 1000.0
	voucher := &domain.Machine{ID: "mch_voucher", Bounds: []domain.AggregateBound{{ID: "b", SumOf: "fld_amount", Per: "fld_period", AtMost: &atMost}}}
	entry := &domain.Machine{ID: "mch_entry", Transitions: []domain.Transition{{ID: "post", Balanced: &domain.BalanceRule{Machine: "mch_line", Via: "fld_entry"}}}}
	line := &domain.Machine{ID: "mch_line"}
	loan := &domain.Machine{ID: "mch_loan", Transitions: []domain.Transition{{ID: "pay", Generates: &domain.GenerateSchedule{Machine: "mch_inst"}}}}
	plain := &domain.Machine{ID: "mch_plain"}
	machines := map[string]*domain.Machine{"mch_voucher": voucher, "mch_entry": entry, "mch_line": line, "mch_loan": loan, "mch_plain": plain}

	for name, tc := range map[string]struct {
		machine        *domain.Machine
		id             string
		existing, next map[string]any
		want           []string
	}{
		"a create under a bound":        {voucher, "", nil, map[string]any{"fld_period": "p1"}, []string{"ensure|ws|mch_voucher|fld_period|p1"}},
		"a move between groups":         {voucher, "v1", map[string]any{"fld_period": "p1"}, map[string]any{"fld_period": "p2"}, []string{"ensure|ws|mch_voucher|fld_period|p1", "ensure|ws|mch_voucher|fld_period|p2"}},
		"a record staying in its group": {voucher, "v1", map[string]any{"fld_period": "p1"}, map[string]any{"fld_period": "p1"}, []string{"ensure|ws|mch_voucher|fld_period|p1"}},
		"a bound with no group yet":     {voucher, "", nil, map[string]any{}, nil},
		"posting an entry":              {entry, "e1", nil, map[string]any{}, []string{"balanced|ws|e1"}},
		"a line added to an entry":      {line, "", nil, map[string]any{"fld_entry": "e1"}, []string{"balanced|ws|e1"}},
		"a line moved between entries":  {line, "l1", map[string]any{"fld_entry": "e1"}, map[string]any{"fld_entry": "e2"}, []string{"balanced|ws|e1", "balanced|ws|e2"}},
		"a line deleted":                {line, "l1", map[string]any{"fld_entry": "e1"}, nil, []string{"balanced|ws|e1"}},
		"a generating loan":             {loan, "ln1", nil, map[string]any{}, []string{"generates|ws|mch_loan|ln1"}},
		"a Machine with none of them":   {plain, "x", map[string]any{"fld_period": "p1"}, map[string]any{"fld_period": "p1"}, nil},
	} {
		// Repeated, because the keys come out of a map and an order that holds once holds by luck: two writers taking
		// the same locks in different orders deadlock.
		for run := 0; run < 30; run++ {
			got := writeLockKeys("ws", machines, tc.machine, tc.id, tc.existing, tc.next)
			if !reflect.DeepEqual(got, tc.want) && !(len(got) == 0 && len(tc.want) == 0) {
				t.Errorf("%s: keys = %v, want %v", name, got, tc.want)
				break
			}
		}
	}
}

// Two writers each add a voucher that fits the fund alone and not together. Without the lock both read the same total and
// both pass; with it the second waits, reads what the first committed, and is refused.
func TestWithWriteLockSerialisesAGroupBound(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Write Lock Test", "write-lock-test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, "write_lock_test@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	fund := 1000.0
	voucher := &domain.Machine{ID: "mch_voucher_lock", Name: "Voucher",
		Fields: []domain.Field{{ID: "fld_amount", Name: "Amount", Type: domain.FieldTypeNumber}, {ID: "fld_period", Name: "Period", Type: domain.FieldTypeText}},
		Bounds: []domain.AggregateBound{{ID: "b", SumOf: "fld_amount", Per: "fld_period", AtMost: &fund}}}
	machines := map[string]*domain.Machine{voucher.ID: voucher}

	const writers = 8
	var wg sync.WaitGroup
	codes := make([]int, writers)
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			values := map[string]any{"fld_amount": 600.0, "fld_period": "p1"}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(wsCtx)
			<-start
			ok := withWriteLock(rec, req, store, machines, voucher, "", values, func(req *http.Request) bool {
				if !withinBounds(rec, req, store, voucher, "", values) {
					return false
				}
				// The window between reading the total and writing the row, made wide enough that writers without a
				// lock all fall into it; with the lock they queue and each reads the previous one's row.
				time.Sleep(40 * time.Millisecond)
				_, err := store.CreateRecord(req.Context(), voucher.ID, values)
				return err == nil
			})
			codes[i] = rec.Code
			if ok {
				codes[i] = http.StatusCreated
			}
		}(i)
	}
	close(start)
	wg.Wait()
	created := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			created++
		}
	}
	rows, err := store.ListRecordsBy(wsCtx, voucher.ID, "fld_period", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 || len(rows) != 1 {
		t.Errorf("%d writers of 600 into a fund of 1000: %d reported created, %d stored, want exactly 1 (codes %v)", writers, created, len(rows), codes)
	}
}

// A refusal inside the transaction rolls the write back with it: nothing half done is left.
func TestWithWriteLockRollsBackARefusal(t *testing.T) {
	pool := authTestPool(t)
	store := data.NewStore(pool)
	ctx := context.Background()
	ws, err := store.CreateWorkspace(ctx, "Write Lock Rollback", "write-lock-rollback-workspace")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthTest(t, pool, ws.ID, "write_lock_rollback@example.com")
	wsCtx := data.WithWorkspaceScope(ctx, ws.ID)
	fund := 1000.0
	m := &domain.Machine{ID: "mch_voucher_rollback", Bounds: []domain.AggregateBound{{ID: "b", SumOf: "fld_amount", Per: "fld_period", AtMost: &fund}}}
	machines := map[string]*domain.Machine{m.ID: m}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(wsCtx)
	ok := withWriteLock(rec, req, store, machines, m, "", map[string]any{"fld_period": "p1"}, func(req *http.Request) bool {
		if _, err := store.CreateRecord(req.Context(), m.ID, map[string]any{"fld_period": "p1", "fld_amount": 1.0}); err != nil {
			t.Error(err)
		}
		http.Error(rec, "refused", http.StatusUnprocessableEntity)
		return false
	})
	if ok || rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("ok=%v code=%d, want the refusal passed through", ok, rec.Code)
	}
	if rows, _ := store.ListRecordsBy(wsCtx, m.ID, "fld_period", "p1"); len(rows) != 0 {
		t.Errorf("a refused write left %d rows behind", len(rows))
	}
}
