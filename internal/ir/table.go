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
	// tableHiddenProp is the places (0-based, comma-joined) of the columns whose header is kept for assistive technology and
	// not drawn (`hide_header: true`), lowered and never written, for the reason the other two are not.
	tableHiddenProp = "hidden"
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
	case len(n.From) > 0 || n.To != "" || n.Param != "":
		return nil, fmt.Errorf("%s: from:, to: and param: do not belong on a Table; a column takes its role with from: inside columns:", path)
	}
	for _, p := range []string{tableHeadersProp, tableAlignsProp, tableHiddenProp} {
		if _, written := n.Props[p]; written {
			return nil, fmt.Errorf("%s: %s: comes from columns: and is never written", path, p)
		}
	}
	// A child marked with a column's place is that column's cell template; the rest are the row actions.
	cellTemplates := map[int]domain.PageNode{}
	var rowActions []domain.PageNode
	for _, ch := range n.Children {
		if ch.Cell > 0 {
			cellTemplates[ch.Cell] = ch
		} else {
			rowActions = append(rowActions, ch)
		}
	}
	if dimension && len(rowActions) > 0 {
		return nil, fmt.Errorf("%s: a Table of a Dimension's rows has no row actions -- a row is a group of records, not one", path)
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
	var hidden []string
	for i, c := range cols {
		where := fmt.Sprintf("%s.columns[%d]", path, i)
		switch {
		case strings.TrimSpace(c.Header) == "" || strings.ContainsAny(c.Header, "\n\r"):
			return nil, fmt.Errorf("%s: header: words on one line (a header drawn for no one to see is hide_header: true, not a blank one)", where)
		case c.Cell && (c.From != "" || c.Field != "" || c.Related != "" || c.Href != "" || c.Of != "" || c.Under != "" || c.Total != "" || c.TotalLabel != ""):
			return nil, fmt.Errorf("%s: a column with a cell: template shows what the template draws, so it takes no from:, field:, related:, href:, under:, of: or total:", where)
		case c.Cell && dimension:
			return nil, fmt.Errorf("%s: a Dimension's row is not a record, so a column of one takes no cell: template", where)
		case c.Empty != "" && !c.Cell:
			return nil, fmt.Errorf("%s: empty: is the words a cell: template says when it draws nothing, so it sits beside cell:", where)
		case c.Cell && cellTemplates[i+1].Kind == "":
			return nil, fmt.Errorf("%s: the cell: template was not carried with the Table's children", where)
		case c.Under != "" && (c.Field == "" || c.Of != ""):
			return nil, fmt.Errorf("%s: under: is a second line under a Field's value, so it sits beside field: and not of:", where)
		case !c.Cell && countNonEmpty(c.From, c.Field, c.Related) != 1:
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
		if c.HideHeader {
			hidden = append(hidden, strconv.Itoa(i))
		}
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
	if len(rowActions) > 0 {
		actions = &domain.PageNode{Kind: string(NodeLayout), Type: string(domain.LayoutRow),
			Props: map[string]string{"gap": string(domain.GapTight), "align": string(domain.RowAlignCenter), "justify": string(domain.RowJustifyStart)}, Children: rowActions}
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
	if len(hidden) > 0 {
		props[tableHiddenProp] = strings.Join(hidden, ",")
	}
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i, rec := range set.Records {
		for j, c := range cols {
			if c.Cell {
				cell, err := lowerCell(cellTemplates[j+1], c.Empty, r, fmt.Sprintf("%s.columns[%d].cell", path, j), set, i)
				if err != nil {
					return nil, err
				}
				out.Children = append(out.Children, cell)
				continue
			}
			key := c.From
			switch {
			case c.Field != "":
				key = domain.PageFieldRole(c.Field)
			case c.Related != "":
				if relation, field := domain.SplitRelatedColumn(c.Related); field != "" {
					key = domain.PageRelatedFieldRole(relation, field)
				} else {
					key = domain.PageRelatedRole(c.Related)
				}
			}
			text, ok := rec[key]
			if c.From == domain.PageRowPositionColumn {
				text, ok = strconv.Itoa(i+1), true
			}
			// A Dimension row's related-record Field may be unset (a category with no type yet): an empty cell.
			if !ok && !set.ShapeOnly && !(dimension && strings.HasPrefix(key, "row:")) {
				return nil, fmt.Errorf("%s.columns[%d]: the column shows %q, which the bound Dataset's Machine does not project (record %d)", path, j, key, i)
			}
			cell, err := tableCell(c, text, rec)
			if err == nil && c.Under != "" {
				cell = underCell(cell, rec[domain.PageFieldRole(c.Under)])
			}
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
	footer, err := tableFooter(cols, set, dimension, path, actions != nil, r.Number)
	if err != nil {
		return nil, err
	}
	if footer != "" {
		out.Props[tableFooterProp] = footer
	}
	return []UINode{out}, nil
}

// lowerCell draws one cell of a column that declares a `cell:` template: the template lowered for this record exactly as a
// Collection's item is, so everything a record template may hold (a Tag per label with `each: tags`, an Avatar, a StatusBadge
// toned by a derived role, the completion Toggle) reads the same grammar. A template that draws nothing for the record (a node
// reading a derived role the record has no value for is not drawn) leaves the column's `empty:` words, and one that draws several
// nodes (the Tags of a record with three labels) lays them in a row, so a cell is always one node.
func lowerCell(tmpl domain.PageNode, empty string, r Resolver, path string, set RecordSet, i int) (UINode, error) {
	items, err := lowerRecordItem(tmpl, r, path, set, i)
	if err != nil {
		return UINode{}, err
	}
	switch len(items) {
	case 0:
		return UINode{Kind: NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": empty}}, nil
	case 1:
		return items[0], nil
	}
	return UINode{Kind: NodeLayout, Type: string(domain.LayoutRow), Props: map[string]string{"gap": string(domain.GapTight), "align": string(domain.RowAlignCenter), "justify": string(domain.RowJustifyStart)}, Children: items}, nil
}

// tableFooterProp is a Table's total row, lowered from its columns' `total:` -- one tab-separated cell per column --
// and never written.
const tableFooterProp = "footer"

// Total keywords a column may declare: `sum` adds the column's figure over the rows shown (a fraction's numerator and
// its `of:` both), `count` says how many rows there are.
const (
	TotalSum   = "sum"
	TotalCount = "count"
)

// tableFooter is the total row, **computed from the rows the Table draws** (the review of step 4: a total read from a
// separate aggregate ignored the search and the chips, and a Dimension's total counted records no row showed), so it
// always adds up what is above it. `sum` adds the column's bare numbers, `count` counts the rows, `total_label:` words
// it ("9 contracts"; alone, "Total"). A list its Dataset's bound cut has no total row: it would add up only the rows
// shown, and the notice already says the list is cut.
func tableFooter(cols []domain.PageColumn, set RecordSet, dimension bool, path string, actions bool, nf domain.NumberFormat) (string, error) {
	asked := false
	for _, c := range cols {
		asked = asked || c.Total != "" || c.TotalLabel != ""
	}
	if !asked || (set.Truncated && !set.ShapeOnly) {
		return "", nil
	}
	number := func(c domain.PageColumn, id string) string {
		if id == "" {
			return ""
		}
		return domain.PageRowNumberRole(id)
	}
	sum := func(key string) float64 {
		var total float64
		for _, rec := range set.Records {
			if n, err := strconv.ParseFloat(rec[key], 64); err == nil {
				total += n
			}
		}
		return total
	}
	cells := make([]string, 0, len(cols)+1)
	for i, c := range cols {
		text := ""
		switch c.Total {
		case "":
		case TotalCount:
			text = strconv.Itoa(len(set.Records))
		case TotalSum:
			value := c.Field
			if dimension {
				value = strings.TrimPrefix(c.From, "measure:")
			}
			if value == "" || (!dimension && c.Field == "") || (dimension && !strings.HasPrefix(c.From, "measure:")) {
				return "", fmt.Errorf("%s.columns[%d]: total: sum adds a figure, so the column shows field: (or from: measure:<id> on a Dimension's rows)", path, i)
			}
			a := sum(number(c, value))
			text = formatTotal(a, set.Records, c, value, dimension, nf)
			if c.Of != "" {
				b := sum(number(c, c.Of))
				text = strconv.FormatFloat(a, 'f', -1, 64) + "/" + strconv.FormatFloat(b, 'f', -1, 64)
			}
		default:
			return "", fmt.Errorf("%s.columns[%d]: total: is %s or %s, not %q", path, i, TotalSum, TotalCount, c.Total)
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

// formatTotal writes a column's sum the way its cells are written, read from the rows' own display: a currency symbol
// before the figure ("Rp ") is kept, and a figure the rows write as money (a thousands mark, or two decimals after the
// Workspace's decimal mark) is written as money. ir sees only strings, so the cells are the one statement of how this
// column is written.
func formatTotal(total float64, rows []map[string]string, c domain.PageColumn, value string, dimension bool, nf domain.NumberFormat) string {
	nf = nf.Or()
	key := domain.PageFieldRole(value)
	if dimension {
		key = domain.PageMeasureRole(value)
	}
	prefix, money := "", false
	for _, rec := range rows {
		shown := strings.TrimPrefix(rec[key], "-")
		if shown == "" {
			continue
		}
		if i := strings.IndexAny(shown, "0123456789"); i > 0 {
			prefix = shown[:i]
		}
		money = money || prefix != "" || strings.Contains(shown, nf.Thousands) || moneyCents(shown, nf)
	}
	if !money {
		return strconv.FormatFloat(total, 'f', -1, 64)
	}
	out := domain.FormatMoney(total, nf)
	if rest, negative := strings.CutPrefix(out, "-"); negative {
		return "-" + prefix + rest
	}
	return prefix + out
}

// moneyCents says shown ends in the Workspace's decimal mark and exactly two digits: how money is written, where a plain
// number is written with as many decimals as it has.
func moneyCents(shown string, nf domain.NumberFormat) bool {
	i := strings.LastIndex(shown, nf.Decimal)
	return i >= 0 && len(shown)-i-len(nf.Decimal) == 2 && strings.Trim(shown[i+len(nf.Decimal):], "0123456789") == ""
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
// underCell is a cell with a second line under it: a stack of the cell and a caption, so one row reads as two lines in
// one column. Nothing under it leaves the cell as it was.
func underCell(cell UINode, under string) UINode {
	if under == "" {
		return cell
	}
	return UINode{Kind: NodeLayout, Type: string(domain.LayoutStack), Props: map[string]string{"gap": string(domain.GapMicro)},
		Children: []UINode{cell, {Kind: NodeStatic, Type: string(domain.StaticCaption), Props: map[string]string{"text": under}}}}
}

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
	case c.Related != "" && strings.Contains(c.Related, "."):
		// One Field of the related record, as words.
		return UINode{Kind: NodeStatic, Type: string(domain.StaticParagraph), Props: map[string]string{"text": text}}, nil
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
