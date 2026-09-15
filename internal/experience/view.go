package experience

import (
	"fmt"

	"menata.app/internal/data"
	"menata.app/internal/domain"
)

// Column is one group of records in a board Layout. ID is the related record's id when the
// column comes from a relation-based grouping (ROADMAP.md Phase 10's ordered Lists); empty for
// the original status-Options grouping (Phase 5), which keys by Label instead.
type Column struct {
	ID      string
	Label   string
	Records []*data.Record
}

// GroupRecords partitions records into ordered Columns keyed by the value of m.View.GroupBy.
//
// If columns is nil, column order/labels come from GroupBy's own status Options (Phase 5's
// original behavior) and a record whose value isn't among them falls into a trailing "Other"
// column. If columns is provided, it is used directly instead -- the relation-based case (Phase
// 10), where columns are real records of another Machine (e.g. mch_list) the caller already
// fetched and labeled, since this package performs no I/O itself.
func GroupRecords(m *domain.Machine, records []*data.Record, columns []Column) []Column {
	var index map[string]int

	if columns == nil {
		groupField, ok := m.FieldByID(m.View.GroupBy)
		if !ok {
			return nil
		}
		columns = make([]Column, 0, len(groupField.Options)+1)
		index = make(map[string]int, len(groupField.Options))
		for _, opt := range groupField.Options {
			index[opt] = len(columns)
			columns = append(columns, Column{Label: opt})
		}
	} else {
		index = make(map[string]int, len(columns))
		for i, c := range columns {
			index[c.ID] = i
		}
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
