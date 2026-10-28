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

// fieldTypeEnum is a generated Field's type Enum, read from domain.FieldTypes. It was a literal list until
// 2026-10-10 and had drifted: it held ten types while the prompt, generated from the same catalogue, named
// eleven, so the model was told about long_text and contractually unable to return it. The prompt and the
// schema now read one list; TestFieldTypeEnumIsTheCatalogue holds it.
func fieldTypeEnum() []string {
	types := domain.FieldTypes()
	out := make([]string, 0, len(types))
	for _, t := range types {
		out = append(out, string(t))
	}
	return out
}

var generatedFieldSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "type", "required", "options", "related_machine", "compute", "default", "stamp", "required_when", "default_from", "editable_by", "show_when"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "type", "required"},
	Properties: map[string]geminiSchema{
		"id":       stringSchema,
		"name":     stringSchema,
		"type":     {Type: "STRING", Enum: fieldTypeEnum()},
		"required": {Type: "BOOLEAN", Description: "Always false for a field with \"compute\": nobody enters it."},
		"options": {
			Type: "ARRAY", Items: &stringSchema,
			Description: "Required, and non-empty, when type is \"status\".",
		},
		"related_machine": {
			Type:        "STRING",
			Description: "Required when type is \"relation\": the id of a machine in this same change, or of any machine already installed in this workspace -- including one another application owns.",
		},
		"default": {Type: "STRING", Description: "The value a new record starts with, as text (\"5\", \"true\"); for a status field, one of its own options (e.g. the first). Never on a computed, stamped or file field."},
		"stamp":   {Type: "STRING", Enum: fieldStampEnum(), Description: "Only on a person field the runtime should fill with who created the record (e.g. \"Requester\"); it is never an input. Do not also give it a default."},
		"required_when": {
			Type:             "OBJECT",
			Description:      "Only on a field that must be filled when another field has one value (e.g. a reference file required when the format is a banner). Then \"required\" must be false. The other field is a status field of this same machine; \"equals\" is one of its options.",
			Required:         []string{"field", "equals"},
			PropertyOrdering: []string{"field", "equals"},
			Properties:       map[string]geminiSchema{"field": stringSchema, "equals": stringSchema},
		},
		"show_when": {
			Type:             "OBJECT",
			Description:      "Only on a field that is asked for just when another field has one value (e.g. a contract number, asked only when the kind is Contract). The other field is a status field of this same machine; \"equals\" is one of its options. A field that is not shown is not required, so it may be \"required\": true. Not together with required_when.",
			Required:         []string{"field", "equals"},
			PropertyOrdering: []string{"field", "equals"},
			Properties:       map[string]geminiSchema{"field": stringSchema, "equals": stringSchema},
		},
		"editable_by": {
			Type: "ARRAY", Items: &stringSchema,
			Description: "Only on a field that just some roles may change after it is written (e.g. only the reviewer sets an amount): roles this application declares. Everyone who may edit the record can still read it. Leave out for a field anyone may change.",
		},
		"default_from": {
			Type:             "OBJECT",
			Description:      "Only on a date field that starts as today plus some days depending on a status field of this same machine (e.g. a due date that depends on the priority). Not together with default.",
			Required:         []string{"field", "days"},
			PropertyOrdering: []string{"field", "days"},
			Properties: map[string]geminiSchema{
				"field": stringSchema,
				"days": {Type: "ARRAY", Items: &geminiSchema{Type: "OBJECT", Required: []string{"value", "days"}, PropertyOrdering: []string{"value", "days"},
					Properties: map[string]geminiSchema{"value": stringSchema, "days": {Type: "INTEGER", Description: "0 or more."}}}},
			},
		},
		"compute": {
			Type:             "OBJECT",
			Description:      "Only on a field that should be calculated rather than entered: a total (op sum, on a number or money field over number or money fields), or a date plus a number of days (op add_days, on a DATE field, fields: [a date field, a number field]). It is never shown as an input; it is calculated every time the record is saved.",
			Required:         []string{"op", "fields"},
			PropertyOrdering: []string{"op", "fields"},
			Properties: map[string]geminiSchema{
				"op":     {Type: "STRING", Enum: computeOpEnum()},
				"fields": {Type: "ARRAY", Items: &stringSchema, Description: "Ids of number or money fields of this same machine that are not computed themselves."},
			},
		},
	},
}

