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
