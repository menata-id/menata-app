package web

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"menata.app/internal/aiassist"
	"menata.app/internal/data"
)

// publishedPlan is the plan of an update_application, computed before the write (afterwards the installed
// state already contains it). It is empty for a new application.
func publishedPlan(change aiassist.GeneratedChange, existing aiassist.ExistingState) (aiassist.Plan, bool) {
	if change.Kind != aiassist.KindUpdateApplication || change.Application == nil {
		return aiassist.Plan{}, false
	}
	cur, ok := existing.Applications[change.TargetAppID]
	if !ok {
		return aiassist.Plan{}, false
	}
	return aiassist.PlanUpdate(aiassist.DescribeApplication(change.TargetAppID, cur), *change.Application), true
}

// logAIUpdate writes one AI-UPDATE line when an update_application is published (roadmap stage 1, WU).
// What people change in an application soon after the assistant built it is the evidence of what its first
// generation lacked or overdid: adds soon after creation mean something was missing, removes mean something was
// too much. age_hours is how long the application had existed since the assistant published it, or
// "unknown" for one the assistant did not create (a library install, a hand-written file) or one published
// before migration 018 -- unknown is never reported as zero.
func logAIUpdate(workspaceID, sessionID, appID string, origin time.Time, haveOrigin bool, plan aiassist.Plan, now time.Time) {
	age := "unknown"
	if haveOrigin {
		age = formatHours(now.Sub(origin))
	}
	var adds, changes, removes int
	var what []string
	for _, it := range plan.Items {
		switch it.Op {
		case "add":
			adds++
		case "change":
			changes++
		case "remove":
			removes++
		}
		what = append(what, it.Op+": "+it.What)
	}
	summary := strings.Join(what, "; ")
	if len(summary) > 600 {
		summary = summary[:600] + "..."
	}
	log.Printf("AI-UPDATE app=%s workspace=%s session=%s age_hours=%s adds=%d changes=%d removes=%d plan=%q", appID, workspaceID, sessionID, age, adds, changes, removes, summary)
}

func formatHours(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return strconv.FormatFloat(d.Hours(), 'f', 1, 64)
}

// markPublished records the publication and, for an update, writes its AI-UPDATE line.
func markPublished(ctx context.Context, store *data.Store, workspaceID, sessionID string, change aiassist.GeneratedChange, newAppID string, plan aiassist.Plan, isUpdate bool) error {
	appID := newAppID
	if isUpdate {
		appID = change.TargetAppID
	}
	var origin time.Time
	var have bool
	if isUpdate {
		var err error
		if origin, have, err = store.AIApplicationOrigin(ctx, workspaceID, appID); err != nil {
			return err
		}
	}
	if err := store.MarkAISessionPublished(ctx, workspaceID, sessionID, appID); err != nil {
		return err
	}
	if isUpdate {
		logAIUpdate(workspaceID, sessionID, appID, origin, have, plan, time.Now())
	}
	return nil
}
