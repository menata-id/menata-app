package aiassist

import (
	"sort"

	"menata.app/internal/domain"
)

// geminiSchema is Gemini's own accepted subset of the OpenAPI 3.0 Schema Object
// (https://ai.google.dev/gemini-api/docs/structured-output) -- a plain Go mirror of it, marshaled
// straight to JSON as generationConfig.responseSchema. This is the mechanical half of "confirm
// until metadata is complete": Gemini is contractually unable to reply with anything that doesn't
// match this shape, so a conversational turn can never hand back free prose where a decision
// object is expected.
type geminiSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]geminiSchema `json:"properties,omitempty"`
	Items      *geminiSchema           `json:"items,omitempty"`
	Required   []string                `json:"required,omitempty"`
	Enum       []string                `json:"enum,omitempty"`
	Nullable   bool                    `json:"nullable,omitempty"`
	// Description is prompt-adjacent, not validation -- Gemini surfaces it to the model as extra
	// guidance for that one field, which is where a handful of the composable-surface boundaries
	// from prompt.go's own system prompt are restated close to the exact field they constrain (a
	// model reliably honours a constraint stated right next to the field more than one stated only
	// in a long system prompt paragraph).
	Description string `json:"description,omitempty"`
	// PropertyOrdering is the order the model writes an object's properties in. Without it Gemini
	// uses alphabetical order, and the model writes forward only: "additions" came before "kind" and
	// "target_app_id", so it had to decide what to add before it had said what it was changing, and
	// skipped it; "change" came before "message", so it built the change before saying what it was.
	// Found 2026-09-30 when the model, unable to go back, wrote its additions *inside* the
	// target_app_id string. TestEveryObjectSchemaDeclaresItsPropertyOrder holds every object to it.
	PropertyOrdering []string `json:"propertyOrdering,omitempty"`
}

var stringSchema = geminiSchema{Type: "STRING"}
var boolSchema = geminiSchema{Type: "BOOLEAN"}

// declarableIconEnum builds icon's own Enum from domain.DeclarableIcons, sorted for a
// deterministic request body rather than Go's randomized map order. An Enum, not a Description
// merely claiming the set is closed: Gemini is contractually unable to return a value outside an
// Enum, the same way it cannot return a "color" outside generatedApplicationSchema's own Enum
// below -- icon's own field used a Description instead until 2026-09-27, and a real conversation
// spent three straight turns inventing "Palette", "Sparkles" then "FileText", each rejected by
// validate.go, because nothing had ever told the model what the real names were.
func declarableIconEnum() []string {
	names := make([]string, 0, len(domain.DeclarableIcons))
	for name := range domain.DeclarableIcons {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var generatedFieldSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "type", "required", "options", "related_machine", "compute"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "type", "required"},
	Properties: map[string]geminiSchema{
		"id":       stringSchema,
		"name":     stringSchema,
		"type":     {Type: "STRING", Enum: []string{"text", "number", "boolean", "date", "status", "person", "money", "relation", "file", "group"}},
		"required": {Type: "BOOLEAN", Description: "Always false for a field with \"compute\": nobody enters it."},
		"options": {
			Type: "ARRAY", Items: &stringSchema,
			Description: "Required, and non-empty, when type is \"status\".",
		},
		"related_machine": {
			Type:        "STRING",
			Description: "Required when type is \"relation\": the id of a machine in this same change, or of any machine already installed in this workspace -- including one another application owns.",
		},
		"compute": {
			Type:             "OBJECT",
			Description:      "Only on a number field that should be calculated rather than entered, e.g. a total. It is never shown as an input; it is calculated every time the record is saved.",
			Required:         []string{"op", "fields"},
			PropertyOrdering: []string{"op", "fields"},
			Properties: map[string]geminiSchema{
				"op":     {Type: "STRING", Enum: computeOpEnum()},
				"fields": {Type: "ARRAY", Items: &stringSchema, Description: "Ids of number fields of this same machine that are not computed themselves."},
			},
		},
	},
}

