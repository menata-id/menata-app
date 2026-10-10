package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRecordNotFound is returned when a record ID doesn't exist for the given Machine.
var ErrRecordNotFound = errors.New("record not found")

// ErrCredentialNotFound is returned when no credential row exists for an email.
var ErrCredentialNotFound = errors.New("credential not found")

// errNotScoped is returned by a record-scoped method called with a context that was never given a
// Workspace via WithWorkspaceScope -- a forgotten scope should fail loudly (005 SS Security
// Ordering: scope must be established before retrieval, not defaulted silently) rather than
// silently read or write across every Workspace.
var errNotScoped = errors.New("data: context has no Workspace scope -- call WithWorkspaceScope first")

// Store is the Data Plane's physical execution against PostgreSQL for the generic `records`
// table. It is intentionally narrow: create and list by Machine, no Query/Projection/Filter
// composition yet (007-composable-runtime-architecture.md SS7-8 -- those land once more than one
// consumer needs them).
//
// Every record-scoped method (CreateRecord/ListRecords/ListRecordsBy/GetRecord/UpdateRecord/
// DeleteRecord) is additionally scoped to one Workspace (development-history.md Phase 21 Step 2 -- "Workspace
// never enters the data path" closed), carried on ctx via WithWorkspaceScope -- the same
// context-carried-per-request-scope shape WithReadLog already established for the query
// diagnostic. Every existing call site already passes the request's own ctx through untouched, so
// none of them need to change: requireAuth's middleware is the one place that decides what a
// request's ctx carries, once per request, after resolving a signed-in identity's real Workspace
// (Phase 21 Step 4) -- a struct-field-scoped Store was tried first and rejected because it would
// have needed a change at every one of the ~20 handler entry points instead of this one place.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps an existing connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type workspaceScopeKey struct{}

// WithWorkspaceScope returns a context scoped to workspaceID. Every record-scoped Store method
// called with it reads and writes only that Workspace's records.
func WithWorkspaceScope(ctx context.Context, workspaceID string) context.Context {
	return context.WithValue(ctx, workspaceScopeKey{}, workspaceID)
}

func workspaceScopeFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(workspaceScopeKey{}).(string)
	return id, ok && id != ""
}

// WorkspaceScope returns the Workspace id a context carries, for the rare caller that needs the
// concrete value itself (e.g. looking up the Workspace's own name) rather than merely scoping a
// Store call by it.
func WorkspaceScope(ctx context.Context) (string, bool) {
	return workspaceScopeFrom(ctx)
}

// CreateRecord inserts a new Record for the given Machine, at the next sort_order after every
// existing record of that Machine in this Store's Workspace (development-history.md Phase 9: child
// collections need a meaningful order). Callers must validate values with ValidateRecord first --
// the store does not know Domain Plane rules.
func (s *Store) CreateRecord(ctx context.Context, machineID string, values map[string]any) (*Record, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal record values: %w", err)
	}

	r := &Record{ID: newRecordID(), MachineID: machineID, WorkspaceID: workspaceID, Values: values}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO records (id, machine_id, workspace_id, data, sort_order)
		VALUES ($1, $2, $3, $4::jsonb, COALESCE((SELECT MAX(sort_order) FROM records WHERE machine_id = $2 AND workspace_id = $3), 0) + 1)
		RETURNING sort_order, created_at, updated_at
	`, r.ID, r.MachineID, workspaceID, data)

	if err := row.Scan(&r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert record: %w", err)
	}
	return r, nil
}

// ListRecords returns every Record for the given Machine in this Store's Workspace, in sort_order.
func (s *Store) ListRecords(ctx context.Context, machineID string) ([]*Record, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record(machineID)
	return s.queryRecords(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND workspace_id = $2
		ORDER BY sort_order ASC, created_at ASC
	`, machineID, workspaceID)
}

