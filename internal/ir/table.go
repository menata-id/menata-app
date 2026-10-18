package ir

import (
	"fmt"
	"strconv"
	"strings"

	"menata.app/internal/domain"
)

// MaxTableColumns bounds a declared Table: a table wider than this is a screen nobody can read, and the limit stops a
// typo from drawing a hundred header cells.
const MaxTableColumns = 12

// tableHeadersProp and tableAlignsProp are the two properties lowering produces for a Table from its `columns:`. An
// author never writes them (a page that does is refused), for the reason `truncated` is not written: they are a fact
// the declaration already states, and a second statement of it could disagree.
const (
	tableHeadersProp = "headers"
	tableAlignsProp  = "aligns"
)

// lowerTable turns a Table bound with `rows: records` into its header row and one row of cells per record. The cells
// are composed here, as a Collection's items are: each is a node the generic renderer already draws -- a status as a
// StatusBadge, a record's own link as a `link`, anything else as a paragraph -- so the Table Component decides nothing
// about a value and the renderer reads no record.
//
// What a Table does not take, refused by name: a dimension binding (`BindableComponents` lists records only: several
// Measures per row is the next case, not built), a measure, a group, children (the cells are produced, not written), `from:`, `to:` and `param:`, and any property that
// speaks of a Collection's list.
func lowerTable(n domain.PageNode, r Resolver, path string) ([]UINode, error) {
	b := n.Binding
	dimension := b.Rows == domain.PageRowsDimension
	switch {
	case b.Measure != "" || b.Group != "":
		return nil, fmt.Errorf("%s: a Table lists records or a Dimension's rows, so its binding takes no measure and no group", path)
	case dimension && len(n.Children) > 0:
		return nil, fmt.Errorf("%s: a Table of a Dimension's rows has no row actions -- a row is a group of records, not one", path)
	case len(n.From) > 0 || n.To != "" || n.Param != "":
		return nil, fmt.Errorf("%s: from:, to: and param: do not belong on a Table; a column takes its role with from: inside columns:", path)
	}
	for _, p := range []string{tableHeadersProp, tableAlignsProp} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: comes from columns: and is never written", path, p)
		}
	}
	cols := n.Columns
	if len(cols) == 0 {
		return nil, fmt.Errorf("%s: a Table declares columns:", path)
	}
	if len(cols) > MaxTableColumns {
		return nil, fmt.Errorf("%s: a Table has at most %d columns, and %d are declared", path, MaxTableColumns, len(cols))
	}
	headers := make([]string, len(cols))
	aligns := make([]string, len(cols))
	for i, c := range cols {
		where := fmt.Sprintf("%s.columns[%d]", path, i)
		switch {
		case strings.TrimSpace(c.Header) == "" || strings.ContainsAny(c.Header, "\n\r"):
			return nil, fmt.Errorf("%s: header: words on one line", where)
		case countNonEmpty(c.From, c.Field, c.Related) != 1:
			return nil, fmt.Errorf("%s: a column shows exactly one of from: (a Projection role, or row:/measure: on a Dimension's rows), field: (a Field id) or related: (a Relation)", where)
		case c.Of != "" && c.Field == "" && !strings.HasPrefix(c.From, "measure:"):
			return nil, fmt.Errorf("%s: of: draws a fraction of two figures, so it sits beside field: or from: measure:<id>", where)
		case dimension && (c.Field != "" || c.Related != "" || c.Href != ""):
			return nil, fmt.Errorf("%s: a Dimension's row is not a record: its columns are from: %s, %s<id> or row:<field>", where, domain.PageRowLabelRole, "measure:")
		case c.Href != "" && c.Href != domain.PageRecordRole:
			return nil, fmt.Errorf("%s: href: is %q, and the only address a cell may take is the record's own (href: %s)", where, c.Href, domain.PageRecordRole)
		case c.From == domain.PageRecordRole:
			return nil, fmt.Errorf("%s: from: %s is the record's address, which a cell takes with href: %s, not as its text", where, domain.PageRecordRole, domain.PageRecordRole)
		case c.Align != "" && !domain.KnownTableAligns[domain.TableAlign(c.Align)]:
			return nil, fmt.Errorf("%s: align: %q is not start or end", where, c.Align)
		}
		headers[i] = c.Header
		aligns[i] = string(domain.TableAlignStart)
		if c.Align != "" {
			aligns[i] = c.Align
		}
	}
	var set RecordSet
	if dimension {
		// A Dimension's rows as the records the cells read: its label, its key, every Measure as a reader reads it and as
		// a bare number, and the related record's Fields (a category's type).
		if r.Rows == nil {
			return nil, fmt.Errorf("%s: no resolver for dimension rows", path)
		}
		rows, err := r.Rows(*b)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			rec := map[string]string{domain.PageRowLabelRole: row.Label}
			for id, v := range row.Measures {
				rec[domain.PageMeasureRole(id)] = v
			}
			for id, v := range row.Numbers {
				rec[domain.PageRowNumberRole(id)] = v
			}
			for id, v := range row.Attrs {
				rec["row:"+id] = v
			}
			set.Records = append(set.Records, rec)
		}
	} else {
		if r.Records == nil {
			return nil, fmt.Errorf("%s: no resolver for records", path)
		}
		var err error
		if set, err = r.Records(*b); err != nil {
			return nil, err
		}
	}
	// The Table's children are its row actions (owner decision K1, 2026-10-17): a template lowered once per record, the
	// way a Collection's item is -- an Edit, a Delete, a transition, each offered only where the record's own rules let
	// this viewer -- and drawn as one more cell at the row's end, under an unnamed header.
	var actions *domain.PageNode
	if len(n.Children) > 0 {
		actions = &domain.PageNode{Kind: string(NodeLayout), Type: string(domain.LayoutRow),
			Props: map[string]string{"gap": string(domain.GapTight), "align": string(domain.RowAlignCenter), "justify": string(domain.RowJustifyStart)}, Children: n.Children}
		headers, aligns = append(headers, ""), append(aligns, string(domain.TableAlignEnd))
	}
	// `complete: true` is the Collection's claim, read the same way (owner decision K2): a list cut by its Dataset's bound
	// says so under the rows, with the count it was cut from when the resolver knows it.
	base, err := lowerComplete(n, set, path)
	if err != nil {
		return nil, err
	}
	props := make(map[string]string, len(base)+2)
	for k, v := range base {
		props[k] = v
	}
	if t, cut := props[collectionTruncatedProp]; cut && set.Total > 0 {
		props[collectionTruncatedProp] = t + "/" + strconv.Itoa(set.Total)
	}
	props[tableHeadersProp], props[tableAlignsProp] = strings.Join(headers, "\n"), strings.Join(aligns, ",")
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i, rec := range set.Records {
		for j, c := range cols {
			key := c.From
			switch {
			case c.Field != "":
				key = domain.PageFieldRole(c.Field)
			case c.Related != "":
				key = domain.PageRelatedRole(c.Related)
			}
			text, ok := rec[key]
			// A Dimension row's related-record Field may be unset (a category with no type yet): an empty cell.
			if !ok && !set.ShapeOnly && !(dimension && strings.HasPrefix(key, "row:")) {
				return nil, fmt.Errorf("%s.columns[%d]: the column shows %q, which the bound Dataset's Machine does not project (record %d)", path, j, key, i)
			}
			cell, err := tableCell(c, text, rec)
			if err != nil {
				return nil, fmt.Errorf("%s.columns[%d]: record %d: %w", path, j, i, err)
			}
			out.Children = append(out.Children, cell)
		}
		if actions != nil {
			items, err := lowerRecordItem(*actions, r, path+".children", set, i)
			if err != nil {
				return nil, err
			}
			cell := UINode{Kind: NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": ""}}
			if len(items) == 1 {
				cell = items[0]
			}
			out.Children = append(out.Children, cell)
		}
	}
	footer, err := tableFooter(n, cols, r, path, actions != nil)
	if err != nil {
		return nil, err
	}
	if footer != "" {
		out.Props[tableFooterProp] = footer
	}
	return []UINode{out}, nil
}

