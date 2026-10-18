package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Extraction is one document a reader proposed values for: the stored file, the proposal (opaque here -- internal/web owns
// its shape) and who asked on which page.
type Extraction struct {
	ID          string
	WorkspaceID string
	UserID      string
	NavID       string
	MachineID   string
	FileKey     string
	FileName    string
	Proposal    json.RawMessage
	Status      string
	RecordID    string
}

// Extraction statuses: reading (the document was sent, no answer yet) -> proposed (an answer a person may check) or failed;
// proposed -> saved.
const (
	ExtractionReading  = "reading"
	ExtractionProposed = "proposed"
	ExtractionSaved    = "saved"
	ExtractionFailed   = "failed"
)

// StartExtraction records that a document is being read, before the reader is asked, so every attempt is counted -- the
// failed ones too. The id is generated here; the proposal is empty until FinishExtraction.
func (s *Store) StartExtraction(ctx context.Context, e Extraction) (*Extraction, error) {
	e.ID, e.Status, e.Proposal = newID("ext_"), ExtractionReading, []byte(`{}`)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO document_extractions (id, workspace_id, user_id, nav_id, machine_id, file_key, file_name, proposal, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.ID, e.WorkspaceID, e.UserID, e.NavID, e.MachineID, e.FileKey, e.FileName, []byte(e.Proposal), e.Status)
	if err != nil {
		return nil, fmt.Errorf("create extraction: %w", err)
	}
	return &e, nil
}

// FinishExtraction stores the reader's answer and makes the reading a proposal a person may check.
func (s *Store) FinishExtraction(ctx context.Context, workspaceID, id string, proposal json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `UPDATE document_extractions SET proposal = $3, status = $4 WHERE id = $1 AND workspace_id = $2 AND status = $5`,
		id, workspaceID, []byte(proposal), ExtractionProposed, ExtractionReading)
	return err
}

// FailExtraction marks a reading that gave no answer.
func (s *Store) FailExtraction(ctx context.Context, workspaceID, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE document_extractions SET status = $3 WHERE id = $1 AND workspace_id = $2 AND status = $4`,
		id, workspaceID, ExtractionFailed, ExtractionReading)
	return err
}

// GetExtraction reads one proposal, in the Workspace the caller names: another Workspace's id is not found.
func (s *Store) GetExtraction(ctx context.Context, workspaceID, id string) (*Extraction, error) {
	readLogFrom(ctx).record("document extraction")
	var e Extraction
	err := s.q(ctx).QueryRow(ctx, `
		SELECT id, workspace_id, user_id, nav_id, machine_id, file_key, file_name, proposal, status, record_id
		FROM document_extractions WHERE id = $1 AND workspace_id = $2`, id, workspaceID).
		Scan(&e.ID, &e.WorkspaceID, &e.UserID, &e.NavID, &e.MachineID, &e.FileKey, &e.FileName, &e.Proposal, &e.Status, &e.RecordID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get extraction: %w", err)
	}
	return &e, nil
}

// MarkExtractionSaved records that the form was submitted and what it made. It only moves a proposal that is still
// proposed, so two submissions of one proposal cannot both win: the second finds no row and is told so.
func (s *Store) MarkExtractionSaved(ctx context.Context, workspaceID, id, recordID string) (bool, error) {
	tag, err := s.q(ctx).Exec(ctx, `
		UPDATE document_extractions SET status = $3, record_id = $4, saved_at = NOW()
		WHERE id = $1 AND workspace_id = $2 AND status = $5`, id, workspaceID, ExtractionSaved, recordID, ExtractionProposed)
	if err != nil {
		return false, fmt.Errorf("mark extraction saved: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// CountExtractions is how many documents a Workspace has sent to be read since the given time, failed attempts included: the
// figure a usage cap reads.
func (s *Store) CountExtractions(ctx context.Context, workspaceID string, since time.Time) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM document_extractions WHERE workspace_id = $1 AND created_at >= to_timestamp($2)`,
		workspaceID, since.Unix()).Scan(&n)
	return n, err
}