// CountRecords returns how many Records of machineID exist in this Store's Workspace.
//
// A real COUNT rather than len(ListRecords(...)): its one caller renders a single integer on a
// card, and the existing path would load every row, decode every record's JSON and discard it all
// to produce that number. Workspace-scoped like every other read (007 §20: scope is established
// before retrieval, never trimmed after).
//
// Deliberately not routed through a Dataset: a Dataset is a *named semantic definition* a screen
// resolves by id, and this is one Application's card count, declared by summary_machine and
// meaningful only there. Making it a Dataset would mean declaring one per Application to say
// "count this Machine's rows", which is the declaration the Machine id already is.
func (s *Store) CountRecords(ctx context.Context, machineID string) (int, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return 0, errNotScoped
	}
	// A distinct target from ListRecords', though both concern the same Machine: they are
	// different statements, and labelling them alike made a page that lists a Machine and counts
	// it look like it had read the same thing twice. /home did exactly that (`mch_document x2` in
	// the log from the day this diagnostic was fixed), and the repeat was in the label, not in the
	// request. Whether counting a Machine a page has *already listed* is itself waste is a
	// separate question, and one this naming now lets someone actually ask.
	readLogFrom(ctx).record(machineID + " count")
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM records WHERE machine_id = $1 AND workspace_id = $2
	`, machineID, workspaceID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count records: %w", err)
	}
	return n, nil
}

// ListRecordsBy returns every Record of machineID in this Store's Workspace whose fieldID value
// equals value, in sort_order -- the query behind a child collection (development-history.md Phase 9): fieldID
// is a reference field on machineID pointing back to another record (value = that record's id).
func (s *Store) ListRecordsBy(ctx context.Context, machineID, fieldID, value string) ([]*Record, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record(machineID + " by " + fieldID)
	return s.queryRecords(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND data->>$2 = $3 AND workspace_id = $4
		ORDER BY sort_order ASC, created_at ASC
	`, machineID, fieldID, value, workspaceID)
}

// SortKey is one ordering step for ListRecordsSelect: a column expression already resolved by the
// caller, plus its direction. internal/data does not know what a Field is, so resolving a declared
// `sort:` entry into either a JSONB path or a real column is internal/composition's job -- this
// package receives the result, which keeps the Domain Plane's vocabulary out of the storage layer
// (boundary_test.go: internal/data owns the connection and has no Runtime Metadata semantics).
// FieldPredicate is one resolved filter for ListRecordsSelect: a JSONB path the caller has already
// built from a validated Field id, an operator, and the value to compare -- with any runtime context
// value (007 §9.2's `current_user`, `parameters`) already resolved into a literal by the caller.
//
// internal/data receives resolved values for the same reason it receives resolved sort columns: it
// owns the connection and has no Runtime Metadata semantics (boundary_test.go). Resolving a sentinel
// here would put the Domain Plane's vocabulary in the storage layer.
type FieldPredicate struct {
	// Field is a validated Field id; the query builds data->>'<id>' from it. Never user input.
	Field string
	// Negate inverts the comparison (`op: not_equals`).
	Negate bool
	// Order, when set, makes this an ordered comparison (`<`, `<=`, `>`, `>=`) instead of an equality;
	// Negate is then ignored. It is one of the four SQL spellings, never an author's text: the caller
	// maps a validated expression.Op onto it.
	Order string
	// Numeric orders the Field as a number, which is what a numeric declared value means (expression.IsNumber).
	// A stored value that is not a number then satisfies nothing, exactly as expression.Ordered answers.
	Numeric bool
	Value   string
	// Empty asks that the Field have no value (expression.OpIsEmpty): absent, or stored as nothing. Value,
	// Negate and Order are ignored.
	Empty bool
	// Alternative marks this predicate as one of an OR-group. Every Alternative predicate of a statement is
	// joined with OR into a single clause, which is then ANDed with the rest (expression.Predicate's Any).
	Alternative bool
}

var orderedSQL = map[string]bool{"<": true, "<=": true, ">": true, ">=": true}

// numberPattern is a stored value that reads as a plain number; anything else is not cast, because a
// ::numeric on a non-number is an error that would fail the whole statement rather than one row.
const numberPattern = `^-?[0-9]+(\.[0-9]+)?$`

