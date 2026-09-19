package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
// DeleteRecord) is additionally scoped to one Workspace (ROADMAP.md Phase 21 Step 2 -- "Workspace
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
// existing record of that Machine in this Store's Workspace (ROADMAP.md Phase 9: child
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

// ListRecordsBy returns every Record of machineID in this Store's Workspace whose fieldID value
// equals value, in sort_order -- the query behind a child collection (ROADMAP.md Phase 9): fieldID
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
type Credential struct {
	Email         string
	PasswordHash  string
	EmailVerified bool
}

// CreateCredential stores a login credential's hashed password, keyed by email (ROADMAP.md
// Phase 21 Step 1). Hashing is internal/authorization's job -- the store only persists whatever
// hash it is given. emailVerified is set by the caller, not assumed: registration's own credential
// starts unverified, an invite's activation starts verified.
func (s *Store) CreateCredential(ctx context.Context, email, passwordHash string, emailVerified bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO credentials (email, password_hash, email_verified) VALUES ($1, $2, $3)
	`, email, passwordHash, emailVerified)
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

// GetCredential returns the stored credential for email, or ErrCredentialNotFound.
func (s *Store) GetCredential(ctx context.Context, email string) (*Credential, error) {
	cred := &Credential{Email: email}
	err := s.pool.QueryRow(ctx, `SELECT password_hash, email_verified FROM credentials WHERE email = $1`, email).Scan(&cred.PasswordHash, &cred.EmailVerified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("get credential: %w", err)
	}
	return cred, nil
}

// MarkEmailVerified sets a credential's EmailVerified to true, once its owner has proven they
// received the emailed link (ROADMAP.md Phase 21 round 2, Step D).
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
