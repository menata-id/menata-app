package registry

import "fmt"

// Capability stages. A key the loader reads is Supported; a key a corpus case wishes for and the loader does not
// read yet is Proposed. (Incubating and Deprecated from the kajian strategi §4.2 are not catalogued until something
// is either.)
const (
	StageSupported = "supported"
	StageProposed  = "proposed"
)

// Capability is one row of the catalogue (kajian strategi §4.2, KT1): a metadata key, or a group of keys that are
// one capability, with the facts that used to live in five places.
//
// **Scope of this first slice, stated so it is not read as the whole of §4.2**: the catalogue covers the keys of
// the two loader structs the assistant's output schema mirrors (`machineDoc`, `fieldDoc`) plus the keys the case
// corpus wishes for. The other structs, the gates/docs columns and the generated documents are later slices.
type Capability struct {
	// ID names the capability. A supported key is its own capability and is named by the key.
	ID string
	// Struct is the unexported loader struct in internal/metadata/parse.go whose `yaml:` tag carries Keys.
	Struct string
	// Keys are the yaml tag names. Supported: every one is a tag on Struct. Proposed: none is.
	Keys []string
	// Kind is what sort of thing the keys declare.
	Kind string
	// Plane is the 007 plane that owns it: domain, behavior, data, experience, runtime.
	Plane string
	Stage string
	// AIWritable says whether the assistant's output schema has a slot for the key. The conformance gate holds it
	// to the schema by reflection, so it cannot be claimed.
	AIWritable bool
	// AIReason is why the assistant has no slot (required when AIWritable is false) or, for a Proposed capability,
	// what must land with it.
	AIReason string
	// Cases are the corpus cases (internal/web/testdata/cases) that wish for the keys. Proposed: required, and the
	// corpus test holds it to the keys each case's loader run rejects.
	Cases []string
}

// editRuleReason is why the four edit and delete rules of 2026-10-17 (owner decision K1: deletable_unless, locked_when,
// set_once, on_parent_delete) have no assistant slot yet: they landed with Dokter Kecil's list screens, and the slot --
// schema, writer, update plan and prompt, as required_when has -- is the next assistant slice (menata-app-document
// audits/2026-10-17-rencana-tampilan-langkah-1-4.md §1).
const editRuleReason = "an edit/delete rule (owner decision K1, 2026-10-17); the assistant's slot is the next assistant slice, not built with the screens"

func key(structName, k, kind, plane string, ai bool, reason string) Capability {
	return Capability{ID: structName + "." + k, Struct: structName, Keys: []string{k}, Kind: kind, Plane: plane, Stage: StageSupported, AIWritable: ai, AIReason: reason}
}

