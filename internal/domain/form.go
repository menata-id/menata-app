package domain

import "strconv"

// FormRoute is the runtime's generic create route for a Machine (`POST /machines/{machineID}/records`). Like
// MachineListRoute it belongs to the runtime and not to an Application, so it is assembled in one place; the
// route still decides for itself who may create, validates, applies defaults and runs the Machine's events.
func FormRoute(machineID string) string {
	return "/machines/" + machineID + "/records"
}

// FormThenParam is the request field a create Form sends `then: record` in, and FormThenRecord its one value: the create
// route answers by opening the new record (its Machine's record page when it has one) rather than redrawing the form.
const (
	FormThenParam  = "_then"
	FormThenRecord = "record"
)

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
	// Fixed is a control whose value came from the request (a Form's `values:`): drawn as its words, carried in a hidden
	// input, so "New instalment" opened from a contract shows the contract and does not offer another.
	Fixed bool
	// Choices are a reference select's records, each a stored id and the words it is shown by (the related record's
	// label, a member's name).
	Choices []FormChoice
	// Note and Flag belong to a proposed value (a document reader's answer): Note is where it was read, Flag the words of a
	// warning when it was inferred rather than quoted or could not be matched. Both empty is an ordinary control.
	Note, Flag string
}

// Empty says a reading found nothing: no Field holds a value and no row was proposed. A document that is not the kind the form
// is for gives this, and a form "filled" with nothing is not a proposal a person should be sent to.
func (p ExtractionProposal) Empty() bool {
	for _, v := range p.Values {
		if v.Value != "" {
			return false
		}
	}
	for _, kind := range p.Rows {
		if len(kind) > 0 {
			return false
		}
	}
	return true
}

// ProposedValue is one answer of a document reader, after the runtime checked it against its Field: Value is the string
// the control starts as ("" when the document did not say or the answer did not fit the Field), Source where it was read, and
// Review whether a person should look twice (it was inferred, or it did not fit and was left empty).
type ProposedValue struct {
	Value  string `json:"value"`
	Source string `json:"source,omitempty"`
	Review bool   `json:"review,omitempty"`
}

// ExtractionProposal is what a reading proposed, in the shape a page's form fills itself from: an answer per Field, and per
// kind of rows (the index into FormExtract.Rows) the rows with an answer per Field.
type ExtractionProposal struct {
	Values map[string]ProposedValue     `json:"values"`
	Rows   [][]map[string]ProposedValue `json:"rows"`
}

// FormChoice is one record a reference select offers.
type FormChoice struct {
	Value, Label string
}

// InputChoiceSep separates a reference choice's stored id from its label inside an `Input`'s options (one choice per
// InputOptionsSep line). A tab; lowering replaces any tab or line break a label holds (ir.oneField), so a record's title
// cannot add a choice of its own.
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

// ButtonMethodPrint is what a `Button` bound with `write: print` is: no request at all -- it opens the browser's print dialog
// for the page (mockup K02e's Print). Derived, never written; it has no route, and its pair is no route.
const ButtonMethodPrint = "print"

// ButtonMethodPDF is what a `Button` bound with `write: pdf` is: a plain request for the page's own address with `/pdf`
// added (`/pages/<nav>[/<id>]/pdf`), answered with the page as a PDF file. Derived, never written; its address is the
// browser's own location, so a filtered list prints filtered.
const ButtonMethodPDF = "pdf"

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
		if f.Compute != nil || f.Stamp != "" || f.Numbering != nil || f.Type == FieldTypeFile {
			continue
		}
		if f.Type == FieldTypeStatus && f.Default != nil && m.GovernsTransitions(f.ID) {
			continue
		}
		// A Field an edge writes when it is taken (`sets:`: an approval date, a payment date) is the runtime's, never typed.
		if m.SetByTransition(f.ID) {
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

// SetByTransition says some Transition of m writes the Field itself when it is taken (`sets:`), so a person is not asked for it.
func (m *Machine) SetByTransition(fieldID string) bool {
	for _, t := range m.Transitions {
		if _, ok := t.Sets[fieldID]; ok {
			return true
		}
	}
	return false
}

// SelectInputs is the controls a Form that declares `fields:` asks for: those Fields of inputs, in that order. An id that is
// no control of the form is left out here (the loader refuses it, so a loaded Form never names one).
func SelectInputs(inputs []FormInput, ids []string) []FormInput {
	if len(ids) == 0 {
		return inputs
	}
	byID := make(map[string]FormInput, len(inputs))
	for _, in := range inputs {
		byID[in.FieldID] = in
	}
	out := make([]FormInput, 0, len(ids))
	for _, id := range ids {
		if in, ok := byID[id]; ok {
			out = append(out, in)
		}
	}
	return out
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
		if f.Compute != nil || f.Stamp != "" || f.Numbering != nil || f.Type == FieldTypeFile || f.Type == FieldTypeGroup ||
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

// Input is the control f is drawn as on a page's form, before any starting value: the shape a transition's own form
// asks its Fields in (a file as a file input).
func (f Field) Input() FormInput {
	if f.Type == FieldTypeFile {
		return FormInput{FieldID: f.ID, Label: f.Name, Kind: InputFile, Required: true}
	}
	return formInputOf(f)
}

// TransitionInputs are what moving a record along e asks for: the Fields that become required when e's Field takes e's
// `to` (`required_when:`), each required. "Mark as paid" asks for the proof of payment, so the form and the edge are
// one submit (mockup K02d).
func (m *Machine) TransitionInputs(e Transition) []FormInput {
	var out []FormInput
	for _, f := range m.Fields {
		if rw := f.RequiredWhen; rw != nil && rw.Field == e.Field && rw.Equals == e.To {
			in := f.Input()
			in.Required = true
			out = append(out, in)
		}
	}
	return out
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