func computeOpEnum() []string {
	ops := make([]string, 0, len(domain.KnownComputeOps))
	for op := range domain.KnownComputeOps {
		ops = append(ops, string(op))
	}
	sort.Strings(ops)
	return ops
}

var generatedPermissionSchema = geminiSchema{
	PropertyOrdering: []string{"id", "action", "roles"},
	Type:             "OBJECT",
	Required:         []string{"id", "action", "roles"},
	Properties: map[string]geminiSchema{
		"id":     stringSchema,
		"action": {Type: "STRING", Enum: []string{"create", "edit", "delete"}, Description: "Never \"decide\" or \"revise\" -- those are hardcoded workflow actions this machine cannot reach."},
		"roles":  {Type: "ARRAY", Items: &stringSchema},
	},
}

var generatedTransitionSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "field", "from", "to"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "field", "from", "to"},
	Properties: map[string]geminiSchema{
		"id":    stringSchema,
		"name":  stringSchema,
		"field": {Type: "STRING", Description: "Must be a status field on this same machine."},
		"from":  stringSchema,
		"to":    stringSchema,
	},
}

var generatedEventSchema = geminiSchema{
	PropertyOrdering: []string{"id", "on_create", "on", "when_equals", "summary"},
	Type:             "OBJECT",
	Required:         []string{"id", "summary"},
	Properties: map[string]geminiSchema{
		"id":          stringSchema,
		"on":          {Type: "STRING", Description: "A field id this event fires when it changes. Omit if on_create is true."},
		"when_equals": {Type: "STRING", Description: "Optional: narrow \"on\" to firing only when the field reaches this exact value."},
		"on_create":   boolSchema,
		"summary":     {Type: "STRING", Description: "The activity-log sentence this event writes, e.g. \"{fld_title} submitted\"."},
	},
}

var generatedMachineSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "fields", "permissions", "transitions", "events"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "fields"},
	Properties: map[string]geminiSchema{
		"id":          {Type: "STRING", Description: "Must match ^mch_[a-z][a-z0-9_]*$."},
		"name":        stringSchema,
		"fields":      {Type: "ARRAY", Items: &generatedFieldSchema},
		"permissions": {Type: "ARRAY", Items: &generatedPermissionSchema},
		"transitions": {Type: "ARRAY", Items: &generatedTransitionSchema},
		"events":      {Type: "ARRAY", Items: &generatedEventSchema},
	},
}

var generatedApplicationSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "description", "icon", "color", "roles", "publisher_role", "machines", "navigation"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "machines", "publisher_role", "navigation"},
	Properties: map[string]geminiSchema{
		"id":          {Type: "STRING", Description: "Must match ^app_[a-z][a-z0-9_]*$."},
		"name":        stringSchema,
		"description": stringSchema,
		"icon":        {Type: "STRING", Enum: declarableIconEnum()},
		"color":       {Type: "STRING", Enum: []string{"blue", "emerald", "amber", "slate"}},
		"roles":       {Type: "ARRAY", Items: &stringSchema},
		"publisher_role": {
			Type:        "STRING",
			Description: "Must be one of roles. Which role the person you are talking to will hold themselves once this is published -- ask them plainly which one that is before setting \"change\"; never guess or pick one on their behalf.",
		},
		"machines": {Type: "ARRAY", Items: &generatedMachineSchema},
		"navigation": {
			Type:        "ARRAY",
			Description: "This application's menu, in order; the first entry is where its Home card opens. Ask the person which machines they want in the menu and what each entry should say before setting \"change\"; never decide it for them.",
			Items:       &generatedMenuItemSchema,
		},
	},
}

var generatedMenuItemSchema = geminiSchema{
	PropertyOrdering: []string{"label", "machine_id"},
	Type:             "OBJECT",
	Required:         []string{"label", "machine_id"},
	Properties: map[string]geminiSchema{
		"label":      {Type: "STRING", Description: "What the menu entry says, in the person's own language."},
		"machine_id": {Type: "STRING", Description: "One of this application's own machine ids; the entry opens that machine's list."},
	},
}

