package data

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func countTestRecords(t *testing.T, store *Store, ctx context.Context) int {
	t.Helper()
	recs, err := store.ListRecords(ctx, storeTestMachine)
	if err != nil {
		t.Fatal(err)
	}
	return len(recs)
}

func TestInTx_commitsTogetherAndRollsBackTogether(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	boom := errors.New("boom")
	err := store.InTx(ctx, func(ctx context.Context) error {
		if _, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"n": 1}); err != nil {
			return err
		}
		// Inside the transaction its own row is visible; outside it, not yet.
		if got := countTestRecords(t, store, ctx); got != 1 {
			t.Errorf("inside the transaction: %d rows, want its own 1", got)
		}
		if got := countTestRecords(t, store, storeTestContext()); got != 0 {
			t.Errorf("outside the transaction: %d rows, want 0 before commit", got)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx = %v, want the function's error", err)
	}
	if got := countTestRecords(t, store, ctx); got != 0 {
		t.Fatalf("after a rollback: %d rows, want 0", got)
	}

	if err := store.InTx(ctx, func(ctx context.Context) error {
		for i := 0; i < 2; i++ {
			if _, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"n": i}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := countTestRecords(t, store, ctx); got != 2 {
		t.Fatalf("after a commit: %d rows, want 2", got)
	}
}

func TestInTx_aNestedCallJoinsTheOuterTransaction(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	boom := errors.New("boom")
	_ = store.InTx(ctx, func(ctx context.Context) error {
		if err := store.InTx(ctx, func(ctx context.Context) error {
			_, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"n": 1})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return boom
	})
	if got := countTestRecords(t, store, ctx); got != 0 {
		t.Errorf("the inner call committed on its own: %d rows survive the outer rollback", got)
	}
}

func TestLock_needsATransactionAndSerialisesWriters(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)
	ctx := storeTestContext()

	if err := store.Lock(ctx, "k"); err == nil {
		t.Fatal("Lock outside a transaction was accepted")
	}

	var mu sync.Mutex
	var order []string
	note := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }
	firstHolds := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = store.InTx(ctx, func(ctx context.Context) error {
			if err := store.Lock(ctx, "tx-test-key"); err != nil {
				t.Error(err)
			}
			close(firstHolds)
			time.Sleep(300 * time.Millisecond)
			note("first done")
			return nil
		})
	}()
	go func() {
		defer wg.Done()
		<-firstHolds
		_ = store.InTx(ctx, func(ctx context.Context) error {
			if err := store.Lock(ctx, "tx-test-key"); err != nil {
				t.Error(err)
			}
			note("second got the lock")
			return nil
		})
	}()
	wg.Wait()
	if len(order) != 2 || order[0] != "first done" {
		t.Errorf("order = %v, want the second writer to wait for the first", order)
	}
	// A different key does not wait.
	done := make(chan struct{})
	go func() {
		_ = store.InTx(ctx, func(ctx context.Context) error { return store.Lock(ctx, "other-key") })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("a lock on another key blocked")
	}
}

func TestInTx_updatesAndDeletesRollBackToo(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()
	keep, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"n": "before"})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"n": "stays"})
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	_ = store.InTx(ctx, func(ctx context.Context) error {
		if _, err := store.UpdateRecord(ctx, storeTestMachine, keep.ID, map[string]any{"n": "after"}); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteRecord(ctx, storeTestMachine, gone.ID); err != nil {
			t.Fatal(err)
		}
		if got, err := store.GetRecord(ctx, storeTestMachine, keep.ID); err != nil || got.Values["n"] != "after" {
			t.Errorf("inside the transaction the update reads back as %v, %v", got, err)
		}
		return boom
	})
	if got, err := store.GetRecord(ctx, storeTestMachine, keep.ID); err != nil || got.Values["n"] != "before" {
		t.Errorf("a rolled-back update left %v, %v", got, err)
	}
	if _, err := store.GetRecord(ctx, storeTestMachine, gone.ID); err != nil {
		t.Errorf("a rolled-back delete removed the row: %v", err)
	}
}
