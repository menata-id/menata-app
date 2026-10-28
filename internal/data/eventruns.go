package data

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// The outcomes of one run of a rule: it did its work, it ran and had nobody or nothing to act on (quiet), or it failed.
const (
	EventRanOutcome    = "ran"
	EventQuietOutcome  = "quiet"
	EventFailedOutcome = "failed"
)

// eventRunsKept is how many runs of one rule are kept; older ones are trimmed when a new one is written.
const eventRunsKept = 200

// EventRun is one run of a rule.
type EventRun struct {
	MachineID, EventID, RecordID, Outcome, Detail string
	RanAt                                         time.Time
}

// RecordEventRun writes one run of a rule and trims the Machine's oldest.
func (s *Store) RecordEventRun(ctx context.Context, machineID, eventID, recordID, outcome, detail string) error {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return errNotScoped
	}
	detail = cleanDetail(detail, 500)
	if _, err := s.q(ctx).Exec(ctx, `INSERT INTO event_runs (workspace_id, machine_id, event_id, record_id, outcome, detail) VALUES ($1, $2, $3, $4, $5, $6)`,
		workspaceID, machineID, eventID, recordID, outcome, detail); err != nil {
		return fmt.Errorf("record event run: %w", err)
	}
	// Trimmed per rule, not per Machine: a busy rule must not push a rare one's last run out, which is the question this table exists to answer.
	_, err := s.q(ctx).Exec(ctx, `DELETE FROM event_runs WHERE workspace_id = $1 AND machine_id = $2 AND event_id = $3 AND id NOT IN
		(SELECT id FROM event_runs WHERE workspace_id = $1 AND machine_id = $2 AND event_id = $3 ORDER BY id DESC LIMIT $4)`, workspaceID, machineID, eventID, eventRunsKept)
	if err != nil {
		return fmt.Errorf("trim event runs: %w", err)
	}
	return nil
}

// RecentEventRuns are the newest runs of one rule, newest first.
func (s *Store) RecentEventRuns(ctx context.Context, machineID, eventID string, limit int) ([]EventRun, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record("event runs " + eventID)
	rows, err := s.q(ctx).Query(ctx, `SELECT machine_id, event_id, record_id, outcome, detail, ran_at FROM event_runs
		WHERE workspace_id = $1 AND machine_id = $2 AND event_id = $3 ORDER BY id DESC LIMIT $4`, workspaceID, machineID, eventID, limit)
	if err != nil {
		return nil, fmt.Errorf("list event runs: %w", err)
	}
	defer rows.Close()
	var out []EventRun
	for rows.Next() {
		var r EventRun
		if err := rows.Scan(&r.MachineID, &r.EventID, &r.RecordID, &r.Outcome, &r.Detail, &r.RanAt); err != nil {
			return nil, fmt.Errorf("scan event run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestEventRuns is the newest run of each rule of the given Machines, keyed "machine/event", for a list that says when each last ran.
func (s *Store) LatestEventRuns(ctx context.Context, machineIDs []string) (map[string]EventRun, error) {
	workspaceID, ok := workspaceScopeFrom(ctx)
	if !ok {
		return nil, errNotScoped
	}
	readLogFrom(ctx).record("latest event runs")
	rows, err := s.q(ctx).Query(ctx, `SELECT DISTINCT ON (machine_id, event_id) machine_id, event_id, record_id, outcome, detail, ran_at FROM event_runs
		WHERE workspace_id = $1 AND machine_id = ANY($2) ORDER BY machine_id, event_id, id DESC`, workspaceID, machineIDs)
	if err != nil {
		return nil, fmt.Errorf("latest event runs: %w", err)
	}
	defer rows.Close()
	out := map[string]EventRun{}
	for rows.Next() {
		var r EventRun
		if err := rows.Scan(&r.MachineID, &r.EventID, &r.RecordID, &r.Outcome, &r.Detail, &r.RanAt); err != nil {
			return nil, fmt.Errorf("scan latest event run: %w", err)
		}
		out[r.MachineID+"/"+r.EventID] = r
	}
	return out, rows.Err()
}

// cleanDetail makes a line fit a column: no NUL (PostgreSQL text refuses it) and at most max bytes, cut on a character boundary so a name in any script is never split.
func cleanDetail(s string, max int) string {
	s = strings.ReplaceAll(s, "\x00", "")
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}
