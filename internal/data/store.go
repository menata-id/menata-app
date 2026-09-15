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

// Store is the Data Plane's physical execution against PostgreSQL for the generic `records`
// table. It is intentionally narrow: create and list by Machine, no Query/Projection/Filter
// composition yet (007-composable-runtime-architecture.md SS7-8 -- those land once more than one
// consumer needs them).
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps an existing connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateRecord inserts a new Record for the given Machine, at the next sort_order after every
// existing record of that Machine (ROADMAP.md Phase 9: child collections need a meaningful
// order). Callers must validate values with ValidateRecord first -- the store does not know
// Domain Plane rules.
func (s *Store) CreateRecord(ctx context.Context, machineID string, values map[string]any) (*Record, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal record values: %w", err)
	}

	r := &Record{ID: newRecordID(), MachineID: machineID, Values: values}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO records (id, machine_id, data, sort_order)
		VALUES ($1, $2, $3::jsonb, COALESCE((SELECT MAX(sort_order) FROM records WHERE machine_id = $2), 0) + 1)
		RETURNING sort_order, created_at, updated_at
	`, r.ID, r.MachineID, data)

	if err := row.Scan(&r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert record: %w", err)
	}
	return r, nil
}

// ListRecords returns every Record for the given Machine, in sort_order.
func (s *Store) ListRecords(ctx context.Context, machineID string) ([]*Record, error) {
	return s.queryRecords(ctx, `
		SELECT id, machine_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1
		ORDER BY sort_order ASC, created_at ASC
	`, machineID)
}

// ListRecordsBy returns every Record of machineID whose fieldID value equals value, in
// sort_order -- the query behind a child collection (ROADMAP.md Phase 9): fieldID is a
// reference field on machineID pointing back to another record (value = that record's id).
func (s *Store) ListRecordsBy(ctx context.Context, machineID, fieldID, value string) ([]*Record, error) {
	return s.queryRecords(ctx, `
		SELECT id, machine_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND data->>$2 = $3
		ORDER BY sort_order ASC, created_at ASC
	`, machineID, fieldID, value)
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
		if err := rows.Scan(&r.ID, &r.MachineID, &data, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		if err := json.Unmarshal(data, &r.Values); err != nil {
			return nil, fmt.Errorf("unmarshal record %s: %w", r.ID, err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetRecord returns one Record by ID, scoped to the given Machine.
func (s *Store) GetRecord(ctx context.Context, machineID, id string) (*Record, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, machine_id, data, sort_order, created_at, updated_at
		FROM records
		WHERE machine_id = $1 AND id = $2
	`, machineID, id)

	r := &Record{}
	var data []byte
	if err := row.Scan(&r.ID, &r.MachineID, &data, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
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

// UpdateRecord replaces a Record's values. Callers must validate values with ValidateRecord
// first, same as CreateRecord.
func (s *Store) UpdateRecord(ctx context.Context, machineID, id string, values map[string]any) (*Record, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal record values: %w", err)
	}

	r := &Record{ID: id, MachineID: machineID, Values: values}
	row := s.pool.QueryRow(ctx, `
		UPDATE records
		SET data = $3::jsonb, updated_at = NOW()
		WHERE machine_id = $1 AND id = $2
		RETURNING sort_order, created_at, updated_at
	`, machineID, id, data)

	if err := row.Scan(&r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("update record: %w", err)
	}
	return r, nil
}

// DeleteRecord removes a Record. It is not an error to delete an already-absent record.
func (s *Store) DeleteRecord(ctx context.Context, machineID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM records WHERE machine_id = $1 AND id = $2`, machineID, id)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	return nil
}
