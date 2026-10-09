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
}

// Form methods: the verb a Form lowers to. It is derived from the binding's mode (create posts, update patches)
// and never written, for the reason `action` is not.
const (
	FormMethodPost  = "post"
	FormMethodPatch = "patch"
)

// CreateFormInputs is every control a create form for m asks for, and the Fields it cannot yet draw.
//
// The rule is the generic create form's own (`rendering.newCardPopover`): a Field is asked for unless it is
// computed (derived on save), stamped (written by the runtime from the request) or a file (a form of a different
// encoding). **A reference or group Field is not drawn, and is returned in `unsupported`**, because its options
// are another Machine's whole record set (007 §20) and a page cannot resolve them without reading it. The
// loader turns a non-empty `unsupported` into an error, so a Form over such a Machine fails at load and does not
// silently omit a Field the Machine marks required.
func (m *Machine) CreateFormInputs() (inputs []FormInput, unsupported []Field) {
	for _, f := range m.Fields {
		if f.Compute != nil || f.Stamp != "" || f.Type == FieldTypeFile {
			continue
		}
		if f.IsReference() || f.Type == FieldTypeGroup {
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
//   - a **status** is left out, because a status moves along the Machine's declared `transitions:` edges and is
//     reserved to the Action that names the edge -- offering it as a free select would offer moves the route
//     refuses;
//   - a reference or group Field is left out for the create form's reason (its options are another Machine's
//     records, 007 §20), and costs nothing here: a record that exists already holds its required values.
//
// Computed, stamped and file Fields are left out as they are there. A record's values arrive as stored (`any`).
func (m *Machine) EditFormInputs(values map[string]any) []FormInput {
	var inputs []FormInput
	for _, f := range m.Fields {
		if f.Compute != nil || f.Stamp != "" || f.Type == FieldTypeFile || f.Type == FieldTypeGroup ||
			f.Type == FieldTypeBoolean || f.Type == FieldTypeStatus || f.IsReference() {
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
	switch f.Type {
	case FieldTypeStatus:
		in.Kind, in.Options = InputSelect, f.Options
	case FieldTypeLongText:
		in.Kind = InputLongText
	case FieldTypeNumber:
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