// applicationColorEnum is domain.KnownApplicationColors, sorted. It was a literal of four names that the loader's
// own list could outgrow without the model ever hearing of it (K16; the same drift K02 found in the field types).
func applicationColorEnum() []string {
	names := make([]string, 0, len(domain.KnownApplicationColors))
	for name := range domain.KnownApplicationColors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sortDirectionEnum is domain.KnownSortDirections, sorted: a list's direction is the loader's own closed set.
func sortDirectionEnum() []string {
	out := make([]string, 0, len(domain.KnownSortDirections))
	for d := range domain.KnownSortDirections {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// transitionSourceEnum is domain.KnownTransitionSources, sorted.
func transitionSourceEnum() []string {
	out := make([]string, 0, len(domain.KnownTransitionSources))
	for s := range domain.KnownTransitionSources {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func notificationPreferenceEnum() []string {
	out := make([]string, 0, len(domain.KnownNotificationPreferenceKeys))
	for k := range domain.KnownNotificationPreferenceKeys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fieldStampEnum() []string {
	out := make([]string, 0, len(domain.KnownFieldStamps))
	for s := range domain.KnownFieldStamps {
		out = append(out, string(s))
	}
	sort.Strings(out)
	return out
}

func computeOpEnum() []string {
	ops := make([]string, 0, len(domain.KnownComputeOps))
	for op := range domain.KnownComputeOps {
		// The assistant writes the flat shape -- an operation over Fields -- so an operation that needs a literal (round)
		// or another Machine (sum_children, count_children) is not offered: it would only ever be refused.
		switch op {
		case domain.ComputeRound, domain.ComputeSumChildren, domain.ComputeCountChildren, domain.ComputeJoin:
			continue
		}
		ops = append(ops, string(op))
	}
	sort.Strings(ops)
	return ops
}

var generatedCardFieldSchema = geminiSchema{
	PropertyOrdering: []string{"field", "role"},
	Type:             "OBJECT",
	Required:         []string{"field", "role"},
	Properties: map[string]geminiSchema{
		"field": {Type: "STRING", Description: "A field of this same machine."},
		"role":  {Type: "STRING", Enum: GeneratableCardRoles, Description: "title: the card's heading (a text field). status: a status field. date: a date field. person: a person field. money: a number or money field."},
	},
}

var generatedViewSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "type", "group_by"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "type"},
	Properties: map[string]geminiSchema{
		"id":       {Type: "STRING", Description: "Must match ^vw_[a-z][a-z0-9_]*$."},
		"name":     {Type: "STRING", Description: "What the person reads when choosing an arrangement, in their own language, e.g. \"Papan\" or \"Tabel\". One line."},
		"type":     {Type: "STRING", Enum: GeneratableViewTypes},
		"group_by": {Type: "STRING", Description: "Required for a board, and only for a board: a STATUS field of this machine; its options become the columns."},
	},
}

var generatedPermissionSchema = geminiSchema{
	PropertyOrdering: []string{"id", "action", "roles", "actor_field", "parent_actor"},
	Type:             "OBJECT",
	Required:         []string{"id", "action", "roles"},
	Properties: map[string]geminiSchema{
		"actor_field": {Type: "STRING", Description: "Optional: a person field of this same machine. Only the person named in it on a record may use this permission on that record (\"only the requester edits\"). Roles may then be empty."},
		"parent_actor": {
			Type:             "OBJECT",
			Description:      "Optional: only the person named in a person field of the RELATED record may use this permission (\"the owner of the room approves\").",
			Required:         []string{"via", "field"},
			PropertyOrdering: []string{"via", "field"},
			Properties: map[string]geminiSchema{
				"via":   {Type: "STRING", Description: "A relation field of this machine."},
				"field": {Type: "STRING", Description: "A person field of the machine that relation points at."},
			},
		},
		"id":     stringSchema,
		"action": {Type: "STRING", Enum: []string{"create", "edit", "delete"}, Description: "Never \"decide\" or \"revise\" -- those are hardcoded workflow actions this machine cannot reach."},
		"roles":  {Type: "ARRAY", Items: &stringSchema},
	},
}

var generatedTransitionSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "field", "from", "to", "roles", "actor_field", "sets", "balanced", "generates"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "field", "from", "to"},
	Properties: map[string]geminiSchema{
		"id":          stringSchema,
		"name":        stringSchema,
		"field":       {Type: "STRING", Description: "Must be a status field on this same machine."},
		"from":        stringSchema,
		"to":          stringSchema,
		"roles":       {Type: "ARRAY", Items: &stringSchema, Description: "Optional: only people holding one of these roles of this application may take this step (\"only a manager approves\"). Leave out for any editor."},
		"actor_field": {Type: "STRING", Description: "Optional: a person field of this same machine. Only the person named in it on a record may take this step on that record (\"only the requester reopens\")."},
		"sets": {Type: "ARRAY", Description: "Optional: date fields of this machine this step writes with the day it is taken (e.g. the completion date when completing). The clock wins over anything typed.",
			Items: &geminiSchema{Type: "OBJECT", Required: []string{"field", "source"}, PropertyOrdering: []string{"field", "source"},
				Properties: map[string]geminiSchema{"field": stringSchema, "source": {Type: "STRING", Enum: transitionSourceEnum()}}}},
		"balanced": {Type: "OBJECT", Description: "Optional: this step moves only when the records of another machine that point at this one have equal debit and credit totals (posting a journal entry whose lines balance). Nothing to total is refused too.",
			Required: []string{"of", "via", "debit", "credit"}, PropertyOrdering: []string{"of", "via", "debit", "credit", "reason"},
			Properties: map[string]geminiSchema{
				"of":     {Type: "STRING", Description: "The machine whose records are totalled (the lines)."},
				"via":    {Type: "STRING", Description: "A relation field of that machine pointing back at THIS machine."},
				"debit":  {Type: "STRING", Description: "A number or money field of that machine."},
				"credit": {Type: "STRING", Description: "A different number or money field of that machine."},
				"reason": {Type: "STRING", Description: "What a refused step is told."},
			}},
		"generates": {Type: "OBJECT", Description: "Optional: taking this step creates a schedule of records of another machine (disbursing a loan writes its instalments), at most 120, never twice for one record.",
			Required:         []string{"machine", "count_field", "link_field", "due_field", "due_every_days", "amount_field", "amount_from"},
			PropertyOrdering: []string{"machine", "count_field", "link_field", "due_field", "due_every_days", "amount_field", "amount_from"},
			Properties: map[string]geminiSchema{
				"machine":        {Type: "STRING", Description: "The machine whose records are created."},
				"count_field":    {Type: "STRING", Description: "A number field of THIS machine: how many records (a whole number, 1 to 120)."},
				"link_field":     {Type: "STRING", Description: "A relation field of the created machine pointing back at THIS machine."},
				"due_field":      {Type: "STRING", Description: "A date field of the created machine: the i-th record is due i x due_every_days days from the day the step is taken."},
				"due_every_days": {Type: "INTEGER", Description: "Days between records, at least 1."},
				"amount_field":   {Type: "STRING", Description: "A number or money field of the created machine."},
				"amount_from":    {Type: "STRING", Description: "A number or money field of THIS machine, split evenly across the records."},
			}},
	},
}

var generatedEnsureSchema = geminiSchema{
	PropertyOrdering: []string{"id", "sum_of", "per", "at_least", "at_most", "reason"},
	Type:             "OBJECT",
	Required:         []string{"id", "sum_of", "per"},
	Properties: map[string]geminiSchema{
		"id":       {Type: "STRING", Description: "Starts with ens_, e.g. ens_no_negative_stock."},
		"sum_of":   {Type: "STRING", Description: "A number (or money) field of this same machine whose values are added up across records."},
		"per":      {Type: "STRING", Description: "A text, status, person or relation field of this same machine: records sharing its value are totalled together (an item, a period)."},
		"at_least": {Type: "NUMBER", Description: "The total may not go below this (0 for \"never negative\"). Give at_least, at_most, or both."},
		"at_most":  {Type: "NUMBER", Description: "The total may not go above this (a fund, a capacity)."},
		"reason":   {Type: "STRING", Description: "What a refused write is told, in the user's language."},
	},
}

var generatedEventSchema = geminiSchema{
	PropertyOrdering: []string{"id", "service", "on_create", "on", "when_equals", "summary", "recipient_field", "recipient_role", "recipient_group_field", "preference_key"},
	Type:             "OBJECT",
	Required:         []string{"id", "summary"},
	Properties: map[string]geminiSchema{
		"id":                    stringSchema,
		"service":               {Type: "STRING", Enum: []string{EventServiceLogActivity, EventServiceSendNotification}, Description: "\"log_activity\" (the default) writes an activity-feed entry; \"send_notification\" tells a person."},
		"recipient_field":       {Type: "STRING", Description: "For send_notification: a PERSON field of this same machine; the person named in it on the record is told. Name exactly one of recipient_field, recipient_role and recipient_group_field. Not for log_activity."},
		"recipient_role":        {Type: "STRING", Description: "For send_notification: one of this application's roles; everyone holding it is told, whoever the record names (\"every coordinator\")."},
		"recipient_group_field": {Type: "STRING", Description: "For send_notification: a GROUP field of this same machine; every member of the group it holds on the record is told."},
		"preference_key":        {Type: "STRING", Enum: notificationPreferenceEnum(), Description: "Required for send_notification: which of the recipient's own notification settings also gates the email (the in-app notice is always written). \"assigned\" for being given something to do, \"decided\" for an outcome, \"sla_breach\" for lateness."},
		"on":                    {Type: "STRING", Description: "A field id this event fires when it changes. Omit if on_create is true."},
		"when_equals":           {Type: "STRING", Description: "Optional: narrow \"on\" to firing only when the field reaches this exact value."},
		"on_create":             boolSchema,
		"summary":               {Type: "STRING", Description: "The activity-log sentence this event writes, e.g. \"{fld_title} submitted\"."},
	},
}

var generatedMachineSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "fields", "completion", "card_fields", "status_categories", "views", "permissions", "transitions", "events", "ensure", "frozen_when"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "fields"},
	Properties: map[string]geminiSchema{
		"id":     {Type: "STRING", Description: "Must match ^mch_[a-z][a-z0-9_]*$."},
		"name":   stringSchema,
		"fields": {Type: "ARRAY", Items: &generatedFieldSchema},
		"completion": {
			Type:             "OBJECT",
			Description:      "Optional: which status field and which of its options mean a record is finished; gives each record a mark-done and reopen action.",
			Required:         []string{"field", "done"},
			PropertyOrdering: []string{"field", "done"},
			Properties: map[string]geminiSchema{
				"field": {Type: "STRING", Description: "A status field of this machine."},
				"done":  {Type: "STRING", Description: "One of that field's options."},
			},
		},
		"status_categories": {
			Type:             "OBJECT",
			Description:      "Optional but recommended for any machine with a status: which category each status option belongs to, so a status shows in the right colour (done is green, attention amber, stopped red, active blue). Every option a transition starts or ends in must be listed. If completion is set, only its done option may be categorised done.",
			Required:         []string{"field", "values"},
			PropertyOrdering: []string{"field", "values"},
			Properties: map[string]geminiSchema{
				"field": {Type: "STRING", Description: "A status field of this machine."},
				"values": {Type: "ARRAY", Items: &geminiSchema{
					Type:             "OBJECT",
					Required:         []string{"option", "category"},
					PropertyOrdering: []string{"option", "category"},
					Properties: map[string]geminiSchema{
						"option":   {Type: "STRING", Description: "One of that field's options."},
						"category": {Type: "STRING", Enum: GeneratableStatusCategories, Description: "start: new, not worked on. active: in progress. attention: late, blocked or waiting on someone. done: finished well. stopped: ended without finishing (rejected, cancelled)."},
					},
				}},
			},
		},
		"card_fields": {Type: "ARRAY", Items: &generatedCardFieldSchema, Description: "What a card shows on a board or cards view. Required (with a title entry) before any board or cards view."},
		"views":       {Type: "ARRAY", Items: &generatedViewSchema, Description: "How this machine's records are arranged; the first is what opens by default. A board is columns per status."},
		"permissions": {Type: "ARRAY", Items: &generatedPermissionSchema},
		"transitions": {Type: "ARRAY", Items: &generatedTransitionSchema},
		"ensure":      {Type: "ARRAY", Items: &generatedEnsureSchema},
		"frozen_when": {Type: "OBJECT", Required: []string{"via", "field", "equals"}, PropertyOrdering: []string{"via", "field", "equals"},
			Description: "Freezes a record while the record it belongs to holds a status (e.g. a voucher of a closed period: not edited, not deleted, none added). via: a RELATION field of this machine; field: a STATUS field of the machine via points at; equals: one of its options.",
			Properties:  map[string]geminiSchema{"via": stringSchema, "field": stringSchema, "equals": stringSchema}},
		"events": {Type: "ARRAY", Items: &generatedEventSchema},
	},
}

