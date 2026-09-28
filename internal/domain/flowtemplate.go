package domain

// FlowTemplate and FlowTemplateStep name the Fields of a *saved* approval flow -- the pattern an
// Application remembers per Document Type and offers back the next time somebody submits one
// (CAP-V28).
//
// They exist for the reason SignaturePlacement did one stage earlier, and it is worth keeping the
// distinction because it decides when to add a declaration rather than derive one. The live Approval
// Step's shape is *derivable*: `sequencing:` says which Field orders siblings, the `decide` Permission
// says which holds the actor and, through its dynamic gate, the actor's kind and Group. A template step
// has none of those, and correctly so -- nothing decides a template. Stage E1 tried the derivation
// anyway and a probe showed every id coming back empty, which would have written four values under the
// empty key and produced a saved flow with no approvers at all.
//
// **So the six keys below are not a second source of truth for the live step's six.** They are a
// different Machine answering for itself. Declaring `sequencing:` on a template to make the derivation
// work would assert locking behaviour it does not have, which is the reading that was checked and
// rejected rather than assumed.
//
// Nil means the Machine declares none, and there is no fallback to internal/action's own constants --
// the same contract EngineFields, SignaturePlacement and SignatureStore all state, for the same reason:
// a silent fallback makes a Machine that declares less than the template library look like it worked
// while matching the wrong records.

// FlowTemplate is the saved flow itself, declared as `flow_template:` on the Machine an Application
// casts in the engine's `flow_template` role.
type FlowTemplate struct {
	// KeyField is what a saved flow is looked up by: one flow per distinct value of it. In the
	// template library that is a Document Type, and find-or-create on this Field is how "the default
	// flow for Contracts" is expressed without a uniqueness primitive this runtime does not have.
	KeyField string
	// ModeField holds the approval mode the saved flow remembers, so reloading it restores
	// sequential-or-parallel along with the rows.
	ModeField string
}

// FlowTemplateStep is one row of that flow -- deliberately the subset of a live step's Fields that
// describes a *shape* rather than a decision. Declared as `flow_template_step:` on the Machine cast in
// the `flow_template_step` role.
type FlowTemplateStep struct {
	// TemplateField is the relation back to the FlowTemplate this row belongs to.
	TemplateField string
	// OrderField is the number Field ordering the rows; lower runs first.
	OrderField string
	// NameField is what the step is for ("Legal Review"), independent of who holds it.
	NameField string
	// ActorField, ActorTypeField and ActorGroupField are the same three-way shape a live step's
	// dynamic actor gate has (CAP-F24): which person, which kind of actor the row chose, and which
	// Group when it chose one.
	ActorField      string
	ActorTypeField  string
	ActorGroupField string
}
