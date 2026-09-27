package behavior

import (
	"fmt"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// MatchedMemberRemovalBlocks returns every domain.MemberRemovalBlock declared on m that currently
// blocks userRecordID from being deactivated out of the Workspace: a record among records whose
// ActorField names userRecordID, with Condition matching that record's own values. Pure -- no I/O,
// the same posture MatchedEvents/MatchedScheduleEvents already take; the caller fetches records.
func MatchedMemberRemovalBlocks(m *domain.Machine, userRecordID string, records []*data.Record) []domain.MemberRemovalBlock {
	var out []domain.MemberRemovalBlock
	for _, b := range m.MemberRemovalBlocks {
		for _, r := range records {
			if fmt.Sprint(r.Values[b.ActorField]) != userRecordID {
				continue
			}
			if !b.Condition.Evaluate(r.Values) {
				continue
			}
			out = append(out, b)
			break
		}
	}
	return out
}