var generatedApplicationSchema = geminiSchema{
	PropertyOrdering: []string{"id", "name", "description", "icon", "color", "roles", "publisher_role", "machines", "navigation"},
	Type:             "OBJECT",
	Required:         []string{"id", "name", "machines", "navigation"},
	Properties: map[string]geminiSchema{
		"id":          {Type: "STRING", Description: "Must match ^app_[a-z][a-z0-9_]*$."},
		"name":        stringSchema,
		"description": stringSchema,
		"icon":        {Type: "STRING", Enum: declarableIconEnum(), Description: "In an update, leave out unless the person asked for an icon."},
		"color":       {Type: "STRING", Enum: applicationColorEnum(), Description: "In an update, leave out unless the person asked for a color."},
		"roles":       {Type: "ARRAY", Items: &stringSchema},
		"publisher_role": {
			Type:        "STRING",
			Description: "new_application only; leave empty for an update. Must be one of roles. Which role the person you are talking to will hold themselves once this is published -- ask them plainly which one that is before setting \"change\"; never guess or pick one on their behalf.",
		},
		"machines": {Type: "ARRAY", Items: &generatedMachineSchema},
		"navigation": {
			Type:        "ARRAY",
			Description: "This application's menu, in order; the first entry is where its Home card opens. Ask the person which machines they want in the menu and what each entry should say before setting \"change\"; never decide it for them.",
			Items:       &generatedMenuItemSchema,
		},
	},
}