type SortKey struct {
	// Column is a validated SQL fragment: either a bare column name from a closed set, or a
	// data->>'fld_x' expression built from a validated Field id. Never user input.
	Column     string
	Descending bool
}

// ListRecordsSelect returns records of machineID in this Store's Workspace, ordered and bounded as
// the caller asks (007 §7.8 Sort, §7.9 Pagination).
//
// **This is the statement that makes a declared `select: records` mean something.** Its siblings
// return every row of a Machine and let Go reduce them, which is the shape 007 §20 names with the
// word never ("query all data -> render -> trim") and §28 invariant 4 forbids by default. Here the
// ORDER BY and LIMIT are the database's, so a Dataset declaring `limit: 10` causes ten rows to be
// fetched rather than ten to be kept.
//
// Workspace scope stays *in the statement*, exactly as every other read here: 007 §20 puts security
// scope before retrieval, never as a filter applied after it.
//
// sort may be empty, which falls back to the same default order the other reads use, so a Dataset
// that declares no ordering is not silently reordered.
// ListRecordsSelect returns at most limit records, and reports whether more matched.
//
// **truncated is not a convenience.** 007 §21.9 requires that a composed experience exceeding its
// budget "fail clearly or degrade through an explicit runtime policy" and "must not silently produce
// unbounded work" -- and the inverse is just as true: a bound that silently drops rows is the same
// class of failure as one that silently reads everything. A screen showing 500 of 700 Documents with
// no indication is indistinguishable from a Workspace that has 500.
//
// It is measured rather than inferred: the query asks for limit+1 and the extra row, if it arrives, is
// the answer. Comparing len(records) == limit would report truncation for a set that happens to be
// exactly the limit, which is a lie in the other direction.
func (s *Store) ListRecordsSelect(ctx context.Context, machineID, datasetID string, where []FieldPredicate, sort []SortKey, limit int) (records []*Record, truncated bool, err error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, false, errNotScoped
	}
	// Named after the Dataset rather than the Machine, because that is what distinguishes it: the
	// same Machine may be read whole on the same request, and a diagnostic that called both
	// "mch_activity" would report a repeat where there are two genuinely different statements.
	readLogFrom(ctx).record(machineID + " select " + datasetID)

	// Predicates are appended as bind parameters; only the JSONB path is built, and it is built from
	// an id metadata validation already accepted. 007 §21.2 is the reason this is here at all --
	// "Filters should execute in PostgreSQL whenever safe and beneficial" -- and §22 is why a path
	// expression is an acceptable realisation: "A logical field may be physically realized as: JSONB
	// value, expression index, stored column, generated column, materialized aggregate", and the
	// runtime may change that without touching metadata.
	args := []any{machineID, workspaceID}
	filter := ""
	var alternatives []string
	for _, p := range where {
		clause, err := predicateSQL(p, &args)
		if err != nil {
			return nil, false, err
		}
		if p.Alternative {
			alternatives = append(alternatives, clause)
			continue
		}
		filter += " AND " + clause
	}
	if len(alternatives) > 0 {
		filter += " AND (" + strings.Join(alternatives, " OR ") + ")"
	}

	orderBy := "sort_order ASC, created_at ASC"
	if len(sort) > 0 {
		parts := make([]string, 0, len(sort))
		for _, k := range sort {
			direction := "ASC"
			if k.Descending {
				direction = "DESC"
			}
			parts = append(parts, k.Column+" "+direction)
		}
		orderBy = strings.Join(parts, ", ")
	}

	// #nosec G201 -- orderBy is assembled from validated identifiers only (a closed set of column
	// names, or a data->>'fld_x' path over a load-validated Field id); every value stays a bind
	// parameter. A placeholder cannot carry an ORDER BY expression, which is why this one is built.
	args = append(args, limit+1)
	query := fmt.Sprintf(`
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND workspace_id = $2%s
		ORDER BY %s
		LIMIT $%d
	`, filter, orderBy, len(args))
	rows, err := s.queryRecords(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	if len(rows) > limit {
		return rows[:limit], true, nil
	}
	return rows, false, nil
}

