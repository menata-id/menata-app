package composition

import (
	"fmt"
	"slices"

	"menata.app/internal/domain"
)

// ProcessGraph is a Machine's lifecycle as the Application Builder draws it: the statuses of its lifecycle Field, the edges its `transitions:` declare between
// them, how many records stand in each status, and what is wrong with the shape. It is derived and never stored (001 #6): the one writer of a workflow
// stays `transitions:`, `status_categories:`, `events:` and `permissions:` (004; 007 §25), and this is only how they read together.
type ProcessGraph struct {
	// Field is the lifecycle status Field.
	Field string
	Nodes []ProcessNode
	Edges []ProcessEdge
	// Issues are the things worth a look about the shape, each naming the status it is about (Status "" for the graph as a whole).
	Issues []ProcessIssue
}

// ProcessNode is one status.
type ProcessNode struct {
	Status string
	// Category is the declared status_categories meaning (start, active, attention, done, stopped), "" when the Machine gives this status none.
	Category string
	// Count is how many records stand in the status now.
	Count int
	// Start says a new record begins here: the Field's default, else its first option.
	Start bool
	// Out and In are the edge ids leaving and entering the status.
	Out, In []string
}

// ProcessEdge is one declared transition.
type ProcessEdge struct {
	ID, Name, From, To string
	// Roles may take the edge; ActorField, when set, limits it to the person that Field of the record names. Neither says "anyone who may edit the record".
	Roles      []string
	ActorField string
	// Action is the Action that performs the edge, "" for one the runtime itself writes.
	Action string
}

// ProcessIssue is a finding about the shape of a workflow.
type ProcessIssue struct {
	Status string
	Kind   string
	Words  string
}

// The kinds of finding.
const (
	// ProcessDeadEnd is a status nothing leaves that is not a finish: records get there and stay.
	ProcessDeadEnd = "dead_end"
	// ProcessUnreachable is a status nothing enters that is not where a record begins.
	ProcessUnreachable = "unreachable"
)

// ResolveProcess reads m's lifecycle. ok is false for a Machine that has none: no status Field, or a status no transition governs (an ordinary choice list, not a
// workflow). counts is the number of records per status, nil when they are not known.
func ResolveProcess(m *domain.Machine, counts map[string]int) (ProcessGraph, bool) {
	field := m.StatusField()
	if field == "" {
		return ProcessGraph{}, false
	}
	var f *domain.Field
	for i := range m.Fields {
		if m.Fields[i].ID == field && m.Fields[i].Type == domain.FieldTypeStatus {
			f = &m.Fields[i]
		}
	}
	if f == nil {
		return ProcessGraph{}, false
	}
	g := ProcessGraph{Field: field}
	start := ""
	if d, ok := f.Default.(string); ok && d != "" {
		start = d
	} else if len(f.Options) > 0 {
		start = f.Options[0]
	}
	index := map[string]int{}
	for i, o := range f.Options {
		category := ""
		if c, ok := m.StatusCategories.CategoryOf(o); ok && m.StatusCategories.Field == field {
			category = string(c)
		}
		g.Nodes = append(g.Nodes, ProcessNode{Status: o, Category: category, Count: counts[o], Start: o == start || category == string(domain.StatusStart)})
		index[o] = i
	}
	for _, t := range m.Transitions {
		if t.Field != field {
			continue
		}
		g.Edges = append(g.Edges, ProcessEdge{ID: t.ID, Name: t.Name, From: t.From, To: t.To, Roles: slices.Clone(t.Roles), ActorField: t.ActorField, Action: t.Action})
		if i, ok := index[t.From]; ok {
			g.Nodes[i].Out = append(g.Nodes[i].Out, t.ID)
		}
		if i, ok := index[t.To]; ok {
			g.Nodes[i].In = append(g.Nodes[i].In, t.ID)
		}
	}
	for _, n := range g.Nodes {
		finish := n.Category == string(domain.StatusDone) || n.Category == string(domain.StatusStopped)
		if len(n.Out) == 0 && !finish {
			g.Issues = append(g.Issues, ProcessIssue{Status: n.Status, Kind: ProcessDeadEnd,
				Words: fmt.Sprintf("Nothing leaves %s, and it is not marked as a finish: records that get there stay there.", n.Status)})
		}
		if len(n.In) == 0 && !n.Start {
			g.Issues = append(g.Issues, ProcessIssue{Status: n.Status, Kind: ProcessUnreachable,
				Words: fmt.Sprintf("Nothing leads to %s, and a record does not begin there.", n.Status)})
		}
	}
	return g, true
}

// NewStatusChoice is the value a move's From/To picker sends for "the status being added in the same form".
const NewStatusChoice = "+new"

// LifecycleField is the status Field a Machine's workflow moves: the one its transitions move, else the Field its status_categories name, else its first status
// Field. nil when it has no status at all.
func LifecycleField(m *domain.Machine) *domain.Field {
	want := m.StatusField()
	if want == "" && m.StatusCategories != nil {
		want = m.StatusCategories.Field
	}
	for i := range m.Fields {
		if m.Fields[i].Type == domain.FieldTypeStatus && (want == "" || m.Fields[i].ID == want) {
			return &m.Fields[i]
		}
	}
	return nil
}