var generatedListSchema = geminiSchema{
	PropertyOrdering: []string{"scope", "person_field", "sort_field", "direction", "limit", "columns", "search_fields", "filter_field", "add_label", "density", "group_by", "row_tone"},
	Type:             "OBJECT",
	Required:         []string{"scope", "limit"},
	Properties: map[string]geminiSchema{
		"scope":         {Type: "STRING", Enum: []string{ListScopeMine, ListScopeAll}, Description: "\"mine\": only the records whose person field is the person looking. \"all\": every record. Ask the person which each list is; never pick for them. This only chooses what the list shows -- it does not stop anyone opening the other records elsewhere."},
		"person_field":  {Type: "STRING", Description: "Required for \"mine\": a person field of that machine saying whose record it is. Leave empty for \"all\"."},
		"sort_field":    {Type: "STRING", Description: "A field of that machine, or created_at / updated_at. Leave empty for newest first by created_at."},
		"direction":     {Type: "STRING", Enum: sortDirectionEnum()},
		"limit":         {Type: "INTEGER", Description: "How many records the list shows at most, 1 to 200."},
		"columns":       {Type: "ARRAY", Items: &stringSchema, Description: "Optional: makes the page a table instead of a plain list of titles. The fields of that machine to show as columns, 1 to 8, in order; the first column links to the record. Choose the fields a person scanning the list needs; ask when unsure."},
		"search_fields": {Type: "ARRAY", Items: &stringSchema, Description: "Only with columns: text fields (type text or long_text) a search box above the table looks in, at most 4."},
		"filter_field":  {Type: "STRING", Description: "Only with columns, and only with scope \"all\": a status field of that machine; chips above the table filter by its options and count each one."},
		"density":       {Type: "STRING", Enum: []string{"compact", "regular", "roomy"}, Description: "Only with columns: how close the rows sit. Leave empty for regular."},
		"group_by":      {Type: "STRING", Description: "Only with columns: a field of that machine (usually a status) whose values fold the rows into groups, each with its count."},
		"row_tone":      {Type: "STRING", Enum: []string{"date:tone", "status:tone"}, Description: "Only with columns: each row takes the tone its record has -- date:tone makes an open record whose date has passed red (the machine needs a card_fields date role), status:tone the status's own tone (needs a status role)."},
		"add_label":     {Type: "STRING", Description: "Optional: the words on a button above the list (in the person's language, e.g. \"Tambah tugas\") that opens a form to add a record of that machine. Set it whenever the person asked for an add/new button, or the list is where they would expect to add. Never name a menu entry \"Add ...\" unless it carries this."},
	},
}