// Capabilities is the closed catalogue. It replaces `aiUnwritableKeys`, the map in internal/conformance that held
// the "no slot" reasons, as the single place that says what a key is and whether the assistant may write it.
var Capabilities = []Capability{
	// machineDoc
	key("machineDoc", "id", "identity", "domain", true, ""),
	key("machineDoc", "name", "identity", "domain", true, ""),
	key("machineDoc", "fields", "structure", "domain", true, ""),
	key("machineDoc", "events", "behaviour", "behavior", true, ""),
	key("machineDoc", "permissions", "authorization", "behavior", true, ""),
	key("machineDoc", "transitions", "behaviour", "behavior", true, ""),
	key("machineDoc", "completion", "behaviour", "behavior", true, ""),
	key("machineDoc", "card_fields", "presentation", "experience", true, ""),
	key("machineDoc", "views", "presentation", "experience", true, ""),
	key("machineDoc", "constraints", "behaviour", "behavior", false, "block-if rules relate two Machines; the assistant has no related-record vocabulary yet"),
	key("machineDoc", "actions", "behaviour", "behavior", false, "declares what an Action writes; only the approval engine's Machines use it"),
	key("machineDoc", "datasets", "data", "data", false, "free-form selection (a status filter, counts); a generated Machine gets a Dataset only as the list behind a menu entry (GeneratedMenuItem.List, W5-a), a status filter, or a work queue (GeneratedMenuItem.Queue)"),
	key("machineDoc", "sequencing", "engine", "behavior", false, "approval-engine ordering; generated Machines have no step Machine"),
	key("machineDoc", "signature_placement", "engine", "behavior", false, "approval-engine role (step); not generatable"),
	key("machineDoc", "signature_store", "engine", "behavior", false, "approval-engine role (signature); not generatable"),
	key("machineDoc", "flow_template", "engine", "behavior", false, "approval-engine role; not generatable"),
	key("machineDoc", "flow_template_step", "engine", "behavior", false, "approval-engine role; not generatable"),
	key("machineDoc", "blocks_member_removal", "behaviour", "behavior", false, "no generated Machine is a member-removal blocker yet"),
	key("machineDoc", "sla_field", "presentation", "experience", false, "SLA badge; no forcing case in a generated Application"),
	key("machineDoc", "status_categories", "presentation", "experience", true, ""),
	key("machineDoc", "card_tags", "presentation", "experience", false, "board card tags; no generated board"),
	key("machineDoc", "append_only", "behaviour", "behavior", false, "no generated Machine is a log"),
	key("machineDoc", "runtime", "data", "data", false, "a runtime-provided Machine belongs to the runtime (W2); a generated Application declares only stored Machines"),
	key("machineDoc", "ensure", "behaviour", "behavior", true, ""),
	key("machineDoc", "frozen_when", "behaviour", "behavior", true, ""),
	key("machineDoc", "deletable_unless", "behaviour", "behavior", false, editRuleReason),
	// transitionDoc
	key("transitionDoc", "id", "identity", "domain", true, ""),
	key("transitionDoc", "name", "identity", "domain", true, ""),
	key("transitionDoc", "field", "structure", "domain", true, ""),
	key("transitionDoc", "from", "structure", "domain", true, ""),
	key("transitionDoc", "to", "structure", "domain", true, ""),
	key("transitionDoc", "action", "behaviour", "behavior", false, "a generated Machine's edges are always the edit Action's; the assistant writes none other"),
	key("transitionDoc", "roles", "authorization", "behavior", true, ""),
	key("transitionDoc", "actor_field", "authorization", "behavior", true, ""),
	key("transitionDoc", "sets", "behaviour", "behavior", true, ""),
	key("transitionDoc", "balanced", "behaviour", "behavior", true, ""),
	key("transitionDoc", "generates", "behaviour", "behavior", true, ""),
	// permissionDoc
	key("permissionDoc", "id", "identity", "domain", true, ""),
	key("permissionDoc", "action", "authorization", "behavior", true, ""),
	key("permissionDoc", "roles", "authorization", "behavior", true, ""),
	key("permissionDoc", "actor_field", "authorization", "behavior", true, ""),
	key("permissionDoc", "parent_actor", "authorization", "behavior", true, ""),
	key("permissionDoc", "workspace_role", "authorization", "behavior", false, "a Workspace role (admin) is the Workspace's, not an Application's; a generated Application gates by its own roles"),
	key("permissionDoc", "actor_type_field", "authorization", "behavior", false, "dynamic actor gate (an approver that is a person or a group per record); only the approval engine's step Machine uses it"),
	key("permissionDoc", "actor_user_field", "authorization", "behavior", false, "dynamic actor gate; see actor_type_field"),
	key("permissionDoc", "actor_group_field", "authorization", "behavior", false, "dynamic actor gate; see actor_type_field"),
	key("parentActorDoc", "via", "authorization", "behavior", true, ""),
	key("parentActorDoc", "field", "authorization", "behavior", true, ""),
	// eventDoc (its `then` block is flattened into service, summary, recipient_field and preference_key in the
	// assistant's schema; the rollup and composite shapes inside it are not catalogued in this slice)
	key("eventDoc", "id", "identity", "domain", true, ""),
	key("eventDoc", "on", "behaviour", "behavior", true, ""),
	key("eventDoc", "when_equals", "behaviour", "behavior", true, ""),
	key("eventDoc", "on_create", "behaviour", "behavior", true, ""),
	key("eventDoc", "then", "behaviour", "behavior", true, ""),
	key("eventDoc", "schedule", "behaviour", "behavior", false, "a scheduled event (SLA breach) runs on a clock and a guard; nothing in a generated Application has asked for one"),
	// viewDoc, cardFieldDoc, completionDoc, computeDoc
	key("viewDoc", "id", "identity", "experience", true, ""),
	key("viewDoc", "name", "identity", "experience", true, ""),
	key("viewDoc", "type", "presentation", "experience", true, ""),
	key("viewDoc", "group_by", "presentation", "experience", true, ""),
	key("viewDoc", "column_actions", "presentation", "experience", false, "a board column's rename/move/delete menu over a relation-grouped board; a generated board groups by a status Field's options, which are not records"),
	key("viewDoc", "add_column", "presentation", "experience", false, "a board's \"add list\" tile leading to a navigation item; a generated board has no record-backed columns to add"),
	key("cardFieldDoc", "field", "presentation", "experience", true, ""),
	key("cardFieldDoc", "role", "presentation", "experience", true, ""),
	key("completionDoc", "field", "behaviour", "behavior", true, ""),
	key("completionDoc", "done", "behaviour", "behavior", true, ""),
	key("computeDoc", "op", "structure", "domain", true, ""),
	key("computeDoc", "fields", "structure", "domain", true, ""),
	// fieldDoc
	key("fieldDoc", "id", "identity", "domain", true, ""),
	key("fieldDoc", "name", "identity", "domain", true, ""),
	key("fieldDoc", "type", "structure", "domain", true, ""),
	key("fieldDoc", "required", "structure", "domain", true, ""),
	key("fieldDoc", "options", "structure", "domain", true, ""),
	key("fieldDoc", "machine", "structure", "domain", true, ""), /* spelt related_machine in the assistant's schema */
	key("fieldDoc", "default", "structure", "domain", true, ""),
	key("fieldDoc", "compute", "structure", "domain", true, ""),
	key("fieldDoc", "stamp", "structure", "domain", true, ""),
	key("fieldDoc", "required_when", "structure", "domain", true, ""),
	key("fieldDoc", "default_from", "structure", "domain", true, ""),
	key("fieldDoc", "locked_when", "behaviour", "behavior", false, editRuleReason),
	key("fieldDoc", "numbering", "behaviour", "behavior", false, "a number the runtime assigns on create (2026-10-18, plan A4: CTR/C/026, PP-0146); the assistant gains it with the edit/delete rules"),
	key("fieldDoc", "currency", "presentation", "experience", false, "a money Field's currency (2026-10-18, plan A2); the assistant gains it with the edit/delete rules"),
	key("fieldDoc", "categories", "presentation", "experience", false, "a second status Field's tones (2026-10-17, Dokter Kecil's payment status); the assistant writes the Machine's status_categories: and gains this slot with the edit/delete rules"),
	key("fieldDoc", "set_once", "behaviour", "behavior", false, editRuleReason),
	key("fieldDoc", "on_parent_delete", "behaviour", "behavior", false, editRuleReason),
	// applicationDoc (an Application's own file): a page's arrangement is declared, not generated -- the assistant writes no `page:` either.
	key("applicationDoc", "view_groups", "presentation", "experience", false, "screens that are arrangements of one Machine's records sharing a header, a switch and a scope; like `page:` (UnwritableByTheAssistant) it is authored, and a generated Application has no Board, Table or Calendar of one Machine to group"),
	key("navItemDoc", "view_group", "presentation", "experience", false, "makes a screen one arrangement of an Application's view group; authored with the group"),
	key("navItemDoc", "summary", "presentation", "experience", false, "words a view group's header with its counts; authored with the group"),

	// Proposed rows (a key a case corpus wishes for and no loader reads) are none today: both corpora, Cases 1, 2, 17 and
	// Cases 4, 5, 6, 14 and Case 9, are fully built (2026-10-16). A new wish arrives as a row here naming its cases, which the
	// corpus gate holds both ways, and leaves as a supported row when it is built.
}