// predicateSQL is one predicate's clause, its value (when it has one) appended to args as a bind parameter. Only
// the JSONB path is built into the text, from an id metadata validation already accepted.
func predicateSQL(p FieldPredicate, args *[]any) (string, error) {
	if p.Empty {
		return fmt.Sprintf("(data->>'%s' IS NULL OR data->>'%s' = '')", p.Field, p.Field), nil
	}
	*args = append(*args, p.Value)
	n := len(*args)
	if p.Order != "" {
		if !orderedSQL[p.Order] {
			return "", fmt.Errorf("data: %q is not an ordering operator", p.Order)
		}
		// An absent or empty stored value satisfies no ordering (expression.Ordered), so the empty
		// string is excluded explicitly: as text it would sort before every date.
		if p.Numeric {
			return fmt.Sprintf("CASE WHEN data->>'%s' ~ '%s' THEN (data->>'%s')::numeric END %s $%d::numeric",
				p.Field, numberPattern, p.Field, p.Order, n), nil
		}
		return fmt.Sprintf("(data->>'%s' <> '' AND data->>'%s' %s $%d)", p.Field, p.Field, p.Order, n), nil
	}
	op := "="
	if p.Negate {
		op = "IS DISTINCT FROM"
	}
	return fmt.Sprintf("data->>'%s' %s $%d", p.Field, op, n), nil
}

// ListRecordsByAny returns every Record of machineID whose fieldID value is one of ids -- the child
// half of a declared Relation, fetched as **one** query rather than one per parent.
//
// 007 §7.5 leaves the physical strategy open ("a join, semi-join, lookup, or other physical
// strategy") and this is the lookup: the parents come back bounded from ListRecordsSelect, their ids
// become this query's parameter, and correlation happens in memory. Two indexed statements at 13
// parents and 23 children beat a join, and the choice is invisible to metadata -- §22's whole point.
//
// **Not one query per parent**, which is the shape this exists to avoid: the GET sweep's own doc
// comment names an N+1 as its blind spot, since a per-row loop repeats nothing when there is one row.
// An empty ids slice returns nothing without querying, because `= ANY('{}')` is a round trip to learn
// what the caller already knows.
func (s *Store) ListRecordsByAny(ctx context.Context, machineID, datasetID, fieldID string, ids []string) ([]*Record, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	if len(ids) == 0 {
		return nil, nil
	}
	readLogFrom(ctx).record(machineID + " related " + datasetID)
	return s.queryRecords(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND workspace_id = $2 AND data->>$3 = ANY($4)
		ORDER BY sort_order ASC, created_at ASC
	`, machineID, workspaceID, fieldID, ids)
}

// ListRecordsByIDs returns the given records of one Machine, Workspace-scoped in the statement. It is the
// read a screen needs when it already holds ids from another Machine's reference Field (a board card's
// tags) and wants those few records, rather than every record of the Machine.
func (s *Store) ListRecordsByIDs(ctx context.Context, machineID string, ids []string) ([]*Record, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	if len(ids) == 0 {
		return nil, nil
	}
	readLogFrom(ctx).record(machineID + " by ids")
	return s.queryRecords(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND workspace_id = $2 AND id = ANY($3)
		ORDER BY sort_order ASC, created_at ASC
	`, machineID, workspaceID, ids)
}