var generatedByStatusSchema = geminiSchema{
	PropertyOrdering: []string{"status_field", "detail_label", "limit"},
	Type:             "OBJECT",
	Required:         []string{"status_field", "detail_label", "limit"},
	Properties: map[string]geminiSchema{
		"status_field": {Type: "STRING", Description: "A status field of that machine; one tile per option."},
		"detail_label": {Type: "STRING", Description: "What the menu entry for the page a tile opens says, in the person's own language (for example \"Records in a status\")."},
		"limit":        {Type: "INTEGER", Description: "How many records that page shows at most, 1 to 200."},
	},
}

var generatedQueueSchema = geminiSchema{
	PropertyOrdering: []string{"date_field", "limit", "words"},
	Type:             "OBJECT",
	Required:         []string{"limit"},
	Properties: map[string]geminiSchema{
		"date_field": {Type: "STRING", Description: "Optional: a date field of that machine that its card_fields projects with role date. Gives sections Overdue, Due today, Next 7 days and Later; the machine must also declare completion. Leave empty to section the queue by the machine's open statuses instead."},
		"limit":      {Type: "INTEGER", Description: "How many records each section shows at most, 1 to 200."},
		"words": {Type: "OBJECT", Description: "The page's headings and labels in the person's own language; leave any out to read in English.",
			PropertyOrdering: []string{"overdue", "due_today", "next_week", "later", "recently_done", "empty", "all", "add"},
			Properties: map[string]geminiSchema{
				"overdue":       {Type: "STRING"},
				"due_today":     {Type: "STRING"},
				"next_week":     {Type: "STRING"},
				"later":         {Type: "STRING"},
				"recently_done": {Type: "STRING"},
				"empty":         {Type: "STRING"},
				"all":           {Type: "STRING"},
				"add":           {Type: "STRING"},
			}},
	},
}

