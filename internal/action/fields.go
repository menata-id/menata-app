package action

import "menata.app/internal/domain"

// EngineFields is where this engine's Field ids come from: derived once, from the two Machines' own
// declarations, and then read like any other value.
//
// Every entry answers a question metadata already answers somewhere, and each takes the declaration
// that answers *its own* question rather than the nearest one (001 Principle #8; criterion B3 --
// the existing primitive already fits):
//
//   - Decision, from the `transitions:` edges naming `action: decide`. **Not** from
//     sequencing.state_field, which really is the same Field in the template library and is the wrong
//     source: Sequencing declares how records are ordered and locked, so a Machine that orders nothing
//     would have no decision Field for no reason.
//   - Parent, from the relation Field pointing at the document Machine -- structural, always true of a
//     step, and independent of whether ordering is declared.
//   - Order, from sequencing.order_field, which is that block's own subject.
//   - Actor/ActorType/ActorGroup, from the Permission governing `decide` (CAP-F24's dynamic gate where
//     one is declared, actor_field otherwise).
//   - DocumentStatus, from the Field the document Machine's own Transitions move. Its six edges name no
//     Action at all -- that status is derived from its steps -- which is exactly why ActionField cannot
//     answer it and StatusField can.
//
// **Empty means the Machine declares nothing for it**, never "use the usual name". There is no fallback
// to this package's own constants on purpose: a silent fallback would make a Machine that declares less
// than the template library look like it worked while matching the wrong records. Callers handle an
// empty id explicitly -- most naturally by doing nothing, which is what a Machine declaring no ordering
// should get from a sort.
type EngineFields struct {
	Decision string
	// Open is the value a record holds while the Action has not been taken -- the `from:` of the edges
	// naming it, so "still undecided" is read from the state model rather than compared against a
	// literal. Empty when those edges disagree, which no real state model does.
	//
	// sequencing.open_value declares the same string and is again the wrong source, for the same reason
	// Decision's is: it belongs to ordering, and a Machine that orders nothing still has open records.
	Open           string
	Parent         string
	Order          string
	Actor          string
	ActorType      string
	ActorGroup     string
	DocumentStatus string
	// Submitter is the document Machine's Field holding whoever created the record: the actor_field of its
	// create Permission. Empty when the Machine declares none.
	Submitter string
}

// DeclaredFields derives the set for one step Machine and its document Machine. Either may be nil --
// a Workspace whose Application casts no such role -- and then every id this would have read from it is
// empty.
func DeclaredFields(stepMachine, docMachine *domain.Machine) EngineFields {
	var f EngineFields
	if stepMachine != nil {
		f.Decision = stepMachine.ActionField(domain.ActionDecide)
		f.Open = openValueFor(stepMachine, domain.ActionDecide)
		f.Order = stepMachine.OrderField()
		f.Actor = stepMachine.ActorFieldFor(domain.ActionDecide)
		if gate := stepMachine.ActorGateFor(domain.ActionDecide); gate != nil {
			f.ActorType, f.ActorGroup = gate.ActorTypeField, gate.ActorGroupField
		}
		if docMachine != nil {
			if parent, ok := stepMachine.ReferenceFieldTo(docMachine.ID); ok {
				f.Parent = parent.ID
			}
		}
	}
	if docMachine != nil {
		f.DocumentStatus = docMachine.StatusField()
		f.Submitter = docMachine.ActorFieldFor(domain.ActionCreate)
	}
	return f
}

// SignatureFields is the step Machine's own declared signature shape, and StoreFields is the
// signature store's (Stage D, 2026-09-28). Both return a zero value when the Machine declares
// nothing, which callers handle explicitly -- the same "empty means undeclared, never assume the
// usual name" contract DeclaredFields above states at length.
//
// They exist as functions here rather than as bare field reads so every caller in every plane asks
// the same question in the same words, the way DeclaredFields already unified the Stage B side. A
// nil Machine is a real input: a Workspace whose Application casts no signature store gets one, and
// the one-time image on the step is then the whole feature.
func SignatureFields(stepMachine *domain.Machine) domain.SignaturePlacement {
	if stepMachine == nil || stepMachine.SignaturePlacement == nil {
		return domain.SignaturePlacement{}
	}
	return *stepMachine.SignaturePlacement
}

func StoreFields(storeMachine *domain.Machine) domain.SignatureStore {
	if storeMachine == nil || storeMachine.SignatureStore == nil {
		return domain.SignatureStore{}
	}
	return *storeMachine.SignatureStore
}

// CompositeFields is the already-declared answer to "which Field on the parent holds the document
// itself" -- `source_field` on whichever Event triggers the compositing Service (Stage C).
//
// It is asked of the *step* Machine because that is where the Event is declared, while the caller
// usually holds the document: a screen wanting to link a Document's own PDF is asking the same
// question the Service answers, so it reads the same declaration instead of naming fld_file. Zero
// value when no such Event exists, same contract as its two neighbours.
func CompositeFields(stepMachine *domain.Machine) domain.Composite {
	if stepMachine == nil {
		return domain.Composite{}
	}
	for _, e := range stepMachine.Events {
		if e.Then.Name == domain.ServiceCompositeSignedDocument && e.Then.Composite != nil {
			return *e.Then.Composite
		}
	}
	return domain.Composite{}
}

// FlowTemplateFields and FlowTemplateStepFields are the saved approval flow's own declared shape
// (Stage E2, 2026-09-29) -- see domain.FlowTemplate for why this pair is declared where a live step's
// equivalent is derived. Zero value when the Machine declares nothing, same contract as every other
// accessor here.
func FlowTemplateFields(templateMachine *domain.Machine) domain.FlowTemplate {
	if templateMachine == nil || templateMachine.FlowTemplate == nil {
		return domain.FlowTemplate{}
	}
	return *templateMachine.FlowTemplate
}

func FlowTemplateStepFields(stepMachine *domain.Machine) domain.FlowTemplateStep {
	if stepMachine == nil || stepMachine.FlowTemplateStep == nil {
		return domain.FlowTemplateStep{}
	}
	return *stepMachine.FlowTemplateStep
}

// FlowTemplateRowFields turns a saved-flow row declaration into the EngineFields the wizard's shared
// row writer takes, so one function writes a live Approval Step and a template row alike.
//
// The mapping is the point: an EngineFields is "which Field holds the order, the actor, the actor's
// kind, the actor's Group" -- and that question has two legitimate answers depending on the Machine,
// derived for the live step and declared for the template. This is where the second one is expressed.
func FlowTemplateRowFields(stepMachine *domain.Machine) EngineFields {
	fs := FlowTemplateStepFields(stepMachine)
	return EngineFields{
		Order:      fs.OrderField,
		Actor:      fs.ActorField,
		ActorType:  fs.ActorTypeField,
		ActorGroup: fs.ActorGroupField,
		Parent:     fs.TemplateField,
	}
}

// openValueFor is the single value actionName's declared edges all move *from* -- the state a record
// sits in while nobody has acted. Empty when the edges disagree, which is no answer rather than a
// guess, the same posture every accessor here takes.
func openValueFor(m *domain.Machine, actionName string) string {
	from := ""
	for _, t := range m.Transitions {
		if t.Action != actionName {
			continue
		}
		if from != "" && from != t.From {
			return ""
		}
		from = t.From
	}
	return from
}
