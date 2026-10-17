package domain

import "strconv"

// FormRoute is the runtime's generic create route for a Machine (`POST /machines/{machineID}/records`). Like
// MachineListRoute it belongs to the runtime and not to an Application, so it is assembled in one place; the
// route still decides for itself who may create, validates, applies defaults and runs the Machine's events.
func FormRoute(machineID string) string {
	return "/machines/" + machineID + "/records"
}

// FormInput is one control of a create form, already decided: which Field it fills, what it is called, what
// sort of control it is, and what it starts as.
type FormInput struct {
	FieldID  string
	Label    string
	Kind     InputKind
	Required bool
	// Default is the Field's declared default as the string a control shows ("" when none).
	Default string
	// Options are a select's values, in the order the Machine declares them.
	Options []string
	// Reference is the Machine a relation or person select chooses a record of; its Choices are that Machine's records,
	// which only the Data Plane can list, so the domain names the target and the composition fills them in.
	Reference string
	// Choices are a reference select's records, each a stored id and the words it is shown by (the related record's
	// label, a member's name).
	Choices []FormChoice
}

// FormChoice is one record a reference select offers.
type FormChoice struct {
	Value, Label string
}

// InputChoiceSep separates a reference choice's stored id from its label inside an `Input`'s options (one choice per
// InputOptionsSep line). A tab, which neither an id nor a record's label holds.
const InputChoiceSep = "\t"

// Form methods: the verb a Form lowers to. It is derived from the binding's mode (create posts, update patches)
// and never written, for the reason `action` is not.
const (
	FormMethodPost  = "post"
	FormMethodPatch = "patch"
)

// ButtonMethodDelete is the verb a `Button` bound with `write: delete` sends. Derived, never written, like a Form's.
const ButtonMethodDelete = "delete"

// ButtonMethodPost is the verb a `Button` bound with `write: move` sends. Derived, never written.
const ButtonMethodPost = "post"

// ButtonMethodPatch is the verb a `Button` bound with `write: transition` sends: a PATCH of one Field of the record,
// carried as the Button's own name/value pair. Derived, never written.
const ButtonMethodPatch = "patch"

// The two directions a `write: move` Button names. A direction is relative to the order the records are listed in,
// which for a move is the Machine's own (`sort_order`), so "up" is toward the first record.
const (
	MoveUp   = "up"
	MoveDown = "down"
)

// RecordMoveRoute is the runtime's generic route that moves one record a step in its Machine's order
// (`POST /machines/{machineID}/records/{id}/move?direction=up|down`). recordRoute is the record's own route.
func RecordMoveRoute(recordRoute, direction string) string {
	return recordRoute + "/move?direction=" + direction
}

// CreateFormInputs is every control a create form for m asks for, and the Fields it cannot yet draw.
//
// The rule is the generic create form's own (`rendering.newCardPopover`): a Field is asked for unless it is
// computed (derived on save), stamped (written by the runtime from the request) or a file (a form of a different
// encoding). **A relation or person Field is a select of the related Machine's records** (2026-10-17: an instalment is
// created for a contract, mockup K01f) whose Choices the composition fills, the way the generic create form's picker
// does. **A group Field is not drawn, and is returned in `unsupported`**: its options are the Workspace's Groups, which
// no page reads yet. The loader turns a required one into an error, so a Form over such a Machine fails at load and
// does not silently omit a Field the Machine marks required.
//
// A **status the Machine governs** (any `transitions:` names it) that declares a `default:` is not asked either: a
// new record starts at that default and leaves it only along the declared edges, so a select of every option would
// offer a borrower a loan that is already disbursed. Without a default the select stays, since nothing else would
// give the record a status.
func (m *Machine) CreateFormInputs() (inputs []FormInput, unsupported []Field) {
	for _, f := range m.Fields {
		if f.Compute != nil || f.Stamp != "" || f.Type == FieldTypeFile {
			continue
		}
		if f.Type == FieldTypeStatus && f.Default != nil && m.GovernsTransitions(f.ID) {
			continue
		}
		if f.Type == FieldTypeGroup {
			unsupported = append(unsupported, f)
			continue
		}
		in := formInputOf(f)
		switch d := f.Default.(type) {
		case nil:
		case string:
			in.Default = d
		case bool:
			in.Default = strconv.FormatBool(d)
		case float64:
			in.Default = strconv.FormatFloat(d, 'f', -1, 64)
		}
		inputs = append(inputs, in)
	}
	return inputs, unsupported
}

// EditFormInputs is every control an update form for one record of m asks for, each starting as the record's
// current value (in `Default`, the one place a control's starting value lives).
//
// It is narrower than the create form on purpose, and each exclusion is a way the generic `PATCH` route would
// otherwise do something the viewer did not mean:
//
//   - a **boolean** is left out, because an unticked checkbox sends nothing and `PATCH` changes only the Fields a
//     request names, so a form could tick a box and never clear it;
//   - a **status the Machine governs** (any `transitions:` names it) is left out, because it moves along those
//     edges and is reserved to the Action that names one -- a free select would offer moves the route refuses. A
//     status no Transition names is unrestricted (`GovernsTransitions`), so it is an ordinary choice and is asked
//     for, as a select of its options;
//   - a reference or group Field is left out for the create form's reason (its options are another Machine's
//     records, 007 §20), and costs nothing here: a record that exists already holds its required values.
//
// Computed, stamped and file Fields are left out as they are there. A record's values arrive as stored (`any`).
func (m *Machine) EditFormInputs(values map[string]any) []FormInput {
	var inputs []FormInput
	for _, f := range m.Fields {
		if f.Compute != nil || f.Stamp != "" || f.Type == FieldTypeFile || f.Type == FieldTypeGroup ||
			f.Type == FieldTypeBoolean || (f.Type == FieldTypeStatus && m.GovernsTransitions(f.ID)) || f.IsReference() {
			continue
		}
		// A Field the Machine locks for this record (`locked_when:`, `set_once:`) is not asked: the route refuses a change.
		if locked, _ := m.LockedField(f, values); locked {
			continue
		}
		in := formInputOf(f)
		switch v := values[f.ID].(type) {
		case string:
			in.Default = v
		case float64:
			in.Default = strconv.FormatFloat(v, 'f', -1, 64)
		}
		inputs = append(inputs, in)
	}
	return inputs
}

// formInputOf is the control a Field is drawn as, before any starting value.
func formInputOf(f Field) FormInput {
	in := FormInput{FieldID: f.ID, Label: f.Name, Required: f.Required && f.Type != FieldTypeBoolean}
	if f.IsReference() {
		in.Kind, in.Reference = InputSelect, f.RelatedMachine
		return in
	}
	switch f.Type {
	case FieldTypeStatus:
		in.Kind, in.Options = InputSelect, f.Options
	case FieldTypeLongText:
		in.Kind = InputLongText
	case FieldTypeNumber, FieldTypeMoney:
		in.Kind = InputNumber
	case FieldTypeDate:
		in.Kind = InputDate
	case FieldTypeBoolean:
		in.Kind = InputBoolean
	default:
		in.Kind = InputText
	}
	return in
}
