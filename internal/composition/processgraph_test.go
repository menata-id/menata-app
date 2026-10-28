package composition

import (
	"testing"

	"menata.app/internal/domain"
)

func flowMachine() *domain.Machine {
	return &domain.Machine{ID: "mch_x", Fields: []domain.Field{
		{ID: "fld_status", Name: "Status", Type: domain.FieldTypeStatus, Default: "Draft", Options: []string{"Draft", "Review", "Approved", "Rejected", "Orphan", "Stuck"}},
	}, Transitions: []domain.Transition{
		{ID: "trn_submit", Name: "Submit", Field: "fld_status", From: "Draft", To: "Review", Roles: []string{"author"}},
		{ID: "trn_approve", Name: "Approve", Field: "fld_status", From: "Review", To: "Approved", Roles: []string{"manager"}},
		{ID: "trn_reject", Name: "Reject", Field: "fld_status", From: "Review", To: "Rejected"},
		{ID: "trn_stick", Name: "Park", Field: "fld_status", From: "Review", To: "Stuck"},
	}, StatusCategories: &domain.StatusCategories{Field: "fld_status", Values: map[string]domain.StatusCategory{
		"Draft": domain.StatusStart, "Review": domain.StatusActive, "Approved": domain.StatusDone, "Rejected": domain.StatusStopped,
	}}}
}

// A workflow reads as its statuses with their meaning and count, its edges with who takes them, and what is wrong with the shape: a status nothing leaves that is not a
// finish (a dead end) and one nothing enters that is not where a record begins.
func TestResolveProcessReadsStatusesEdgesAndTheShapesProblems(t *testing.T) {
	g, ok := ResolveProcess(flowMachine(), map[string]int{"Draft": 2, "Review": 5})
	if !ok || g.Field != "fld_status" || len(g.Nodes) != 6 || len(g.Edges) != 4 {
		t.Fatalf("graph = %+v ok=%v", g, ok)
	}
	if n := g.Nodes[1]; n.Status != "Review" || n.Category != "active" || n.Count != 5 || len(n.In) != 1 || len(n.Out) != 3 || n.Start {
		t.Errorf("Review = %+v", n)
	}
	if !g.Nodes[0].Start || g.Nodes[2].Start {
		t.Errorf("a record begins at Draft only: %+v %+v", g.Nodes[0], g.Nodes[2])
	}
	if e := g.Edges[1]; e.Name != "Approve" || e.From != "Review" || e.To != "Approved" || len(e.Roles) != 1 || e.Roles[0] != "manager" {
		t.Errorf("edge = %+v", e)
	}
	kinds := map[string]string{}
	for _, i := range g.Issues {
		kinds[i.Status] = i.Kind
	}
	// Approved and Rejected are finishes (no way out is right); Stuck is a dead end, and Orphan has neither way in nor out and is no start.
	if kinds["Stuck"] != ProcessDeadEnd || kinds["Orphan"] == "" || len(g.Issues) != 3 {
		t.Errorf("issues = %+v", g.Issues)
	}
	for _, i := range g.Issues {
		if i.Status == "Approved" || i.Status == "Rejected" || i.Status == "Draft" || i.Status == "Review" {
			t.Errorf("a sound status was flagged: %+v", i)
		}
	}
}

// A status no transition governs is a choice list, not a workflow: there is nothing to draw.
func TestResolveProcessIsAbsentWhereThereIsNoWorkflow(t *testing.T) {
	m := &domain.Machine{ID: "mch_x", Fields: []domain.Field{{ID: "fld_kind", Type: domain.FieldTypeStatus, Options: []string{"A", "B"}}}}
	if _, ok := ResolveProcess(m, nil); ok {
		t.Error("a choice list was read as a workflow")
	}
}
