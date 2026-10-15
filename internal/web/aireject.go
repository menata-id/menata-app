package web

import (
	"context"
	"log"
	"strings"

	"menata.app/internal/data"
)

// logAIReject writes one AI-REJECT line whenever metadata the assistant produced fails the runtime's
// own checks (the automatic validation, a publish-time validation, or the writer's load-verify). It is
// the mechanical twin of AI-GAP (roadmap stage 1, WG, owner direction 2026-10-15): AI-GAP is what the
// model *says* it could not do and depends on the model obeying; this is what the runtime *refused*,
// which needs nobody's cooperation. A refusal is evidence of a missing capability, a writing-guide
// that did not tell the model the rule, or a grammar the model cannot hit -- the development process
// reads these lines to decide which.
//
// final= tells noise from signal: "pending" is a first refusal the model is about to be asked to
// correct, "recovered" a correction that then validated, "unresolved" one that did not, and "returned"
// a publish-time refusal sent back to the conversation. A rejection that is always recovered is a
// prompt problem; one that is never recovered is a grammar or capability problem.
func logAIReject(ctx context.Context, sessionID, stage, final string, attempt int, problem error) {
	workspaceID, _ := data.WorkspaceScope(ctx)
	issue := ""
	if problem != nil {
		issue = strings.Join(strings.Fields(problem.Error()), " ")
		if len(issue) > 600 {
			issue = issue[:600] + "..."
		}
	}
	log.Printf("AI-REJECT stage=%s final=%s attempt=%d workspace=%s session=%s issue=%q", stage, final, attempt, workspaceID, sessionID, issue)
}
