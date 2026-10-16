package domain

// Transition is one declared edge in a Machine's own state model: a named move of one status
// Field from one of its declared options to another, and which Action is allowed to perform it
// (ROADMAP.md Case 03 Fase 7).
//
// It is `menata-runtime`'s Process Overlay `transitions[]` implemented rather than re-decided --
// upstream's shipped compiler (`internal/metadata/compile.go`) turns exactly `{name, from, to}`
// plus an actor into one guarded Event per edge, and its CAP-W05 process map reads that same
// shape back out. Two deliberate divergences, both because the *shipped* shape of this runtime
// differs from that one and the shipped shape is what runs:
//
//   - Upstream compiles a transition into an Event. Here it does not, because a domain.Event is a
//     post-write notification ("something already happened", see Event above) rather than a
//     triggerable operation -- compiling an edge into one would produce a declaration that
//     notifies but never guards. A Transition is therefore its own primitive, evaluated where the
//     write happens (behavior.CheckTransitions).
//   - Upstream's `actor: {role, owner_field}` carries both halves of authorization inline. Here
//     Action does, and it names one of KnownActions: *which* records an actor may move is already
//     said, more richly than upstream can say it, by the Permission governing that Action
//     (actor_field plus CAP-F24's dynamic gate). Restating a role here would make a Permission's
//     Roles and a Transition's actor two sources for one answer -- 001 Principle #8. So a
//     Transition answers "which edges exist, and through which Action", and Permission stays the
//     single authority on who may take one (metadata/applications/document-approval.yaml's own
//     comment already insists on that).
type Transition struct {
	ID string
	// Name is what this edge is called in the business ("Approve", "Reject"). It is the row label
	// on the Approval Role Matrix and is never derived from From/To, which read as storage values.
	Name string
	// Field is the status Field on this Machine that this transition moves.
	Field string
	// From and To are values of that Field's own declared options.
	From string
	To   string
	// Action names which of KnownActions may perform this edge, or "" for an edge no human Action
	// performs -- one the runtime itself writes, today only through a declared Event's own
	// rollup_parent_status Service. A "" Action is still declared rather than omitted: it is a
	// real edge of the state model, it is what the Approval Role Matrix renders as "System", and
	// leaving it out would make the same write look like an undeclared transition and be refused.
	//
	// An edge whose Action is set is refused through every *other* Action's route, which is how
	// one declaration replaced internal/web's own allowsDecisionChange -- a hand-written rule
	// naming one Machine and one Field that said exactly this for fld_decision alone.
	Action string
	// Roles are Application roles, any one of which the actor must hold to take this edge (the Permission
	// `roles:` arm, read per edge). Empty says nothing about roles. Only an edge performed by the `edit` Action
	// may declare it: that is the route the gate is enforced on, and a key enforced nowhere would read as a
	// guard and protect nothing.
	Roles []string
	// ActorField, when set, is a person Field of this Machine: the actor must be the person the record holds in
	// it (the Permission `actor_field` arm, read per edge). ANDs with Roles, like every arm. A record whose
	// field is empty satisfies nothing: an unassigned record is takeable by no one on this edge.
	ActorField string
	// Sets are Fields this edge writes when it is taken, each from a clock source in KnownTransitionSources: field id to
	// source. The write overrides whatever the form carried, so "the date the task was completed" is the day it was, not
	// a day someone typed. Only an edge performed by the `edit` Action may declare it, like Roles.
	Sets map[string]string
	// Generates makes taking this edge create a schedule of records of another Machine: see GenerateSchedule. Only an
	// edge performed by the `edit` Action may declare it.
	Generates *GenerateSchedule
}

// MaxGeneratedRecords bounds how many records one edge may generate, so a mistyped tenor cannot write thousands of rows.
const MaxGeneratedRecords = 120

// GenerateSchedule is what an edge writes besides the status move: Count records of Machine, each pointing back at the
// record that took the edge (LinkField, a relation Field of Machine), the i-th due i*DueEveryDays days from the day the
// edge is taken (DueField, a date) and carrying an even share of AmountFrom (AmountField, a number; the last takes the
// rounding remainder, so the shares add up exactly). Count is read from CountField, a number Field of this record. It
// is the repayment schedule of a loan, the instalments of a subscription, the visits of a contract.
//
// The runtime writes the records, so the target Machine's create Permission is not asked: the edge is the authority. An
// edge refuses to generate twice for one record (the schedule is not rewritten), and every row is validated before the
// status moves, so a schedule that cannot be written refuses the move rather than leaving half of it.
type GenerateSchedule struct {
	Machine      string
	CountField   string
	LinkField    string
	DueField     string
	DueEveryDays int
	AmountField  string
	AmountFrom   string
}

// TransitionSourceToday is the day the edge is taken, as a date.
const TransitionSourceToday = "today"

// KnownTransitionSources is the closed set of values a Transition.Sets entry may be written from.
var KnownTransitionSources = map[string]bool{TransitionSourceToday: true}

// TransitionFor returns the declared Transition moving field from -> to, if m declares one.
//
// Used both as the write gate (behavior.CheckTransitions) and as the Approval Role Matrix's own
// row source, so "this edge exists" means the same thing to the guard and to the screen that
// draws it -- the property authorization.AllowsAction's own doc comment describes for buttons and
// POSTs, applied to the state model.
func (m *Machine) TransitionFor(field, from, to string) (Transition, bool) {
	for _, t := range m.Transitions {
		if t.Field == field && t.From == from && t.To == to {
			return t, true
		}
	}
	return Transition{}, false
}

// GovernsTransitions reports whether m declares any Transition on field. A Field no Transition
// mentions is unrestricted -- metadata describes exceptions, not defaults (001 Principle #6) --
// which is what lets this primitive be declared on one Machine's one Field without freezing every
// other status Field in the manifest.
func (m *Machine) GovernsTransitions(field string) bool {
	for _, t := range m.Transitions {
		if t.Field == field {
			return true
		}
	}
	return false
}

// TransitionsFrom returns every declared Transition leaving field's value `from`, in declaration
// order. It answers "what can happen next", which is what a screen asks when deciding whether to
// offer an action at all -- replacing a hardcoded comparison against one option value.
func (m *Machine) TransitionsFrom(field, from string) []Transition {
	var out []Transition
	for _, t := range m.Transitions {
		if t.Field == field && t.From == from {
			out = append(out, t)
		}
	}
	return out
}
