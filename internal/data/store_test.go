package data

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// storePool connects to a real Postgres database for Store's own integration tests -- CRUD
// against the `records` table isn't something a mock is worth building for, and
// internal/composition/threshold_test.go already established the pattern this reuses: skip
// without a real database rather than fail, so `make test` stays fast and DB-free by default.
func storePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run internal/data's store integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

const storeTestMachine = "mch_store_test"

func cleanupStoreTest(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM records WHERE machine_id = $1`, storeTestMachine); err != nil {
			t.Errorf("cleanup %s records: %v", storeTestMachine, err)
		}
	})
}

func TestStore_CreateAndGetRecord(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := context.Background()

	created, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": "Ada"})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateRecord: got empty ID")
	}
	if created.SortOrder != 1 {
		t.Errorf("CreateRecord: SortOrder = %d, want 1 for the first record", created.SortOrder)
	}

	got, err := store.GetRecord(ctx, storeTestMachine, created.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got.Values["fld_name"] != "Ada" {
		t.Errorf("GetRecord: fld_name = %v, want %q", got.Values["fld_name"], "Ada")
	}
}

func TestStore_CreateRecord_incrementsSortOrder(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := context.Background()

	first, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": "one"})
	if err != nil {
		t.Fatalf("CreateRecord(1): %v", err)
	}
	second, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": "two"})
	if err != nil {
		t.Fatalf("CreateRecord(2): %v", err)
	}
	if second.SortOrder <= first.SortOrder {
		t.Errorf("SortOrder did not increase: first=%d second=%d", first.SortOrder, second.SortOrder)
	}
}

func TestStore_GetRecord_notFound(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)

	_, err := store.GetRecord(context.Background(), storeTestMachine, "rec_does_not_exist")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("GetRecord(missing) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_UpdateRecord(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := context.Background()

	created, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": "before"})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	updated, err := store.UpdateRecord(ctx, storeTestMachine, created.ID, map[string]any{"fld_name": "after"})
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if updated.Values["fld_name"] != "after" {
		t.Errorf("UpdateRecord: fld_name = %v, want %q", updated.Values["fld_name"], "after")
	}

	got, err := store.GetRecord(ctx, storeTestMachine, created.ID)
	if err != nil {
		t.Fatalf("GetRecord after update: %v", err)
	}
	if got.Values["fld_name"] != "after" {
		t.Errorf("GetRecord after update: fld_name = %v, want %q", got.Values["fld_name"], "after")
	}
}

func TestStore_UpdateRecord_notFound(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)

	_, err := store.UpdateRecord(context.Background(), storeTestMachine, "rec_does_not_exist", map[string]any{"fld_name": "x"})
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("UpdateRecord(missing) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_DeleteRecord(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := context.Background()

	created, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": "gone soon"})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	if err := store.DeleteRecord(ctx, storeTestMachine, created.ID); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}

	_, err = store.GetRecord(ctx, storeTestMachine, created.ID)
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("GetRecord after delete: error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_DeleteRecord_alreadyAbsentIsNotAnError(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)

	if err := store.DeleteRecord(context.Background(), storeTestMachine, "rec_never_existed"); err != nil {
		t.Errorf("DeleteRecord(absent) = %v, want nil", err)
	}
}

func TestStore_ListRecords_orderedBySortOrder(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := context.Background()

	names := []string{"first", "second", "third"}
	for _, name := range names {
		if _, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": name}); err != nil {
			t.Fatalf("CreateRecord(%s): %v", name, err)
		}
	}

	records, err := store.ListRecords(ctx, storeTestMachine)
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(records) != len(names) {
		t.Fatalf("ListRecords: got %d records, want %d", len(records), len(names))
	}
	for i, want := range names {
		if got := records[i].Values["fld_name"]; got != want {
			t.Errorf("ListRecords[%d].fld_name = %v, want %q", i, got, want)
		}
	}
}

func TestStore_ListRecordsBy_filtersOnFieldValue(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := context.Background()

	if _, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_owner": "usr_a", "fld_name": "mine"}); err != nil {
		t.Fatalf("CreateRecord(usr_a): %v", err)
	}
	if _, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_owner": "usr_b", "fld_name": "not mine"}); err != nil {
		t.Fatalf("CreateRecord(usr_b): %v", err)
	}

	records, err := store.ListRecordsBy(ctx, storeTestMachine, "fld_owner", "usr_a")
	if err != nil {
		t.Fatalf("ListRecordsBy: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ListRecordsBy: got %d records, want 1", len(records))
	}
	if records[0].Values["fld_name"] != "mine" {
		t.Errorf("ListRecordsBy: fld_name = %v, want %q", records[0].Values["fld_name"], "mine")
	}
}