func (s *Store) queryRecords(ctx context.Context, query string, args ...any) ([]*Record, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}
	defer rows.Close()

	var records []*Record
	for rows.Next() {
		r := &Record{}
		var data []byte
		if err := rows.Scan(&r.ID, &r.MachineID, &r.WorkspaceID, &data, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		if err := json.Unmarshal(data, &r.Values); err != nil {
			return nil, fmt.Errorf("unmarshal record %s: %w", r.ID, err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetRecord returns one Record by ID, scoped to the given Machine and this Store's Workspace.
func (s *Store) GetRecord(ctx context.Context, machineID, id string) (*Record, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record(machineID + " by id")
	row := s.pool.QueryRow(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND id = $2 AND workspace_id = $3
	`, machineID, id, workspaceID)

	r := &Record{}
	var data []byte
	if err := row.Scan(&r.ID, &r.MachineID, &r.WorkspaceID, &data, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get record: %w", err)
	}
	if err := json.Unmarshal(data, &r.Values); err != nil {
		return nil, fmt.Errorf("unmarshal record %s: %w", r.ID, err)
	}
	return r, nil
}

// UpdateRecord replaces a Record's values, scoped to this Store's Workspace. Callers must
// validate values with ValidateRecord first, same as CreateRecord.
func (s *Store) UpdateRecord(ctx context.Context, machineID, id string, values map[string]any) (*Record, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal record values: %w", err)
	}

	r := &Record{ID: id, MachineID: machineID, WorkspaceID: workspaceID, Values: values}
	row := s.pool.QueryRow(ctx, `
		UPDATE records
		SET data = $3::jsonb, updated_at = NOW()
		WHERE machine_id = $1 AND id = $2 AND workspace_id = $4
		RETURNING sort_order, created_at, updated_at
	`, machineID, id, data, workspaceID)

	if err := row.Scan(&r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("update record: %w", err)
	}
	return r, nil
}

// PlaceRecord moves one Record to sit immediately before beforeID in this Machine's order, or last when
// beforeID is empty, and renumbers sort_order to say so. Order is the only thing it changes; the Record's
// values are UpdateRecord's. The renumbering is the whole Machine, not the neighbours, because sort_order
// is creation order (MAX+1) with no gaps to slot a value into -- and the select locks the rows, so two
// moves cannot interleave into a mixed order. Workspace is in every statement (the same rule as every
// other Store method taking an id).
func (s *Store) PlaceRecord(ctx context.Context, machineID, id, beforeID string) error {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return errNotScoped
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("place record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	readLogFrom(ctx).record(machineID + " order")
	rows, err := tx.Query(ctx, `
		SELECT id FROM records
		WHERE machine_id = $1 AND workspace_id = $2
		ORDER BY sort_order ASC, created_at ASC
		FOR UPDATE
	`, machineID, workspaceID)
	if err != nil {
		return fmt.Errorf("place record: %w", err)
	}
	var current []string
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			rows.Close()
			return fmt.Errorf("place record: %w", err)
		}
		current = append(current, rid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("place record: %w", err)
	}

	order := make([]string, 0, len(current))
	found, anchored := false, beforeID == ""
	for _, rid := range current {
		found = found || rid == id
		anchored = anchored || rid == beforeID
	}
	if !found || !anchored {
		return ErrRecordNotFound
	}
	if beforeID == id {
		return nil
	}
	for _, rid := range current {
		if rid == id {
			continue
		}
		if rid == beforeID {
			order = append(order, id)
		}
		order = append(order, rid)
	}
	if beforeID == "" {
		order = append(order, id)
	}
	numbers := make([]int32, len(order))
	for i := range order {
		numbers[i] = int32(i + 1)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE records SET sort_order = v.n, updated_at = NOW()
		FROM unnest($3::text[], $4::int[]) AS v(id, n)
		WHERE records.id = v.id AND records.machine_id = $1 AND records.workspace_id = $2
	`, machineID, workspaceID, order, numbers); err != nil {
		return fmt.Errorf("place record: %w", err)
	}
	return tx.Commit(ctx)
}

// DeleteRecord removes a Record from this Store's Workspace. It is not an error to delete an
// already-absent record.
func (s *Store) DeleteRecord(ctx context.Context, machineID, id string) error {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return errNotScoped
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM records WHERE machine_id = $1 AND id = $2 AND workspace_id = $3`, machineID, id, workspaceID)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	return nil
}

// Credential is one login identity's stored password hash and verification state (ROADMAP.md
// Phase 21 round 2, Step D) -- EmailVerified is false for a self-registered admin until they click
// their emailed link, but true immediately for an invited member's own first-login activation
// (Step 6), a deliberately different trust model since a Workspace Admin already vouches for that
// specific email by typing it in themselves.
// Credential is one login identity: the email that keys it, the password hash, whether that email
// has been proven, and -- since 2026-09-22 (migration 010) -- the person's full name.
//
// FullName lives here rather than on their mch_user record because a name belongs to the person
// who owns the email, not to any one Workspace they happen to join (owner decision; see
// migrations/010_identity_full_name.sql for the full reasoning). This is the single source of
// reference for it: nothing copies it into a record, and only its owner may change it
// (web.submitProfile).
type Credential struct {
	Email         string
	FullName      string
	PasswordHash  string
	EmailVerified bool
	// NotifyAssigned/NotifyDecided are Tahap 6's own email preferences (migrations/
	// 014_notification_preferences.sql) -- identity-level, like everything else here, and default
	// true so an existing credential (created before this column existed) keeps getting the emails
	// it would have gotten had the preference always existed. execution.sendNotification reads
	// these; they gate only the email, never the in-app mch_notification row.
	NotifyAssigned bool
	NotifyDecided  bool
	// NotifySLABreach is the SLA-breach reminder's own preference (migrations/
	// 015_notify_sla_breach_preference.sql, Flow 2 canvas re-audit 2026-09-27) -- same shape and
	// same default-true reasoning as the two above.
	NotifySLABreach bool
}

// CreateCredential stores a login credential's hashed password, keyed by email (ROADMAP.md
// Phase 21 Step 1). Hashing is internal/authorization's job -- the store only persists whatever
// hash it is given. emailVerified is set by the caller, not assumed: registration's own credential
// starts unverified, an invite's activation starts verified.
func (s *Store) CreateCredential(ctx context.Context, email, fullName, passwordHash string, emailVerified bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO credentials (email, full_name, password_hash, email_verified) VALUES ($1, $2, $3, $4)
	`, email, fullName, passwordHash, emailVerified)
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

// GetCredential returns the stored credential for email, or ErrCredentialNotFound.
func (s *Store) GetCredential(ctx context.Context, email string) (*Credential, error) {
	readLogFrom(ctx).record("credential by email")
	cred := &Credential{Email: email}
	err := s.pool.QueryRow(ctx, `SELECT full_name, password_hash, email_verified, notify_assigned, notify_decided, notify_sla_breach FROM credentials WHERE email = $1`, email).
		Scan(&cred.FullName, &cred.PasswordHash, &cred.EmailVerified, &cred.NotifyAssigned, &cred.NotifyDecided, &cred.NotifySLABreach)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("get credential: %w", err)
	}
	return cred, nil
}

// SetCredential replaces an *existing* credential's password hash -- a real UPDATE, not an upsert:
// a reset token is only ever issued for an email GetCredential already found (forgot-password's own
// gate), so this must fail loudly (ErrCredentialNotFound) rather than silently create a brand-new,
// never-registered-or-invited account if it were ever called for one that doesn't exist. Distinct
// from CreateCredential (a plain insert, which must fail on conflict for the registration
// duplicate-email check). Resetting does not change EmailVerified, only the hash.
func (s *Store) SetCredential(ctx context.Context, email, passwordHash string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE credentials SET password_hash = $2 WHERE email = $1`, email, passwordHash)
	if err != nil {
		return fmt.Errorf("set credential: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// ReplaceCredentialIf sets a new password hash only while the stored one is still oldHash, in one statement.
// A password-reset link is single-use because of this predicate (K04): two submissions of the same link race
// on the row, and the second finds the hash already changed. ErrCredentialNotFound covers both a missing
// account and a changed hash -- to the caller they are the same answer, "this link no longer works".
func (s *Store) ReplaceCredentialIf(ctx context.Context, email, oldHash, newHash string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE credentials SET password_hash = $3 WHERE email = $1 AND password_hash = $2`, email, oldHash, newHash)
	if err != nil {
		return fmt.Errorf("replace credential: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// SetFullName replaces an identity's own full name (migration 010). An UPDATE, not an upsert, for
// the same reason SetCredential is one: the only caller is the Profile screen, acting for an
// identity that is already signed in, so an email with no credential row reaching here is a bug to
// surface rather than a new account to invent.
//
// One write changes the name everywhere it is displayed, in every Workspace, because nothing
// stores a second copy of it -- which is the whole point of it living here.
func (s *Store) SetFullName(ctx context.Context, email, fullName string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE credentials SET full_name = $2 WHERE email = $1`, email, fullName)
	if err != nil {
		return fmt.Errorf("set full name: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// UpdateNotificationPreferences replaces an identity's own email-notification preferences (Flow 2
// gap study Tahap 6, extended 2026-09-27 with the SLA-breach reminder's own third column) -- an
// UPDATE, not an upsert, the same posture SetCredential/SetFullName take: the only caller is
// /account-notifications, acting for an identity that is already signed in.
func (s *Store) UpdateNotificationPreferences(ctx context.Context, email string, assigned, decided, slaBreach bool) error {
	ct, err := s.pool.Exec(ctx, `UPDATE credentials SET notify_assigned = $2, notify_decided = $3, notify_sla_breach = $4 WHERE email = $1`, email, assigned, decided, slaBreach)
	if err != nil {
		return fmt.Errorf("update notification preferences: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// MemberNames maps every mch_user record id in workspaceID to the display name of the identity
// behind it -- the runtime's single resolution point for "who is this person", replacing the
// fld_name each record used to carry (migration 010).
//
// The join is the membership row: workspace_members is what ties a Workspace's own mch_user record
// to a login identity, and it is the only thing that does. A record with no membership therefore
// resolves to no name at all, which is not a gap to paper over -- under the model this runtime now
// follows, an mch_user record only ever comes into existence when an invitation is accepted or a
// Workspace is created, both of which require an identity to exist first.
//
// Falls back to the email when an identity has no name yet, so a screen degrades to something
// addressable rather than to a bare record id.
func (s *Store) MemberNames(ctx context.Context, workspaceID string) (map[string]string, error) {
	readLogFrom(ctx).record("member names")
	rows, err := s.pool.Query(ctx, `
		SELECT wm.user_record_id, COALESCE(NULLIF(c.full_name, ''), wm.email)
		FROM workspace_members wm
		LEFT JOIN credentials c ON c.email = wm.email
		WHERE wm.workspace_id = $1
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("member names: %w", err)
	}
	defer rows.Close()

	names := map[string]string{}
	for rows.Next() {
		var recordID, name string
		if err := rows.Scan(&recordID, &name); err != nil {
			return nil, fmt.Errorf("scan member name: %w", err)
		}
		names[recordID] = name
	}
	return names, rows.Err()
}

// MarkEmailVerified sets a credential's EmailVerified to true, once its owner has proven they
// received the emailed link (development-history.md Phase 21 round 2, Step D).
func (s *Store) MarkEmailVerified(ctx context.Context, email string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE credentials SET email_verified = true WHERE email = $1`, email)
	if err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// CurrentSessionGeneration returns subject's current session generation (security audit
// 2026-09-19, M2) -- 0 for a subject that has never been bumped, which is not an error: every
// subject implicitly starts at generation 0 whether or not a row exists for it yet.
func (s *Store) CurrentSessionGeneration(ctx context.Context, subject string) (int, error) {
	readLogFrom(ctx).record("session generation")
	var gen int
	err := s.pool.QueryRow(ctx, `SELECT generation FROM session_generations WHERE subject = $1`, subject).Scan(&gen)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get session generation: %w", err)
	}
	return gen, nil
}

// BumpSessionGeneration invalidates every session cookie previously issued for subject: a cookie
// signed under an older generation number is rejected by requireAuth even though its own HMAC
// signature is still perfectly valid (security audit 2026-09-19, M2 -- logout and a successful
// password reset each call this for the subject(s) they affect). Upserts, since a subject's
// generation is implicitly 0 until its first bump ever creates a row for it.
func (s *Store) BumpSessionGeneration(ctx context.Context, subject string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO session_generations (subject, generation) VALUES ($1, 1)
		ON CONFLICT (subject) DO UPDATE SET generation = session_generations.generation + 1
	`, subject)
	if err != nil {
		return fmt.Errorf("bump session generation: %w", err)
	}
	return nil
}