// tableFooterProp is a Table's total row, lowered from its columns' `total:` -- one tab-separated cell per column --
// and never written.
const tableFooterProp = "footer"

// tableFooter is the total row: each column's `total:` read from the Table's totals Dataset (`totals:` on the Table, or
// its own binding for a Dimension's rows) as a figure, or two as a fraction ("9/36"), worded by `total_label:`. Empty when
// no column asks for a total.
func tableFooter(n domain.PageNode, cols []domain.PageColumn, r Resolver, path string, actions bool) (string, error) {
	asked := false
	for _, c := range cols {
		asked = asked || c.Total != "" || c.TotalLabel != ""
	}
	if !asked {
		return "", nil
	}
	dataset := n.Totals
	if n.Binding.Rows == domain.PageRowsDimension && dataset == "" {
		dataset = n.Binding.Dataset
	}
	needsDataset := false
	for _, c := range cols {
		needsDataset = needsDataset || c.Total != ""
	}
	if dataset == "" && needsDataset {
		return "", fmt.Errorf("%s: a column asks for a total, so the Table names the Dataset that adds it up in totals:", path)
	}
	if r.Total == nil {
		return "", fmt.Errorf("%s: no resolver for totals", path)
	}
	figure := func(measure string) (string, error) {
		return r.Total(domain.PageBinding{Dataset: dataset, Measure: measure, Rows: domain.PageRowsTotal})
	}
	cells := make([]string, 0, len(cols)+1)
	for _, c := range cols {
		text := ""
		if c.Total != "" {
			a, b, fraction := strings.Cut(c.Total, "/")
			v, err := figure(a)
			if err != nil {
				return "", err
			}
			text = v
			if fraction {
				w, err := figure(b)
				if err != nil {
					return "", err
				}
				text = v + "/" + w
			}
		}
		if c.TotalLabel != "" {
			text = strings.TrimSpace(text + " " + c.TotalLabel)
		}
		cells = append(cells, oneField(text))
	}
	if actions {
		cells = append(cells, "")
	}
	return strings.Join(cells, "\t"), nil
}

