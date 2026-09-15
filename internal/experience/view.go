package experience

import (
	"fmt"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// Column is one group of records in a board Layout.
type Column struct {
	Label   string
	Records []*data.Record
}

// GroupRecords partitions records into ordered Columns keyed by the value of m.View.GroupBy.
// Column order follows that Field's declared Options; a record whose value isn't among them
// falls into a trailing "Other" column. Returns nil if GroupBy doesn't name a real Field.
func GroupRecords(m *domain.Machine, records []*data.Record) []Column {
	groupField, ok := fieldByID(m, m.View.GroupBy)
	if !ok {
		return nil
	}

	columns := make([]Column, 0, len(groupField.Options)+1)
	index := make(map[string]int, len(groupField.Options))
	for _, opt := range groupField.Options {
		index[opt] = len(columns)
		columns = append(columns, Column{Label: opt})
	}

	otherIndex := -1
	for _, r := range records {
		value := fmt.Sprint(r.Values[m.View.GroupBy])
		idx, known := index[value]
		if !known {
			if otherIndex == -1 {
				columns = append(columns, Column{Label: "Other"})
				otherIndex = len(columns) - 1
			}
			idx = otherIndex
		}
		columns[idx].Records = append(columns[idx].Records, r)
	}
	return columns
}

func fieldByID(m *domain.Machine, id string) (domain.Field, bool) {
	for _, f := range m.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return domain.Field{}, false
}
