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

// errNotScoped is returned by a record-scoped method called on a Store that was never given a
// Workspace via WithWorkspace -- a forgotten scope should fail loudly (005 SS Security Ordering:
// scope must be established before retrieval, not defaulted silently) rather than silently read
// or write across every Workspace.
var errNotScoped = errors.New("data: Store has no Workspace scope -- call WithWorkspace first")

// Store is the Data Plane's physical execution against PostgreSQL for the generic `records`
// table. It is intentionally narrow: create and list by Machine, no Query/Projection/Filter
// composition yet (007-composable-runtime-architecture.md SS7-8 -- those land once more than one
// consumer needs them).
//
// Every record-scoped method (CreateRecord/ListRecords/ListRecordsBy/GetRecord/UpdateRecord/
// DeleteRecord) is additionally scoped to one Workspace (ROADMAP.md Phase 21 Step 2 -- "Workspace
// never enters the data path" closed). A Store returned by NewStore is unscoped and can only be
// used for Workspace/credential/membership methods, which are not per-Workspace themselves;
// WithWorkspace returns a copy scoped to one Workspace for everything else. Scoping through the
// Store value itself, rather than a parameter on every call, means the ~40 existing call sites
// across internal/web and internal/composition need no signature change at all -- only the one
// place that constructs a request's Store value needs to know the Workspace.
type Store struct {
	pool        *pgxpool.Pool
	workspaceID string
}

// NewStore wraps an existing connection pool. The result is unscoped -- see WithWorkspace.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// WithWorkspace returns a copy of s scoped to workspaceID. Every record-scoped method called on
// the result reads and writes only that Workspace's records.
func (s *Store) WithWorkspace(workspaceID string) *Store {
	scoped := *s
	scoped.workspaceID = workspaceID
	return &scoped
}

// CreateRecord inserts a new Record for the given Machine, at the next sort_order after every
// existing record of that Machine in this Store's Workspace (ROADMAP.md Phase 9: child
// collections need a meaningful order). Callers must validate values with ValidateRecord first --
// the store does not know Domain Plane rules.
func (s *Store) CreateRecord(ctx context.Context, machineID string, values map[string]any) (*Record, error) {
	if s.workspaceID == "" {
		return nil, errNotScoped
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal record values: %w", err)
	}

	r := &Record{ID: newRecordID(), MachineID: machineID, WorkspaceID: s.workspaceID, Values: values}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO records (id, machine_id, workspace_id, data, sort_order)
		VALUES ($1, $2, $3, $4::jsonb, COALESCE((SELECT MAX(sort_order) FROM records WHERE machine_id = $2 AND workspace_id = $3), 0) + 1)
		RETURNING sort_order, created_at, updated_at
	`, r.ID, r.MachineID, s.workspaceID, data)

	if err := row.Scan(&r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert record: %w", err)
	}
	return r, nil
}

// ListRecords returns every Record for the given Machine in this Store's Workspace, in sort_order.
func (s *Store) ListRecords(ctx context.Context, machineID string) ([]*Record, error) {
	if s.workspaceID == "" {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record(machineID)
	return s.queryRecords(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND workspace_id = $2
		ORDER BY sort_order ASC, created_at ASC
	`, machineID, s.workspaceID)
}

// ListRecordsBy returns every Record of machineID in this Store's Workspace whose fieldID value
// equals value, in sort_order -- the query behind a child collection (ROADMAP.md Phase 9): fieldID
// is a reference field on machineID pointing back to another record (value = that record's id).
func (s *Store) ListRecordsBy(ctx context.Context, machineID, fieldID, value string) ([]*Record, error) {
	if s.workspaceID == "" {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record(machineID + " by " + fieldID)
	return s.queryRecords(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND data->>$2 = $3 AND workspace_id = $4
		ORDER BY sort_order ASC, created_at ASC
	`, machineID, fieldID, value, s.workspaceID)
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
	if s.workspaceID == "" {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record(machineID + " by id")
	row := s.pool.QueryRow(ctx, `
		SELECT id, machine_id, workspace_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND id = $2 AND workspace_id = $3
	`, machineID, id, s.workspaceID)

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
	if s.workspaceID == "" {
		return nil, errNotScoped
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal record values: %w", err)
	}

	r := &Record{ID: id, MachineID: machineID, WorkspaceID: s.workspaceID, Values: values}
	row := s.pool.QueryRow(ctx, `
		UPDATE records
		SET data = $3::jsonb, updated_at = NOW()
		WHERE machine_id = $1 AND id = $2 AND workspace_id = $4
		RETURNING sort_order, created_at, updated_at
	`, machineID, id, data, s.workspaceID)

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
	if s.workspaceID == "" {
		return errNotScoped
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM records WHERE machine_id = $1 AND id = $2 AND workspace_id = $3`, machineID, id, s.workspaceID)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	return nil
}

// CreateCredential stores a login credential's hashed password, keyed by email (ROADMAP.md
// Phase 21 Step 1). Hashing is internal/authorization's job -- the store only persists whatever
// hash it is given.
func (s *Store) CreateCredential(ctx context.Context, email, passwordHash string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO credentials (email, password_hash) VALUES ($1, $2)
	`, email, passwordHash)
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

// GetCredential returns the stored password hash for email, or ErrCredentialNotFound.
func (s *Store) GetCredential(ctx context.Context, email string) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM credentials WHERE email = $1`, email).Scan(&hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrCredentialNotFound
		}
		return "", fmt.Errorf("get credential: %w", err)
	}
	return hash, nil
}
