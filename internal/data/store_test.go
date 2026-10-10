package data

import (
	"context"
	"errors"
	"fmt"
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
	// Credentials are not Workspace-scoped (login identity is global, development-history.md Phase 21 Step
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

// TestStore_ListRecordsSelectBoundsAndOrdersInTheDatabase is the proof that a declared
// `select: records` narrows *retrieval* rather than filtering afterwards.
//
// **The read diagnostic cannot see this**, which is why the assertion is here rather than left to
// the GET sweep: that sweep counts statements, and thirty rows or ten, this is one statement. Its
// `queries`/`reads` numbers do not move, and reading that as "nothing improved" would be wrong --
// the same blind spot already recorded against it for N+1.
//
// What is asserted instead: the row count comes back bounded, and the ordering is the database's.
// Together they are the difference between 007 §20's "never: query all data -> render -> trim" and a
// declaration that reached the query.
func TestStore_ListRecordsSelectBoundsAndOrdersInTheDatabase(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	const total = 30
	for i := range total {
		if _, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{
			"fld_name": fmt.Sprintf("row-%02d", i),
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	newestFirst := []SortKey{{Column: "sort_order", Descending: true}}
	got, _, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", nil, newestFirst, 10)
	if err != nil {
		t.Fatalf("ListRecordsSelect: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d records from a limit of 10 over %d rows -- the LIMIT did not reach the query", len(got), total)
	}
	if name := got[0].Values["fld_name"]; name != "row-29" {
		t.Errorf("first record is %v, want row-29: the ORDER BY did not reach the query", name)
	}

	// Ascending is the other direction, asserted rather than assumed symmetric.
	asc, _, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", nil, []SortKey{{Column: "sort_order"}}, 3)
	if err != nil {
		t.Fatalf("ListRecordsSelect ascending: %v", err)
	}
	if len(asc) != 3 || asc[0].Values["fld_name"] != "row-00" {
		t.Errorf("ascending select = %d records starting %v, want 3 starting row-00", len(asc), asc[0].Values["fld_name"])
	}

	// No sort declared falls back to the same default order the Machine's other reads use, so a
	// Dataset that declares only a limit is not silently reordered.
	none, _, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", nil, nil, 2)
	if err != nil {
		t.Fatalf("ListRecordsSelect unsorted: %v", err)
	}
	if len(none) != 2 || none[0].Values["fld_name"] != "row-00" {
		t.Errorf("unsorted select = %d records starting %v, want 2 starting row-00 (sort_order ASC)", len(none), none[0].Values["fld_name"])
	}
}

// TestStore_ListRecordsSelectStaysWorkspaceScoped: 007 §20 puts security scope before retrieval, and
// a new read path is exactly where that gets forgotten. Asserted directly rather than trusted to the
// shape of the SQL.
func TestStore_ListRecordsSelectStaysWorkspaceScoped(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)

	if _, err := store.CreateRecord(storeTestContext(), storeTestMachine, map[string]any{"fld_name": "ours"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	other := WithWorkspaceScope(context.Background(), "ws_store_test_other")
	got, _, err := store.ListRecordsSelect(other, storeTestMachine, "ds_test", nil, nil, 10)
	if err != nil {
		t.Fatalf("ListRecordsSelect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("another Workspace's select returned %d records -- the scope is not in the statement", len(got))
	}
}

// TestStore_ListRecordsSelectFiltersInTheDatabase covers the predicate half, and its second case is
// the one worth having.
//
// `op: not_equals` cannot be `data->>'x' != $n` in SQL: when the Field is absent the JSONB path is
// NULL, `NULL != 'done'` is NULL, and the row is dropped. expression.Comparison in Go treats a
// missing Field as "" -- which is not "done", so it *keeps* the row. Those are different queries,
// and the difference only shows on records that do not declare the Field at all. IS DISTINCT FROM is
// what makes the pushed-down filter mean what the in-Go one means.
func TestStore_ListRecordsSelectFiltersInTheDatabase(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	seed := []map[string]any{
		{"fld_name": "mine-todo", "fld_assignee": "usr_1", "fld_status": "todo"},
		{"fld_name": "mine-done", "fld_assignee": "usr_1", "fld_status": "done"},
		{"fld_name": "theirs", "fld_assignee": "usr_2", "fld_status": "todo"},
		{"fld_name": "mine-nostatus", "fld_assignee": "usr_1"}, // fld_status absent entirely
	}
	for _, v := range seed {
		if _, err := store.CreateRecord(ctx, storeTestMachine, v); err != nil {
			t.Fatalf("seed %v: %v", v, err)
		}
	}

	got, _, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", []FieldPredicate{
		{Field: "fld_assignee", Value: "usr_1"},
		{Field: "fld_status", Negate: true, Value: "done"},
	}, nil, 50)
	if err != nil {
		t.Fatalf("ListRecordsSelect: %v", err)
	}

	names := map[string]bool{}
	for _, r := range got {
		names[r.Values["fld_name"].(string)] = true
	}
	if !names["mine-todo"] {
		t.Error("mine-todo is missing: the conjunction dropped a row that satisfies both predicates")
	}
	if names["mine-done"] {
		t.Error("mine-done is present: `not_equals done` did not reach the query")
	}
	if names["theirs"] {
		t.Error("theirs is present: the identity predicate did not reach the query")
	}
	if !names["mine-nostatus"] {
		t.Error("mine-nostatus is missing -- a record that declares no fld_status at all must still " +
			"satisfy `not_equals done`, the way expression.Comparison treats an absent Field as \"\". " +
			"This is the IS DISTINCT FROM case; plain != drops it silently.")
	}
	if len(got) != 2 {
		t.Errorf("got %d records, want 2", len(got))
	}
}

// TestStore_ListRecordsSelectOrdersByValueNotByText: the database's `<` must mean what
// expression.Ordered means. A date orders as text (ISO sorts), a number orders as a number ("10" is above "9",
// which text gets backwards), and a record with the Field absent, empty or not a number satisfies nothing --
// where plain SQL would put "" before every date and raise on a ::numeric cast of "abc".
func TestStore_ListRecordsSelectOrdersByValueNotByText(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	seed := []map[string]any{
		{"fld_name": "old", "fld_due": "2026-10-01", "fld_points": "10"},
		{"fld_name": "today", "fld_due": "2026-10-10", "fld_points": "9"},
		{"fld_name": "later", "fld_due": "2026-10-20", "fld_points": "abc"},
		{"fld_name": "empty", "fld_due": "", "fld_points": ""},
		{"fld_name": "absent"},
	}
	for _, v := range seed {
		if _, err := store.CreateRecord(ctx, storeTestMachine, v); err != nil {
			t.Fatalf("seed %v: %v", v, err)
		}
	}
	names := func(where ...FieldPredicate) map[string]bool {
		t.Helper()
		got, _, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", where, nil, 50)
		if err != nil {
			t.Fatalf("ListRecordsSelect: %v", err)
		}
		out := map[string]bool{}
		for _, r := range got {
			out[r.Values["fld_name"].(string)] = true
		}
		return out
	}

	before := names(FieldPredicate{Field: "fld_due", Order: "<", Value: "2026-10-10"})
	if len(before) != 1 || !before["old"] {
		t.Errorf("due < today = %v, want only old (empty and absent must not count as earlier)", before)
	}
	upTo := names(FieldPredicate{Field: "fld_due", Order: "<=", Value: "2026-10-10"})
	if len(upTo) != 2 || !upTo["old"] || !upTo["today"] {
		t.Errorf("due <= today = %v, want old and today", upTo)
	}
	big := names(FieldPredicate{Field: "fld_points", Order: ">", Numeric: true, Value: "9"})
	if len(big) != 1 || !big["old"] {
		t.Errorf("points > 9 = %v, want only old: 10 is above 9 as a number, and \"abc\" must satisfy nothing without erroring", big)
	}
	both := names(
		FieldPredicate{Field: "fld_due", Order: "<", Value: "2026-10-10"},
		FieldPredicate{Field: "fld_points", Order: ">=", Numeric: true, Value: "10"},
	)
	if len(both) != 1 || !both["old"] {
		t.Errorf("conjunction = %v, want only old", both)
	}
	if _, _, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", []FieldPredicate{{Field: "fld_due", Order: "; DROP", Value: "x"}}, nil, 5); err == nil {
		t.Error("an operator outside the four ordering spellings reached the statement")
	}
}

// TestStore_ListRecordsSelectReportsTruncation is 007 §21.9's requirement applied to the bound rather
// than to the work: "a composed experience that exceeds configured budgets should fail clearly or
// degrade through an explicit runtime policy". A limit that silently drops rows is the same class of
// failure as one that silently reads everything -- a screen showing 500 of 700 Documents with no
// indication is indistinguishable from a Workspace that has 500, and at 13 Documents that is invisible.
//
// Exactly-at-the-limit is the case worth having: it must report NOT truncated, which is why the query
// asks for limit+1 rather than comparing len(records) == limit.
func TestStore_ListRecordsSelectReportsTruncation(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	for i := range 5 {
		if _, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": fmt.Sprintf("r%d", i)}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	cases := []struct {
		limit     int
		wantLen   int
		wantTrunc bool
		why       string
	}{
		{3, 3, true, "3 of 5 came back, so more matched"},
		{5, 5, false, "exactly the limit is not truncation -- nothing was dropped"},
		{9, 5, false, "fewer rows than the limit"},
	}
	for _, c := range cases {
		got, truncated, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", nil, nil, c.limit)
		if err != nil {
			t.Fatalf("limit %d: %v", c.limit, err)
		}
		if len(got) != c.wantLen {
			t.Errorf("limit %d returned %d records, want %d", c.limit, len(got), c.wantLen)
		}
		if truncated != c.wantTrunc {
			t.Errorf("limit %d reported truncated=%v, want %v -- %s", c.limit, truncated, c.wantTrunc, c.why)
		}
	}
}

func TestStore_PlaceRecord(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	ids := map[string]string{}
	for _, name := range []string{"a", "b", "c", "d"} {
		r, err := store.CreateRecord(ctx, storeTestMachine, map[string]any{"fld_name": name})
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = r.ID
	}
	order := func() string {
		rs, err := store.ListRecords(ctx, storeTestMachine)
		if err != nil {
			t.Fatal(err)
		}
		var s string
		for _, r := range rs {
			s += r.Values["fld_name"].(string)
		}
		return s
	}

	if err := store.PlaceRecord(ctx, storeTestMachine, ids["d"], ids["b"]); err != nil || order() != "adbc" {
		t.Errorf("d before b: err=%v order=%s, want adbc", err, order())
	}
	if err := store.PlaceRecord(ctx, storeTestMachine, ids["a"], ""); err != nil || order() != "dbca" {
		t.Errorf("a last: err=%v order=%s, want dbca", err, order())
	}
	if err := store.PlaceRecord(ctx, storeTestMachine, ids["c"], ids["c"]); err != nil || order() != "dbca" {
		t.Errorf("a card before itself must change nothing: err=%v order=%s", err, order())
	}
	if err := store.PlaceRecord(ctx, storeTestMachine, "rec_missing", ""); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("an unknown record = %v, want ErrRecordNotFound", err)
	}
	if err := store.PlaceRecord(ctx, storeTestMachine, ids["a"], "rec_missing"); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("an unknown anchor = %v, want ErrRecordNotFound", err)
	}
	if err := store.PlaceRecord(context.Background(), storeTestMachine, ids["a"], ""); err == nil {
		t.Error("an unscoped context must not place anything")
	}
	other := WithWorkspaceScope(context.Background(), "ws_someone_else")
	if err := store.PlaceRecord(other, storeTestMachine, ids["a"], ""); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("another Workspace placing this record = %v, want ErrRecordNotFound", err)
	}
	if order() != "dbca" {
		t.Errorf("a refused placement changed the order: %s", order())
	}
}

