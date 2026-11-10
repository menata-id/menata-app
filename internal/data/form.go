package data

import (
	"net/url"
	"strconv"

	"menata.app/internal/domain"
)

// ValuesFromForm converts URL-encoded form values into a record value map, coercing each value
// per its Field's declared type. A text field absent from the form is omitted entirely (so
// ValidateRecord reports it the same way it would report a JSON caller omitting the key); a
// boolean field is always present, since an unchecked HTML checkbox submits nothing and absence
// must mean false, not "not provided."
func ValuesFromForm(m *domain.Machine, form url.Values) map[string]any {
	values := make(map[string]any, len(m.Fields))
	for _, f := range m.Fields {
		if f.Compute != nil || f.Stamp != "" {
			continue // never an input; ApplyComputed derives it, ApplyStamps stamps it
		}
		if f.Type == domain.FieldTypeBoolean {
			values[f.ID] = ticked(form[f.ID])
			continue
		}

		raw := form.Get(f.ID)
		if raw == "" {
			continue
		}
		if f.Type.IsNumeric() {
			if n, err := strconv.ParseFloat(raw, 64); err == nil {
				values[f.ID] = n
				continue
			}
		}
		values[f.ID] = raw
	}
	return values
}

// ticked reads a checkbox's answer: absent is false (an unticked box sends nothing), and so is a lone "false" (the
// hidden answer a switch sends first, InputSwitch); any other value -- "on", or "false" followed by the box's own --
// is true.
func ticked(sent []string) bool {
	for _, v := range sent {
		if v != "false" {
			return true
		}
	}
	return false
}
