package composition

import (
	"context"
	"fmt"

	"menata.app/internal/behavior"
	"menata.app/internal/data"
)

// BlockingReasonsForMemberRemoval answers "may userRecordID be deactivated out of this
// Workspace?" by sweeping every currently-loaded Machine that declares at least one
// blocks_member_removal entry (Flow 2 canvas re-audit, ROADMAP.md, 2026-09-27) -- today, only
// mch_approval_step's own pending-step guard, but this walks every Machine so a second Machine
// declaring one needs no Go code to be honored. The pure decision (does a fetched record actually
// match) lives in behavior.MatchedMemberRemovalBlocks; this is only the I/O half, the same split
// every other Event/Constraint dispatch in this codebase already takes.
//
// One query per distinct actor_field a Machine's own blocks name (l.ListRecordsBy, the same
// indexed reference-field lookup rollUpParentStatus's own sibling read already uses) -- a Machine
// declaring none costs nothing here.
func BlockingReasonsForMemberRemoval(ctx context.Context, l *Loader, userRecordID string) ([]string, error) {
	var reasons []string
	for _, m := range l.machineSlice() {
		if len(m.MemberRemovalBlocks) == 0 {
			continue
		}
		actorFields := map[string]bool{}
		var candidates []*data.Record
		for _, block := range m.MemberRemovalBlocks {
			if actorFields[block.ActorField] {
				continue
			}
			actorFields[block.ActorField] = true
			records, err := l.ListRecordsBy(ctx, m.ID, block.ActorField, userRecordID)
			if err != nil {
				return nil, fmt.Errorf("blocking reasons for member removal: %w", err)
			}
			candidates = append(candidates, records...)
		}
		for _, block := range behavior.MatchedMemberRemovalBlocks(m, userRecordID, candidates) {
			reasons = append(reasons, block.Reason)
		}
	}
	return reasons, nil
}
