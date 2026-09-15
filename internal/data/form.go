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
		if f.Type == domain.FieldTypeBoolean {
			values[f.ID] = form.Has(f.ID)
			continue
		}

		raw := form.Get(f.ID)
		if raw == "" {
			continue
		}
		if f.Type == domain.FieldTypeNumber {
			if n, err := strconv.ParseFloat(raw, 64); err == nil {
				values[f.ID] = n
				continue
			}
		}
		values[f.ID] = raw
	}
	return values
}