// CapabilityProblems checks the catalogue's own rules (the loader and the assistant's schema are checked in
// internal/conformance, which can read both). It takes the rows so a test can hand it a broken one.
func CapabilityProblems(rows []Capability) []string {
	var out []string
	seenID := map[string]bool{}
	seenKey := map[string]string{}
	for _, c := range rows {
		switch {
		case c.ID == "":
			out = append(out, "a capability has no id")
			continue
		case seenID[c.ID]:
			out = append(out, fmt.Sprintf("capability %q is listed twice", c.ID))
		}
		seenID[c.ID] = true
		if c.Struct == "" || len(c.Keys) == 0 {
			out = append(out, fmt.Sprintf("%s names no struct or no key", c.ID))
		}
		for _, k := range c.Keys {
			if other, dup := seenKey[c.Struct+"."+k]; dup {
				out = append(out, fmt.Sprintf("%s.%s is claimed by both %s and %s", c.Struct, k, other, c.ID))
			}
			seenKey[c.Struct+"."+k] = c.ID
		}
		switch c.Stage {
		case StageSupported:
			if !c.AIWritable && c.AIReason == "" {
				out = append(out, fmt.Sprintf("%s: the assistant cannot write it and no reason is given", c.ID))
			}
			if c.AIWritable && c.AIReason != "" {
				out = append(out, fmt.Sprintf("%s: writable by the assistant yet carries a reason it is not", c.ID))
			}
			if len(c.Cases) > 0 {
				out = append(out, fmt.Sprintf("%s: a supported capability still names corpus cases", c.ID))
			}
		case StageProposed:
			if c.AIWritable {
				out = append(out, fmt.Sprintf("%s: a proposed capability cannot already be writable", c.ID))
			}
			if len(c.Cases) == 0 {
				out = append(out, fmt.Sprintf("%s: a proposed capability names no case that wishes for it", c.ID))
			}
		default:
			out = append(out, fmt.Sprintf("%s: unknown stage %q", c.ID, c.Stage))
		}
	}
	return out
}
