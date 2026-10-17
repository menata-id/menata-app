package ir

import (
	"fmt"
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
	switch {
	case b.Measure != "" || b.Group != "":
		return nil, fmt.Errorf("%s: a Table lists records, so its binding takes no measure and no group", path)
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
		case (c.From == "") == (c.Field == ""):
			return nil, fmt.Errorf("%s: a column shows exactly one of from: (a Projection role) or field: (a Field id)", where)
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
	if r.Records == nil {
		return nil, fmt.Errorf("%s: no resolver for records", path)
	}
	set, err := r.Records(*b)
	if err != nil {
		return nil, err
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
	props := make(map[string]string, len(n.Props)+2)
	for k, v := range n.Props {
		props[k] = v
	}
	props[tableHeadersProp], props[tableAlignsProp] = strings.Join(headers, "\n"), strings.Join(aligns, ",")
	out := UINode{Kind: NodeKind(n.Kind), Type: n.Type, Props: props}
	for i, rec := range set.Records {
		for j, c := range cols {
			key := c.From
			if c.Field != "" {
				key = domain.PageFieldRole(c.Field)
			}
			text, ok := rec[key]
			if !ok {
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
	return []UINode{out}, nil
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