var generatedMenuItemSchema = geminiSchema{
	PropertyOrdering: []string{"id", "label", "machine_id", "list", "by_status", "queue", "parent", "visible_to", "badge"},
	Type:             "OBJECT",
	Required:         []string{"label"},
	Properties: map[string]geminiSchema{
		"id":         {Type: "STRING", Description: "Only for a menu item that already exists: keep its id exactly, so it keeps where it leads. Leave empty for a new item."},
		"parent":     {Type: "STRING", Description: "Only for a menu item that already exists: the id of another top-level item of this menu to hang it under (one level only; an item that others hang under cannot itself hang under one). Empty keeps it a top-level item."},
		"visible_to": {Type: "ARRAY", Items: &stringSchema, Description: "Only for a menu item that already exists: roles this application declares; only they are offered the item and may open its screen. Once set it cannot be emptied again."},
		"badge":      {Type: "STRING", Description: "Only for a menu item that already exists: the id of a Dataset of this application's machines that lists records (ds_...); the item then shows how many it selects for the viewer."},
		"label":      {Type: "STRING", Description: "What the menu entry says, in the person's own language."},
		"machine_id": {Type: "STRING", Description: "Required for a new item: one of this application's own machine ids; the entry opens that machine's list."},
		"list":       withDescription(generatedListSchema, "For a NEW item: makes the entry a list page of that machine's records (all of them, or only the viewer's own). Needs the machine to have a card_fields entry with role title. For an existing item that is already a list (it comes back with its list): return it with the changes -- its columns, order, search, chips or add button; the page is written again from it. Never give an existing item that is not a list a list."),
		"by_status":  withDescription(generatedByStatusSchema, "Only for a NEW item, instead of list: the entry is a page of one tile per status counting that machine's records, each tile opening a page of the records in that status. Needs a card_fields entry with role title. Never on an existing item."),
		"queue":      withDescription(generatedQueueSchema, "Only for a NEW item, instead of list and by_status: the entry is the machine's work queue -- figures, then what is open in sections, with a button on each row for the next step and a form to add one. Needs a card_fields entry with role title, status_categories on the machine, and (without date_field) a card_fields entry with role status. Never on an existing item."),
	},
}