// TestStore_ListRecordsSelectGroupsAlternativesAndFindsEmpty: the SQL for `any:` and `is_empty` must select what
// expression.Predicate.Evaluate selects. The cases that differ if either is built wrong are the absent Field (SQL
// NULL), the Field stored as "" and the OR-group being ANDed with the conjunction rather than flattened into it.
func TestStore_ListRecordsSelectGroupsAlternativesAndFindsEmpty(t *testing.T) {
	pool := storePool(t)
	cleanupStoreTest(t, pool)
	store := NewStore(pool)
	ctx := storeTestContext()

	for _, v := range []map[string]any{
		{"fld_name": "soon", "fld_status": "todo", "fld_due": "2026-10-12"},
		{"fld_name": "late", "fld_status": "todo", "fld_due": "2026-10-20"},
		{"fld_name": "undated", "fld_status": "todo"},
		{"fld_name": "blank", "fld_status": "todo", "fld_due": ""},
		{"fld_name": "undated-done", "fld_status": "done"},
		{"fld_name": "late-done", "fld_status": "done", "fld_due": "2026-10-20"},
	} {
		if _, err := store.CreateRecord(ctx, storeTestMachine, v); err != nil {
			t.Fatalf("seed %v: %v", v, err)
		}
	}
	names := func(where ...FieldPredicate) map[string]bool {
		t.Helper()
		got, _, err := store.ListRecordsSelect(ctx, storeTestMachine, "ds_test", where, nil, 50)
		if err != nil {
			t.Fatalf("ListRecordsSelect: %v", err)
		}
		out := map[string]bool{}
		for _, r := range got {
			out[r.Values["fld_name"].(string)] = true
		}
		return out
	}
	same := func(label string, got map[string]bool, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", label, got, want)
			return
		}
		for _, w := range want {
			if !got[w] {
				t.Errorf("%s: got %v, want %v", label, got, want)
			}
		}
	}

	same("is_empty alone", names(FieldPredicate{Field: "fld_due", Empty: true}), "undated", "blank", "undated-done")
	same("open and (undated or late)", names(
		FieldPredicate{Field: "fld_status", Negate: true, Value: "done"},
		FieldPredicate{Field: "fld_due", Empty: true, Alternative: true},
		FieldPredicate{Field: "fld_due", Order: ">", Value: "2026-10-17", Alternative: true},
	), "late", "undated", "blank")
	same("one alternative only is just that predicate", names(
		FieldPredicate{Field: "fld_due", Order: ">", Value: "2026-10-17", Alternative: true},
	), "late", "late-done")
}
