package registry

import "sort"

// RuntimeAction is one operation the runtime performs on its own state (its groups, members, installation) that is not a
// write of a Machine's record: create a group, remove one. A page of the runtime's own names it with
// `binding: {write: action, action: <id>}` on a Form or a Button, and the runtime's route performs it.
//
// It is a dispatch seam like `Services`: the closed set, what each action asks and who may do it live here; the executor
// that touches the database lives in `internal/web`, and `TestRuntimeActionsAndRunnersAgree` is the only thing binding
// the two. A page never says how an action is done, only which one, so what the action may do is bounded by this list
// and not by what an author writes.
type RuntimeAction struct {
	ID string
	// Inputs are what a Form for this action asks, in order. A Button action asks nothing.
	Inputs []ActionInput
	// Source, when set, makes the action about one record of that runtime source (registry.RuntimeSources): it is valid only
	// on a Button inside a Collection of that source's Machine, and acts on that record. Empty means the action is not about
	// a record (create one).
	Source string
	// Admin says only a Workspace admin may perform it. Every action is, today; the field is here so a member-level
	// action is a statement and not an absence.
	Admin bool
	Doc   string
}

// ActionInput is one thing a Form action asks of the person.
type ActionInput struct {
	Name     string
	Label    string
	Required bool
}

// RuntimeActions is the closed set a binding's `action:` may name.
var RuntimeActions = map[string]RuntimeAction{
	"create_group": {ID: "create_group", Inputs: []ActionInput{{Name: "name", Label: "Group name", Required: true}}, Admin: true, Doc: "adds a Group to the Workspace"},
	"delete_group": {ID: "delete_group", Source: "groups", Admin: true, Doc: "removes a Group and what it granted"},
}

// RuntimeActionIDs lists the set, sorted, for messages.
func RuntimeActionIDs() []string {
	ids := make([]string, 0, len(RuntimeActions))
	for id := range RuntimeActions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
