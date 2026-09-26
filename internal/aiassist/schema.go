package aiassist

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
}

var stringSchema = geminiSchema{Type: "STRING"}
var boolSchema = geminiSchema{Type: "BOOLEAN"}

var generatedFieldSchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"id", "name", "type", "required"},
	Properties: map[string]geminiSchema{
		"id":       stringSchema,
		"name":     stringSchema,
		"type":     {Type: "STRING", Enum: []string{"text", "number", "boolean", "date", "status", "person", "money", "relation", "file", "group"}},
		"required": boolSchema,
		"options": {
			Type: "ARRAY", Items: &stringSchema,
			Description: "Required, and non-empty, when type is \"status\".",
		},
		"related_machine": {
			Type:        "STRING",
			Description: "Required when type is \"relation\": the id of another machine in this same application.",
		},
	},
}

var generatedPermissionSchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"id", "action", "roles"},
	Properties: map[string]geminiSchema{
		"id":     stringSchema,
		"action": {Type: "STRING", Enum: []string{"create", "edit", "delete"}, Description: "Never \"decide\" or \"revise\" -- those are hardcoded workflow actions this machine cannot reach."},
		"roles":  {Type: "ARRAY", Items: &stringSchema},
	},
}

var generatedTransitionSchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"id", "name", "field", "from", "to"},
	Properties: map[string]geminiSchema{
		"id":    stringSchema,
		"name":  stringSchema,
		"field": {Type: "STRING", Description: "Must be a status field on this same machine."},
		"from":  stringSchema,
		"to":    stringSchema,
	},
}

var generatedEventSchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"id", "summary"},
	Properties: map[string]geminiSchema{
		"id":          stringSchema,
		"on":          {Type: "STRING", Description: "A field id this event fires when it changes. Omit if on_create is true."},
		"when_equals": {Type: "STRING", Description: "Optional: narrow \"on\" to firing only when the field reaches this exact value."},
		"on_create":   boolSchema,
		"summary":     {Type: "STRING", Description: "The activity-log sentence this event writes, e.g. \"{fld_title} submitted\"."},
	},
}

var generatedMachineSchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"id", "name", "fields"},
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
	Type:     "OBJECT",
	Required: []string{"id", "name", "machines"},
	Properties: map[string]geminiSchema{
		"id":          {Type: "STRING", Description: "Must match ^app_[a-z][a-z0-9_]*$."},
		"name":        stringSchema,
		"description": stringSchema,
		"icon":        {Type: "STRING", Description: "One of the known icon names named in the system prompt."},
		"color":       {Type: "STRING", Enum: []string{"blue", "emerald", "amber", "slate"}},
		"roles":       {Type: "ARRAY", Items: &stringSchema},
		"machines":    {Type: "ARRAY", Items: &generatedMachineSchema},
	},
}

var metadataAdditionSchema = geminiSchema{
	Type: "OBJECT",
	Properties: map[string]geminiSchema{
		"machine_id": stringSchema,
		"field_id":   stringSchema,
		"new_option": {Type: "STRING", Description: "Set together with machine_id/field_id to add one option to an existing status field."},
		"new_role":   {Type: "STRING", Description: "Set alone to add one role to the target application's own vocabulary."},
	},
}

var generatedChangeSchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"kind"},
	Properties: map[string]geminiSchema{
		"kind":          {Type: "STRING", Enum: []string{KindNewApplication, KindExtendApplication}},
		"target_app_id": {Type: "STRING", Description: "Required when kind is \"extend_application\": the id of the already-installed application being extended."},
		"application":   generatedApplicationSchema,
		"additions":     {Type: "ARRAY", Items: &metadataAdditionSchema},
	},
}

var capabilityGapSchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"requested", "note"},
	Properties: map[string]geminiSchema{
		"requested": {Type: "STRING", Description: "A short name for the missing capability, e.g. \"multi-person voting approval\" -- stable enough that the same request from different conversations names the same thing, so gaps can be counted."},
		"note":      {Type: "STRING", Description: "One sentence of context: what the user actually asked for."},
	},
}

// replySchema is the whole response shape (see Reply) -- message is always required, change and
// capability_gap are each optional and mutually exclusive in practice (the system prompt says so;
// the schema cannot express "at most one of").
var replySchema = geminiSchema{
	Type:     "OBJECT",
	Required: []string{"message"},
	Properties: map[string]geminiSchema{
		"message":        stringSchema,
		"change":         generatedChangeSchema,
		"capability_gap": capabilityGapSchema,
	},
}