var metadataAdditionSchema = geminiSchema{
	PropertyOrdering: []string{"machine_id", "field_id", "new_option", "new_role", "new_machine", "new_nav_item", "rename_application", "relabel_nav_item", "reorder_navigation"},
	Type:             "OBJECT",
	Properties: map[string]geminiSchema{
		"machine_id": stringSchema,
		"field_id":   stringSchema,
		"new_option": {Type: "STRING", Description: "Set together with machine_id/field_id to add one option to an existing status field."},
		"new_role":   {Type: "STRING", Description: "Set alone to add one role to the target application's own vocabulary."},
		"new_machine": withDescription(generatedMachineSchema,
			"Set alone to add a whole new machine to the target application. Its relation fields may point at any machine already in this workspace."),
		"new_nav_item": {
			Type:             "OBJECT",
			Description:      "Set alone to add one menu item to the target application, opening one of its machines (including one this same change adds). Ask the person for the label first.",
			Required:         []string{"id", "label", "machine_id"},
			PropertyOrdering: []string{"id", "label", "machine_id", "title", "description", "icon"},
			Properties: map[string]geminiSchema{
				"id":          {Type: "STRING", Description: "Must match ^nav_[a-z][a-z0-9_]*$ and not be used anywhere in this workspace."},
				"label":       stringSchema,
				"title":       stringSchema,
				"description": stringSchema,
				"machine_id":  stringSchema,
				"icon":        {Type: "STRING", Enum: declarableIconEnum()},
			},
		},
		"rename_application": {Type: "STRING", Description: "Set alone to give the target application a new display name. Its id does not change."},
		"relabel_nav_item": {
			Type:             "OBJECT",
			Description:      "Set alone to change the label of one of the target application's existing menu items, by its nav id.",
			Required:         []string{"nav_id", "label"},
			PropertyOrdering: []string{"nav_id", "label"},
			Properties:       map[string]geminiSchema{"nav_id": stringSchema, "label": stringSchema},
		},
		"reorder_navigation": {
			Type:        "ARRAY",
			Items:       &stringSchema,
			Description: "Set alone to reorder the target application's menu: every one of its menu item ids, including ones this change adds, in the new order.",
		},
	},
}

func withDescription(s geminiSchema, d string) geminiSchema {
	s.Description = d
	return s
}

var generatedChangeSchema = geminiSchema{
	PropertyOrdering: []string{"kind", "target_app_id", "application", "additions"},
	Type:             "OBJECT",
	Required:         []string{"kind"},
	Properties: map[string]geminiSchema{
		"kind":          {Type: "STRING", Enum: []string{KindNewApplication, KindExtendApplication}},
		"target_app_id": {Type: "STRING", Description: "Required when kind is \"extend_application\": the id of the already-installed application being extended."},
		"application":   generatedApplicationSchema,
		"additions":     {Type: "ARRAY", Items: &metadataAdditionSchema},
	},
}

var capabilityGapSchema = geminiSchema{
	PropertyOrdering: []string{"requested", "note"},
	Type:             "OBJECT",
	Required:         []string{"requested", "note"},
	Properties: map[string]geminiSchema{
		"requested": {Type: "STRING", Description: "A short name for the missing capability, e.g. \"multi-person voting approval\" -- stable enough that the same request from different conversations names the same thing, so gaps can be counted."},
		"note":      {Type: "STRING", Description: "One sentence of context: what the user actually asked for."},
	},
}

// replySchema is the whole response shape (see Reply) -- message is always required, change and
// capability_gap are each optional and mutually exclusive in practice (the system prompt says so;
// the schema cannot express "at most one of").
var replySchema = geminiSchema{
	PropertyOrdering: []string{"message", "capability_gap", "change"},
	Type:             "OBJECT",
	Required:         []string{"message"},
	Properties: map[string]geminiSchema{
		"message":        stringSchema,
		"change":         generatedChangeSchema,
		"capability_gap": capabilityGapSchema,
	},
}