func countNonEmpty(values ...string) int {
	n := 0
	for _, v := range values {
		if v != "" {
			n++
		}
	}
	return n
}

// tableCell is one cell as a node the renderer already draws. A cell with an href is a link whatever its role; a
// `status` role, or a status Field named by field:, is a StatusBadge in the tone the Machine's `status_categories:`
// gives the option (resolved by the Data Plane, never here), the way the generic list draws it; anything else is a
// paragraph.
func tableCell(c domain.PageColumn, text string, rec map[string]string) (UINode, error) {
	badge := func(tone string) UINode {
		if !domain.KnownBadgeTones[domain.BadgeTone(tone)] {
			tone = string(domain.ToneNeutral)
		}
		return UINode{Kind: NodeComponent, Type: string(domain.ComponentStatusBadge), Props: map[string]string{"label": text, "tone": tone, "size": string(domain.BadgeCompact)}}
	}
	switch {
	case c.Of != "":
		// A fraction with its bar: the column's figure over its `of:`, drawn from the bare numbers.
		of := c.Of
		valueKey, maxKey := domain.PageRowNumberRole(c.Field), domain.PageRowNumberRole(of)
		if c.Field == "" {
			valueKey = domain.PageRowNumberRole(strings.TrimPrefix(c.From, "measure:"))
		}
		value, max := rec[valueKey], rec[maxKey]
		if value == "" {
			value = "0"
		}
		if max == "" {
			max = "0"
		}
		return UINode{Kind: NodeComponent, Type: string(domain.ComponentProgress), Props: map[string]string{"label": c.Header, "value": value, "max": max, "size": "compact"}}, nil
	case c.Related != "":
		if text == "" {
			return UINode{Kind: NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": ""}}, nil
		}
		row := UINode{Kind: NodeLayout, Type: string(domain.LayoutRow), Props: map[string]string{"gap": string(domain.GapTight), "align": string(domain.RowAlignCenter)},
			Children: []UINode{{Kind: NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": text}}}}
		if status := rec[domain.PageRelatedStatusRole(c.Related)]; status != "" {
			tone := rec[domain.PageRelatedToneRole(c.Related)]
			if !domain.KnownBadgeTones[domain.BadgeTone(tone)] {
				tone = string(domain.ToneNeutral)
			}
			row.Children = append(row.Children, UINode{Kind: NodeComponent, Type: string(domain.ComponentStatusBadge), Props: map[string]string{"label": status, "tone": tone, "size": string(domain.BadgeCompact)}})
		}
		return row, nil
	case c.Href == domain.PageRecordRole:
		href, ok := rec[domain.PageRecordRole]
		if !ok || href == "" {
			return UINode{}, fmt.Errorf("href: %s names no address, which the resolver gives a record of a Machine it can open", domain.PageRecordRole)
		}
		return UINode{Kind: NodeStatic, Type: string(domain.StaticLink), Props: map[string]string{"text": text, "href": href}}, nil
	case c.From == string(domain.CardFieldRoleStatus) && text != "":
		return badge(rec[domain.PageStatusToneRole]), nil
	case c.Field != "" && text != "":
		if tone, ok := rec[domain.PageFieldToneRole(c.Field)]; ok {
			return badge(tone), nil
		}
	}
	return UINode{Kind: NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": text}}, nil
}
