package builder

import (
	"slices"

	"menata.app/internal/registry"
)

// The editors of the Apps Builder (menata-app-document audits/2026-10-27-kajian-apps-builder.md, §6), and "none" for a
// key the builder never edits.
const (
	EditorField      = "field"      // Fase 4 (and 8)
	EditorMenu       = "menu"       // Fase 5 (and 8)
	EditorView       = "view"       // Fase 6, 9
	EditorWorkflow   = "workflow"   // Fase 7
	EditorAutomation = "automation" // Fase 10
	EditorNone       = "none"
)

// Owner says which editor an operation on a key will belong to, and why the assistant has no slot for it today. A key
// the assistant cannot write cannot be reached by the one write path either (aiassist.Write edits the keys its shape
// names), so every such key is work owed by the phase of its editor: the slot (schema, writer, update plan, prompt) lands
// with that editor, as the repo's rule for a new capability requires. Reason is the catalogue's own account of why the
// slot is missing, not a second one.
type Owner struct {
	Editor string
	// Why is what places the key in that editor, or why no editor edits it.
	Why string
}

// engineKey is a key only the approval engine's Machines carry; the builder does not author an engine (a workflow is lowered to
// ordinary transitions, permissions and events -- 004, 007 §25).
const engineKey = "declared by the approval engine's role Machines (workflow: in the Application file); the builder writes a workflow as ordinary transitions, not as an engine"

// Ownership covers every supported key the assistant has no slot for (registry.Capabilities, AIWritable false).
// TestEveryKeyWithoutASlotHasAnEditor holds it complete and free of stale entries in both directions, so a key added to
// the catalogue without a decision, or given a slot without removing its entry, fails.
var Ownership = map[string]Owner{
	// Data & form (Fase 4): the rules a Field or a Machine carries.
	"fieldDoc.locked_when":             {EditorField, "an edit rule on a Field"},
	"fieldDoc.set_once":                {EditorField, "an edit rule on a Field"},
	"fieldDoc.on_parent_delete":        {EditorField, "a delete rule on a relation Field"},
	"fieldDoc.numbering":               {EditorField, "a Field the runtime numbers on create"},
	"fieldDoc.currency":                {EditorField, "a money Field's currency"},
	"fieldDoc.categories":              {EditorField, "a second status Field's tones; beside status_categories"},
	"machineDoc.deletable_unless":      {EditorField, "a delete rule on a Machine"},
	"machineDoc.constraints":           {EditorField, "block-if rules between Machines, written as sentences with choices"},
	"machineDoc.append_only":           {EditorField, "a Machine that is a log"},
	"machineDoc.blocks_member_removal": {EditorField, "a Machine whose records stop a member's removal"},

	// Views and pages (Fase 6, 9).
	"machineDoc.plural":          {EditorView, "the word a board says of several records"},
	"machineDoc.datasets":        {EditorView, "a saved selection behind a list or a card"},
	"machineDoc.sla_field":       {EditorView, "a badge on a list row"},
	"machineDoc.card_tags":       {EditorView, "tags on a board card"},
	"viewDoc.column_actions":     {EditorView, "a board column's menu"},
	"viewDoc.add_column":         {EditorView, "a board's add-list tile"},
	"applicationDoc.view_groups": {EditorView, "screens that share a header and a scope"},
	"navItemDoc.view_group":      {EditorView, "which group a screen belongs to"},
	"navItemDoc.summary":         {EditorView, "the words of a group's header"},

	// Workflow and automation (Fase 7, 10).
	"machineDoc.actions":   {EditorWorkflow, "what an Action writes"},
	"transitionDoc.action": {EditorWorkflow, "an edge that is not the edit Action's"},
	"eventDoc.schedule":    {EditorAutomation, "a rule that runs on a clock"},

	// Not edited by the builder.
	"machineDoc.sequencing":           {EditorNone, engineKey},
	"machineDoc.signature_placement":  {EditorNone, engineKey},
	"machineDoc.signature_store":      {EditorNone, engineKey},
	"machineDoc.flow_template":        {EditorNone, engineKey},
	"machineDoc.flow_template_step":   {EditorNone, engineKey},
	"permissionDoc.actor_type_field":  {EditorNone, engineKey},
	"permissionDoc.actor_user_field":  {EditorNone, engineKey},
	"permissionDoc.actor_group_field": {EditorNone, engineKey},
	"permissionDoc.workspace_role":    {EditorNone, "a Workspace role is the Workspace's, not an Application's"},
	"machineDoc.runtime":              {EditorNone, "a runtime-provided Machine belongs to the runtime"},
	"applicationDoc.companion_of":     {EditorNone, "the library's own packaging"},
}

// Uncovered lists the supported keys the assistant can write that no operation names yet (an id is never edited), in
// catalogue order. It is computed, not written: adding an operation for a key removes it, and the worklist of the editor
// phases is whatever remains. TestUncoveredWritableKeysOnlyShrink holds its size.
func Uncovered() []string {
	covered := map[string]bool{}
	for _, id := range capabilityOf {
		covered[id] = true
	}
	var out []string
	for _, c := range registry.Capabilities {
		if c.Stage != registry.StageSupported || !c.AIWritable || covered[c.ID] || slices.Equal(c.Keys, []string{"id"}) {
			continue
		}
		out = append(out, c.ID)
	}
	return out
}