var generatedChangeSchema = geminiSchema{
	PropertyOrdering: []string{"kind", "target_app_id", "application"},
	Type:             "OBJECT",
	Required:         []string{"kind", "application"},
	Properties: map[string]geminiSchema{
		"kind":          {Type: "STRING", Enum: []string{KindNewApplication, KindUpdateApplication}},
		"target_app_id": {Type: "STRING", Description: "Required when kind is \"update_application\": the id of the installed application being updated."},
		"application": withDescription(generatedApplicationSchema,
			"The whole application. For update_application: start from its current definition, keep every id, change what was asked, and leave everything else exactly as it is -- anything left out is read as a removal."),
	},
}

var capabilityGapSchema = geminiSchema{
	PropertyOrdering: []string{"capability_key", "requested", "kind", "class", "ideal_capability", "wanted_shape", "where", "intent", "benchmark", "note"},
	Type:             "OBJECT",
	Required:         []string{"capability_key", "requested", "kind", "class", "ideal_capability", "note"},
	Properties: map[string]geminiSchema{
		"capability_key":   {Type: "STRING", Description: "A stable lowercase slug (words joined by hyphens) naming the capability, not the application. When the need is the same as one in the known list, reuse that key exactly."},
		"requested":        {Type: "STRING", Description: "A short human name for the missing capability, e.g. \"multi-person voting approval\"."},
		"kind":             {Type: "STRING", Enum: []string{GapRuntime, GapAssistant}, Description: "\"runtime\" when the runtime cannot express this at all yet; \"assistant\" when the runtime can (it is in the first list) but you cannot write it yet."},
		"class":            {Type: "STRING", Enum: GapClasses, Description: "ui_component: a display component common UI frameworks offer; workflow_logic: state moves, roles, rules, automation; data_model: field types, relations, computed values; configuration: settings a user changes without editing metadata; integration: something outside the runtime."},
		"ideal_capability": {Type: "STRING", Description: "The capability the runtime ideally SHOULD have, in general terms that fit any application, not only this request: what it can do, for whom, and how it behaves. Two or three sentences with the context a developer needs to build it."},
		"wanted_shape":     {Type: "STRING", Description: "Optional. A sketch of how Runtime Metadata would ideally declare it, e.g. the YAML keys."},
		"where":            {Type: "STRING", Description: "Optional. The machine or screen where the need arose."},
		"intent":           {Type: "STRING", Description: "Optional. The person's own words, briefly."},
		"benchmark":        {Type: "STRING", Description: "Optional. The counterpart in a common UI framework or workflow engine, when there is one."},
		"note":             {Type: "STRING", Description: "One sentence of context: what the user actually asked for."},
	},
}

// replySchema is the whole response shape (see Reply) -- message is always required; change and
// capability_gaps are optional and may appear together (a reply can propose most of an application
// and still name what it left out).
var replySchema = geminiSchema{
	PropertyOrdering: []string{"message", "capability_gaps", "change"},
	Type:             "OBJECT",
	Required:         []string{"message"},
	Properties: map[string]geminiSchema{
		"message":         stringSchema,
		"change":          generatedChangeSchema,
		"capability_gaps": {Type: "ARRAY", Items: &capabilityGapSchema, Description: "Every part of the request you could not satisfy, one entry each. Empty or absent when nothing was left out."},
	},
}

func withDescription(s geminiSchema, d string) geminiSchema {
	s.Description = d
	return s
}
