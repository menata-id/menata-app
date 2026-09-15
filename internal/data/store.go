package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

// CreateRecord inserts a new Record for the given Machine. Callers must validate values with
// ValidateRecord first -- the store does not know Domain Plane rules.
func (s *Store) CreateRecord(ctx context.Context, machineID string, values map[string]any) (*Record, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal record values: %w", err)
	}

	r := &Record{ID: newRecordID(), MachineID: machineID, Values: values}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO records (id, machine_id, data)
		VALUES ($1, $2, $3::jsonb)
		RETURNING created_at, updated_at
	`, r.ID, r.MachineID, data)

	if err := row.Scan(&r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert record: %w", err)
	}
	return r, nil
}

// ListRecords returns every Record for the given Machine, most recently created first.
func (s *Store) ListRecords(ctx context.Context, machineID string) ([]*Record, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, machine_id, data, created_at, updated_at
		FROM records
		WHERE machine_id = $1
		ORDER BY created_at DESC
	`, machineID)
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}
	defer rows.Close()

	var records []*Record
	for rows.Next() {
		r := &Record{}
		var data []byte
		if err := rows.Scan(&r.ID, &r.MachineID, &data, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		if err := json.Unmarshal(data, &r.Values); err != nil {
			return nil, fmt.Errorf("unmarshal record %s: %w", r.ID, err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
