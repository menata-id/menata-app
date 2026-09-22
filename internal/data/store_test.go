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

// storeTestContext scopes ctx to a dedicated test Workspace -- every record-scoped Store method
// requires one (WithWorkspaceScope), the same as a real request's requireAuth-resolved Workspace.
func storeTestContext() context.Context {
	return WithWorkspaceScope(context.Background(), "ws_store_test")
}

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
	ctx := storeTestContext()

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
	ctx := storeTestContext()

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

	_, err := store.GetRecord(storeTestContext(), storeTestMachine, "rec_does_not_exist")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("GetRecord(missing) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_GetRecord_unscopedContext(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	_, err := store.GetRecord(context.Background(), storeTestMachine, "rec_whatever")
	if !errors.Is(err, errNotScoped) {
		t.Errorf("GetRecord(unscoped ctx) error = %v, want errNotScoped", err)
	}
}

func TestStore_UpdateRecord(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

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

	_, err := store.UpdateRecord(storeTestContext(), storeTestMachine, "rec_does_not_exist", map[string]any{"fld_name": "x"})
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("UpdateRecord(missing) error = %v, want ErrRecordNotFound", err)
	}
}

func TestStore_DeleteRecord(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

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

	if err := store.DeleteRecord(storeTestContext(), storeTestMachine, "rec_never_existed"); err != nil {
		t.Errorf("DeleteRecord(absent) = %v, want nil", err)
	}
}

func TestStore_ListRecords_orderedBySortOrder(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

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
	ctx := storeTestContext()

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

func cleanupCredentialTest(t *testing.T, pool *pgxpool.Pool, email string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM credentials WHERE email = $1`, email); err != nil {
			t.Errorf("cleanup credential %s: %v", email, err)
		}
	})
}

func TestStore_CreateAndGetCredential(t *testing.T) {
	pool := storePool(t)
	const email = "store_test@example.com"
	cleanupCredentialTest(t, pool, email)
	store := NewStore(pool)
	// Credentials are not Workspace-scoped (login identity is global, ROADMAP.md Phase 21 Step
	// 3) -- an ordinary, unscoped context is correct here, unlike the record-scoped tests above.
	ctx := context.Background()

	if err := store.CreateCredential(ctx, email, "Test Person", "hashed-value", false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}

	got, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if got.PasswordHash != "hashed-value" {
		t.Errorf("GetCredential().PasswordHash = %q, want %q", got.PasswordHash, "hashed-value")
	}
	if got.EmailVerified {
		t.Error("GetCredential().EmailVerified = true, want false (created unverified)")
	}
}

func TestStore_CreateCredential_verified(t *testing.T) {
	pool := storePool(t)
	const email = "store_test_verified@example.com"
	cleanupCredentialTest(t, pool, email)
	store := NewStore(pool)
	ctx := context.Background()

	if err := store.CreateCredential(ctx, email, "Test Person", "hashed-value", true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	got, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !got.EmailVerified {
		t.Error("GetCredential().EmailVerified = false, want true (created verified)")
	}
}

func TestStore_SetCredential_updatesExisting(t *testing.T) {
	pool := storePool(t)
	const email = "store_test_set_credential@example.com"
	cleanupCredentialTest(t, pool, email)
	store := NewStore(pool)
	ctx := context.Background()

	if err := store.CreateCredential(ctx, email, "Test Person", "old-hash", true); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	if err := store.SetCredential(ctx, email, "new-hash"); err != nil {
		t.Fatalf("SetCredential: %v", err)
	}
	got, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if got.PasswordHash != "new-hash" {
		t.Errorf("GetCredential().PasswordHash = %q, want %q", got.PasswordHash, "new-hash")
	}
	if !got.EmailVerified {
		t.Error("GetCredential().EmailVerified = false after SetCredential, want unchanged (true)")
	}
}

func TestStore_SetCredential_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	err := store.SetCredential(context.Background(), "no-such-user@example.com", "new-hash")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("SetCredential(missing) error = %v, want ErrCredentialNotFound -- must not silently create a never-registered account", err)
	}
}

func TestStore_MarkEmailVerified(t *testing.T) {
	pool := storePool(t)
	const email = "store_test_mark_verified@example.com"
	cleanupCredentialTest(t, pool, email)
	store := NewStore(pool)
	ctx := context.Background()

	if err := store.CreateCredential(ctx, email, "Test Person", "hashed-value", false); err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	if err := store.MarkEmailVerified(ctx, email); err != nil {
		t.Fatalf("MarkEmailVerified: %v", err)
	}
	got, err := store.GetCredential(ctx, email)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if !got.EmailVerified {
		t.Error("GetCredential().EmailVerified = false after MarkEmailVerified, want true")
	}
}

func TestStore_MarkEmailVerified_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	err := store.MarkEmailVerified(context.Background(), "no-such-user@example.com")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("MarkEmailVerified(missing) error = %v, want ErrCredentialNotFound", err)
	}
}

func TestStore_GetCredential_notFound(t *testing.T) {
	pool := storePool(t)
	store := NewStore(pool)

	_, err := store.GetCredential(context.Background(), "no-such-user@example.com")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Errorf("GetCredential(missing) error = %v, want ErrCredentialNotFound", err)
	}
}
