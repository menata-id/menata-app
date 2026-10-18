package metadata

import (
	"fmt"
	"maps"
	"menata.app/internal/expression"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/ir"
	"menata.app/internal/registry"
)

// pageNodeDoc is one node of a navigation item's `page:` block.
//
// It is decoded by hand rather than into a struct, because the keys a node accepts depend on its type: a
// `grid` takes `columns`, a `Metric` takes `hint`. Putting every property of every type in one struct would
// make a typo in `columns` and a typo in `hnit` the same silent nothing; instead every non-reserved scalar
// becomes a property, and `ir.Validate`'s allowed-property table (007 §15.3's fourth rejection) refuses the
// ones the node's type does not declare. The strictness lives in one place, the one that already knows the
// vocabulary.
//
// The reserved keys are the three discriminators, `children`, `binding`, `from`, `to`, `param`, `list_of`, `count`, `hint_figure`, `when`, `each`, `step` and `window`. Exactly one discriminator per
// node: `layout:`, `static:` or `component:`, whose value is the type.
type pageNodeDoc struct {
	kind     string
	typ      string
	props    map[string]string
	binding  *pageBindingDoc
	from     map[string]string
	to       string
	listOf   string
	param    string
	count    *domain.PageCount
	hint     *domain.PageCount
	when     *domain.PageCondition
	each     string
	step     *domain.PageStep
	window   string
	columns  []domain.PageColumn
	pass     map[string]string
	values   map[string]string
	children []pageNodeDoc
}

type pageBindingDoc struct {
	Dataset string
	Measure string
	Rows    string
	Write   string
	Group   string
	All     bool
	Action  string
	// Transition: see domain.PageBinding.Transition.
	Transition string
}

var pageDiscriminators = []string{"layout", "static", "component"}

func (p *pageNodeDoc) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a page node is a mapping with one of layout:, static: or component:", n.Line)
	}
	p.props = map[string]string{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		switch key.Value {
		case "layout", "static", "component":
			if val.Kind != yaml.ScalarNode || val.Value == "" {
				return fmt.Errorf("line %d: %s: must name a type", key.Line, key.Value)
			}
			if p.kind != "" {
				return fmt.Errorf("line %d: a page node is exactly one of layout:, static: or component:, not %s: as well as %s:", key.Line, key.Value, p.kind)
			}
			p.kind, p.typ = key.Value, val.Value
		case "children":
			if val.Kind != yaml.SequenceNode {
				return fmt.Errorf("line %d: children: must be a list of page nodes", key.Line)
			}
			if err := val.Decode(&p.children); err != nil {
				return err
			}
		case "from":
			f, err := decodePageFrom(val)
			if err != nil {
				return err
			}
			p.from = f
		case "to":
			if val.Kind != yaml.ScalarNode || val.Value == "" {
				return fmt.Errorf("line %d: to: names one navigation item id, such as nav_approval_inbox -- never a route", key.Line)
			}
			p.to = val.Value
		case "param":
			if val.Kind != yaml.ScalarNode || val.Value == "" {
				return fmt.Errorf("line %d: param: names one query parameter, such as status -- the name a destination page's Dataset reads as $parameters.<name>", key.Line)
			}
			p.param = val.Value
		case "list_of":
			if val.Kind != yaml.ScalarNode || val.Value == "" {
				return fmt.Errorf("line %d: list_of: names one dataset id, such as ds_recent_documents -- never a route", key.Line)
			}
			p.listOf = val.Value
		case "count":
			c, err := decodePageCount(val, "count")
			if err != nil {
				return err
			}
			p.count = c
		case "hint_figure":
			c, err := decodePageCount(val, "hint_figure")
			if err != nil {
				return err
			}
			p.hint = c
		case "each":
			if val.Kind != yaml.ScalarNode || val.Value == "" {
				return fmt.Errorf("line %d: each: names the list a record owns, which is %s", key.Line, domain.PageEachTags)
			}
			p.each = val.Value
		case "step":
			st, err := decodePageStep(val)
			if err != nil {
				return err
			}
			p.step = st
		case "window":
			if val.Kind != yaml.ScalarNode || val.Value == "" {
				return fmt.Errorf("line %d: window: names one dataset id, such as ds_tasks_due_week -- the Dataset whose date window the text says", key.Line)
			}
			p.window = val.Value
		case "columns":
			// A Grid layout's `columns: 4` is a scalar property; a Table's `columns:` is a list. The shape says which.
			if val.Kind != yaml.SequenceNode {
				if val.Kind != yaml.ScalarNode {
					return fmt.Errorf("line %d: columns: is a number on a grid or a list of columns on a Table", key.Line)
				}
				if _, dup := p.props[key.Value]; dup {
					return fmt.Errorf("line %d: %s: declared more than once", key.Line, key.Value)
				}
				p.props[key.Value] = val.Value
				break
			}
			cols, err := decodePageColumns(val)
			if err != nil {
				return err
			}
			p.columns = cols
		case "pass", "values":
			m, err := decodePageFrom(val)
			if err != nil {
				return fmt.Errorf("line %d: %s: is a mapping of a name to one value -- %w", key.Line, key.Value, err)
			}
			if key.Value == "pass" {
				p.pass = m
			} else {
				p.values = m
			}
		case "when":
			w, err := decodePageWhen(val)
			if err != nil {
				return err
			}
			p.when = w
		case "binding":
			b, err := decodePageBinding(val)
			if err != nil {
				return err
			}
			p.binding = b
		default:
			if val.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: %s: a page property is a single value, not a list or mapping", key.Line, key.Value)
			}
			if _, dup := p.props[key.Value]; dup {
				return fmt.Errorf("line %d: %s: declared more than once", key.Line, key.Value)
			}
			p.props[key.Value] = val.Value
		}
	}
	if p.kind == "" {
		return fmt.Errorf("line %d: a page node needs exactly one of %v", n.Line, pageDiscriminators)
	}
	return nil
}

// decodePageColumns reads a Table's `columns:`: a list of `{header, from, align, href}`, every value a scalar. Whether
// the role exists and the Table is bound is `ir.Lower`'s and `bindingIssues`' question, which have the tree and the Machine.
func decodePageColumns(n *yaml.Node) ([]domain.PageColumn, error) {
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		return nil, fmt.Errorf("line %d: columns: is a list of {header, from} entries, such as [{header: Title, from: title}]", n.Line)
	}
	var out []domain.PageColumn
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d: a column is a mapping of header and from, and optionally align and href", item.Line)
		}
		var c domain.PageColumn
		for i := 0; i+1 < len(item.Content); i += 2 {
			key, val := item.Content[i], item.Content[i+1]
			if val.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: columns.%s: a single value, not a list or mapping", key.Line, key.Value)
			}
			switch key.Value {
			case "header":
				c.Header = val.Value
			case "from":
				c.From = val.Value
			case "field":
				c.Field = val.Value
			case "align":
				c.Align = val.Value
			case "href":
				c.Href = val.Value
			default:
				return nil, fmt.Errorf("line %d: %q is not a key a column declares (header, from or field, align, href)", key.Line, key.Value)
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// decodePageFrom reads `from: {text: title}`: a property of this node, and the Projection role it takes from
// the record. Only scalars on both sides -- the same "no grammar" rule a binding follows (007 §9.2).
func decodePageFrom(n *yaml.Node) (map[string]string, error) {
	if n.Kind != yaml.MappingNode || len(n.Content) == 0 {
		return nil, fmt.Errorf("line %d: from: is a mapping of a property to the Projection role it takes, such as {text: title}", n.Line)
	}
	out := map[string]string{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if val.Kind != yaml.ScalarNode || val.Value == "" {
			return nil, fmt.Errorf("line %d: from.%s: names one Projection role, not an expression or a list", key.Line, key.Value)
		}
		if _, dup := out[key.Value]; dup {
			return nil, fmt.Errorf("line %d: from.%s: declared more than once", key.Line, key.Value)
		}
		out[key.Value] = val.Value
	}
	return out, nil
}

// decodePageCount reads `count: {of, one, other}`. The wording is checked in `ir.Lower` where the record is
// known; here only the shape of the block is.
func decodePageCount(n *yaml.Node, name string) (*domain.PageCount, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: %s: is a mapping of of, one and other", n.Line, name)
	}
	c := &domain.PageCount{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if val.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: %s.%s: a single value, not an expression or a list", key.Line, name, key.Value)
		}
		switch key.Value {
		case "of":
			c.Of = val.Value
		case "one":
			c.One = val.Value
		case "other":
			c.Other = val.Value
		default:
			return nil, fmt.Errorf("line %d: %q is not a key %s declares (of, one, other)", key.Line, key.Value, name)
		}
	}
	return c, nil
}

// decodePageStep reads `step: {parameter: week, by: 1}`: the parameter and a whole number of steps, both scalars.
func decodePageStep(n *yaml.Node) (*domain.PageStep, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: step: is a mapping of parameter and by", n.Line)
	}
	st := &domain.PageStep{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if val.Kind != yaml.ScalarNode || val.Value == "" {
			return nil, fmt.Errorf("line %d: step.%s: a single value, not an expression or a list", key.Line, key.Value)
		}
		switch key.Value {
		case "parameter":
			st.Parameter = val.Value
		case "by":
			by, err := strconv.Atoi(val.Value)
			if err != nil {
				return nil, fmt.Errorf("line %d: step.by: %q is not a whole number", key.Line, val.Value)
			}
			if by > expression.MaxWeekOffset || by < -expression.MaxWeekOffset {
				return nil, fmt.Errorf("line %d: step.by: %d is beyond the %d weeks either way a request can be moved", key.Line, by, expression.MaxWeekOffset)
			}
			st.By = by
		default:
			return nil, fmt.Errorf("line %d: %q is not a key step declares (parameter, by)", key.Line, key.Value)
		}
	}
	if st.Parameter == "" || st.By == 0 {
		return nil, fmt.Errorf("line %d: step: names a parameter and a by: that is not zero", n.Line)
	}
	return st, nil
}

// decodePageWhen reads `when: {role: status, is_not: $done}`: one role and exactly one comparison, both scalars.
func decodePageWhen(n *yaml.Node) (*domain.PageCondition, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: when: is a mapping of role and one of is / is_not", n.Line)
	}
	c := &domain.PageCondition{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if val.Kind != yaml.ScalarNode || val.Value == "" {
			return nil, fmt.Errorf("line %d: when.%s: a single value, not an expression or a list -- a condition has no grammar (007 §9.2)", key.Line, key.Value)
		}
		switch key.Value {
		case "role":
			c.Role = val.Value
		case "parameter":
			c.Parameter = val.Value
		case "viewer":
			c.Viewer = val.Value
		case "is":
			c.Is = val.Value
		case "is_not":
			c.IsNot = val.Value
		default:
			return nil, fmt.Errorf("line %d: %q is not a key when declares (role, parameter or viewer, is, is_not)", key.Line, key.Value)
		}
	}
	named := 0
	for _, v := range []string{c.Role, c.Parameter, c.Viewer} {
		if v != "" {
			named++
		}
	}
	if named != 1 || (c.Is == "") == (c.IsNot == "") {
		return nil, fmt.Errorf("line %d: when: names a role, a parameter or a viewer fact (not both of any two) and exactly one of is: and is_not:", n.Line)
	}
	if c.Parameter != "" {
		operand := c.Is + c.IsNot
		if weeks, err := strconv.Atoi(operand); err == nil && (weeks > expression.MaxWeekOffset || weeks < -expression.MaxWeekOffset) {
			return nil, fmt.Errorf("line %d: when: %s %s is beyond the %d weeks either way a request is clamped to, so the comparison could never (or would always) hold", n.Line, c.Parameter, operand, expression.MaxWeekOffset)
		}
	}
	return c, nil
}

func decodePageBinding(n *yaml.Node) (*pageBindingDoc, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: binding: is a mapping of dataset, measure, rows, write and group", n.Line)
	}
	b := &pageBindingDoc{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if val.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: binding.%s: a single value, not an expression or a list -- a binding names a dataset, a measure and a mode, and has no grammar (007 §9.2)", key.Line, key.Value)
		}
		switch key.Value {
		case "dataset":
			b.Dataset = val.Value
		case "measure":
			b.Measure = val.Value
		case "rows":
			b.Rows = val.Value
		case "write":
			b.Write = val.Value
		case "group":
			b.Group = val.Value
		case "action":
			b.Action = val.Value
		case "transition":
			b.Transition = val.Value
		case "all":
			if val.Value != "true" && val.Value != "false" {
				return nil, fmt.Errorf("line %d: binding.all: %q is not true or false", key.Line, val.Value)
			}
			b.All = val.Value == "true"
		default:
			return nil, fmt.Errorf("line %d: %q is not a key a binding declares (dataset, measure, rows, write, group, all, action, transition)", key.Line, key.Value)
		}
	}
	return b, nil
}

func (p pageNodeDoc) toDomain() domain.PageNode {
	out := domain.PageNode{Kind: p.kind, Type: p.typ, Props: p.props, To: p.to, ListOf: p.listOf, Param: p.param, Count: p.count, HintFigure: p.hint, When: p.when, Each: p.each, Step: p.step, Window: p.window, Columns: p.columns, Pass: p.pass, Values: p.values}
	if len(out.Props) == 0 {
		out.Props = nil
	}
	if p.binding != nil {
		out.Binding = &domain.PageBinding{Dataset: p.binding.Dataset, Measure: p.binding.Measure, Rows: p.binding.Rows, Write: p.binding.Write, Group: p.binding.Group, All: p.binding.All, Action: p.binding.Action, Transition: p.binding.Transition}
		for _, c := range p.columns {
			if c.Field != "" && !slices.Contains(out.Binding.Fields, c.Field) {
				out.Binding.Fields = append(out.Binding.Fields, c.Field)
			}
		}
		out.Binding.CountCut = p.typ == string(domain.ComponentTable) && p.props["complete"] == "true"
		out.Binding.Values = p.values
		// A records template that reads a Field by id (`from: {text: "field:fld_x"}`, its `tone:` or its `steps:`)
		// names it to the Data Plane the same way a Table's column does.
		for _, id := range templateFieldRoles(p.children) {
			if !slices.Contains(out.Binding.Fields, id) {
				out.Binding.Fields = append(out.Binding.Fields, id)
			}
		}
	}
	if len(p.from) > 0 {
		out.From = p.from
	}
	for _, c := range p.children {
		out.Children = append(out.Children, c.toDomain())
	}
	return out
}

// PlaceholderResolver stands in for the database when only a tree's *shape* is being checked: one dimension
// row, and one record that projects every role in `domain.KnownCardFieldRoles` (each "x", except `color`,
// which is a palette entry). A `from:` naming anything
// outside that vocabulary therefore fails to lower, which is how a typo in a role is a load error. Exported so
// the conformance sweep over installed manifests asks the same question the loader does.
//
// Routes are **not** a placeholder: a `to:` is resolved against the Application's real navigation, because
// whether that item exists is exactly what is being checked, and no database is needed to know.
func PlaceholderResolver(navigation []domain.NavigationItem, datasets map[string]domain.Dataset) ir.Resolver {
	return ir.Resolver{
		Route: ir.NavigationRoutes(navigation),
		// Any dataset id resolves here: whether this Workspace declares it is `bindingIssues`' question, which
		// has the Workspace's Datasets, and this resolver only needs a Machine to name.
		Source: func(string) (string, bool) { return "machine", true },
		// Every Measure the bound Dataset declares, so a dimension row's template reading `measure:<id>` lowers
		// exactly when the id is one of them -- a mistyped Measure fails as a mistyped role does.
		Rows: func(b domain.PageBinding) ([]ir.Row, error) {
			measures := map[string]string{}
			for _, m := range datasets[b.Dataset].Measures {
				measures[m.ID] = "0"
			}
			return []ir.Row{{Label: "label", Value: "0", Measures: measures}}, nil
		},
		// A non-zero figure, so a `tone:` on a total is checked rather than dropped as a zero's would be.
		Total: func(domain.PageBinding) (string, error) { return "1", nil },
		// A window is words: any Dataset id answers, whether the Workspace declares it and can read a window out of
		// it is `windowIssues`' question, which has the Datasets.
		Window: func(string) (string, error) { return "x", nil },
		Step:   func(string, int) string { return "1" },
		// A form's shape is checked with one text control: whether the real Machine can be written by a form at
		// all is `bindingIssues`' question, which has the Machine and this resolver does not.
		Form: func(b domain.PageBinding) (ir.FormSpec, error) {
			return ir.FormSpec{
				Route: domain.FormRoute("machine"), Permitted: true,
				Inputs: []domain.FormInput{{FieldID: "x", Label: "x", Kind: domain.InputText}},
			}, nil
		},
		// A bulk transition is drawn when there is something to move, so the placeholder says there is, which keeps the
		// Button and its properties validated.
		Query: func(string) string { return "" },
		Navigation: func(section string) (ir.NavSection, error) {
			return ir.NavSection{Label: "x", Home: "/x", Entries: []ir.NavEntry{{ID: "x", Group: "x", Label: "x", Href: "/x"}}}, nil
		},
		Action: func(b domain.PageBinding, _ string) (ir.FormSpec, error) {
			a := registry.RuntimeActions[b.Action]
			spec := ir.FormSpec{Route: domain.RuntimeActionRoute(b.Action, ""), Permitted: true}
			for _, in := range a.Inputs {
				spec.Inputs = append(spec.Inputs, domain.FormInput{FieldID: in.Name, Label: in.Label, Kind: domain.InputText, Required: in.Required})
			}
			return spec, nil
		},
		TransitionAll: func(b domain.PageBinding, becomes string) (ir.AllTransition, error) {
			return ir.AllTransition{Route: domain.TransitionAllRoute("machine", b.Dataset, "x"), Count: 1}, nil
		},
		Records: func(b domain.PageBinding) (ir.RecordSet, error) {
			rec := map[string]string{domain.PageRecordRole: "/x"}
			for role := range domain.KnownCardFieldRoles {
				rec[string(role)] = "x"
			}
			// The one role whose contract is a closed set: a record's `color` is a palette entry by
			// definition, so a Tag filled from it must validate. Every other role stays "x", which is how
			// `from: {color: title}` is a load error -- a title is not a palette entry.
			rec[string(domain.CardFieldRoleColor)] = string(domain.TagSlate)
			// The derived date roles: a label, and a tone out of the closed set a StatusBadge validates against.
			rec[domain.PageDateLabelRole], rec[domain.PageDateToneRole] = "x", string(domain.ToneNeutral)
			rec[domain.PageGroupLabelRole] = "x"
			rec[domain.PageCreatedLabelRole] = "x"
			rec[domain.PageStatusToneRole] = string(domain.ToneNeutral)
			// The Fields a Table names by id: whether each is a Field of the real Machine is `recordsBindingIssues`'.
			for _, id := range b.Fields {
				rec[domain.PageFieldRole(id)] = "x"
				rec[domain.PageFieldToneRole(id)] = string(domain.ToneNeutral)
				rec[domain.PageFieldStepsRole(id)] = "x\tcurrent"
			}
			rec[domain.PageRecordIDRole] = "x"
			rec[string(domain.CardFieldRoleLink)] = "/x" // a destination the runtime would hand a page, so a link taking its href from it validates
			// A count exists exactly for the Relations the bound Dataset declares, so a `count: {of: ...}`
			// naming any other fails to lower -- the same way a mistyped role does.
			for _, rel := range datasets[b.Dataset].Relations {
				rec[domain.PageCountRole(rel.ID)] = "0"
			}
			// One edit form, text-shaped, for the same reason the create form's is: whether the real Machine
			// has anything to edit is `updateBindingIssues`' question, which has the Machine.
			edit := ir.FormSpec{
				Route: domain.FormRoute("machine") + "/x", Method: domain.FormMethodPatch, Permitted: true,
				Inputs: []domain.FormInput{{FieldID: "x", Label: "x", Kind: domain.InputText}},
			}
			// One delete the viewer may do, for the same reason: whether the real Machine can be deleted from is
			// `deleteBindingIssues`' question.
			del := ir.RecordAction{Route: domain.FormRoute("machine") + "/x", Permitted: true}
			mv := ir.RecordMove{Route: domain.FormRoute("machine") + "/x", Up: true, Down: true}
			// A transition that is allowed to anywhere, with the two sentinels answering: whether the real
			// Machine has a status Field, that option, and a `completion:` is `transitionBindingIssues`' question.
			tr := ir.RecordTransition{
				Route: domain.FormRoute("machine") + "/x", Field: "x", Done: "x", Reopen: "x",
				Allows: func(string) (bool, bool) { return true, true },
			}
			// ShapeOnly: every `when:` holds here, so the nodes a real record would hide are still checked. Whether
			// the role and the operand are right for the real Machine is `conditionIssues`' question.
			// One tag, a palette entry for the same reason `color` above is one: an `each: tags` node is checked as a
			// Tag would be drawn. Whether the real Machine declares `card_tags:` is `bindingIssues`' question.
			tags := []map[string]string{{string(domain.CardFieldRoleTitle): "x", string(domain.CardFieldRoleColor): string(domain.TagSlate)}}
			set := ir.RecordSet{Records: []map[string]string{rec}, Tags: [][]map[string]string{tags}, Edits: []ir.FormSpec{edit}, Deletes: []ir.RecordAction{del}, Moves: []ir.RecordMove{mv}, Transitions: []ir.RecordTransition{tr}, Done: "x", Reopen: "x", ShapeOnly: true}
			if b.Group != "" {
				// One bucket holding the one record, so both templates of a grouped Collection are lowered and checked.
				set.Buckets, set.BucketKeys = []ir.Bucket{{Key: "x", Label: "x"}}, []string{"x"}
			}
			return set, nil
		},
	}
}

var registerIRVocabulary sync.Once

// ensureIRVocabulary hands `ir` the Component catalogue it may not import. `cmd/server` does this at startup;
// the loader does it too because it is the second place `ir.Validate` runs, and a validator whose vocabulary
// depends on who called first would reject every Component in a test and accept them in production.
// Idempotent: the registration is a set.
func ensureIRVocabulary() {
	registerIRVocabulary.Do(func() {
		ir.RegisterComponentTypes(registry.ComponentTypeNames())
		ir.RegisterSlottedComponentTypes(registry.SlottedComponentTypeNames())
	})
}

// validatePages checks every declared `page:` in a Workspace, after the Machines are loaded because a Binding
// is checked against the Datasets those Machines declare.
//
// What is refused here, and why it is a load error rather than a request-time one (the reason
// `validateWorkflowDatasets` states): a Page whose Dataset is missing would answer 500 on every visit, and
// nothing else would be red.
//
//   - the route is not `/pages/<id>` -- the one generic handler could not find the item;
//   - the root is not a Layout -- a body is composed, and a root Binding would expand to several roots;
//   - a Binding names no Dataset in this Workspace, no such Measure, a Dataset with no Dimension, or a
//     `select: records` Dataset (§15.3's fourth rejection: a binding outside the permitted scope);
//   - the lowered tree fails `ir.Validate` (cycle, unresolved child, slot mismatch, unknown or undeclared
//     property, depth) or a Component's own registered validator.
//
// The tree is checked with a **placeholder row** standing in for each Binding, so shape, properties and
// Component inputs are verified without a database: a bound Metric is validated as a Metric with a label and
// a value.
func validatePages(applications []domain.Application, machines []*domain.Machine, workspaceNav, runtimePages []domain.NavigationItem, runtimeMachines []*domain.Machine) error {
	datasets := map[string]domain.Dataset{}
	byID := map[string]*domain.Machine{}
	runtimeDatasets := map[string]bool{}
	for _, m := range runtimeMachines {
		byID[m.ID] = m
		for _, ds := range m.Datasets {
			datasets[ds.ID] = ds
			runtimeDatasets[ds.ID] = true
		}
	}
	var issues []string
	for _, m := range machines {
		if _, taken := byID[m.ID]; taken {
			issues = append(issues, fmt.Sprintf("machine %q: this id is one the runtime's own machines use", m.ID))
		}
		byID[m.ID] = m
		for _, ds := range m.Datasets {
			if runtimeDatasets[ds.ID] {
				issues = append(issues, fmt.Sprintf("machine %q: dataset %q: this id is one the runtime's own machines use", m.ID, ds.ID))
			}
			datasets[ds.ID] = ds
		}
	}
	// What the runtime provides (its members, its groups) is read by the runtime's own pages and by no other: a
	// Workspace's or an Application's page binding one would put the Workspace's people on a screen whose gate is
	// not the one that was meant for them.
	readsRuntime := func(item domain.NavigationItem, where string) {
		if usesAction(*item.Page) {
			issues = append(issues, fmt.Sprintf("%s: page: write: %s performs an operation of the runtime and is for the runtime's own pages only", where, domain.PageWriteAction))
		}
		for _, id := range boundDatasets(*item.Page) {
			if runtimeDatasets[id] {
				issues = append(issues, fmt.Sprintf("%s: page: dataset %q is provided by the runtime and is read by the runtime's own pages only", where, id))
			}
		}
	}
	for _, app := range applications {
		for _, item := range app.AllNavigation {
			if item.OpenTo != "" {
				issues = append(issues, fmt.Sprintf("application %q: navigation item %q: open_to: belongs to the Workspace's own pages; an Application's page is opened by its roles", app.ID, item.ID))
			}
			if item.Page == nil {
				continue
			}
			where := fmt.Sprintf("application %q: navigation item %q", app.ID, item.ID)
			readsRuntime(item, where)
			if want := "/pages/" + item.ID; item.Route != want {
				issues = append(issues, fmt.Sprintf("%s: page: route is %q but a page is rendered at %q", where, item.Route, want))
			}
			if item.Page.Kind != "layout" {
				issues = append(issues, fmt.Sprintf("%s: page: the root must be a layout, not %s %q", where, item.Page.Kind, item.Page.Type))
				continue
			}
			issues = append(issues, bindingIssues(*item.Page, datasets, byID, nil, where)...)
			issues = append(issues, shapeIssues(*item.Page, app.LinkTargets(workspaceNav), datasets, where)...)
			issues = append(issues, paramLinkIssues(*item.Page, app.LinkTargets(workspaceNav), datasets, where)...)
			issues = append(issues, stepIssues(*item.Page, app.LinkTargets(workspaceNav), datasets, where)...)
		}
	}
	// The Workspace's own pages (`navigation:` on the manifest) are checked the same way. What differs is what a link on
	// one may name: no Application owns the page, so `to:` resolves against the whole Workspace -- its own pages, the
	// runtime's screens and every installed Application's navigation.
	links := domain.Workspace{Navigation: workspaceNav, Applications: applications}.LinkTargets()
	// The runtime's own pages are checked as the Workspace's are, against this Workspace's Datasets: a Workspace that
	// cannot supply what a runtime page binds is refused at load, like any other. Their route is the screen's own.
	for _, item := range append(slices.Clone(workspaceNav), runtimePages...) {
		if item.OpenTo != "" && (item.Page == nil || !domain.KnownOpenTo[item.OpenTo]) {
			issues = append(issues, fmt.Sprintf("workspace navigation item %q: open_to: %q must be admin or member, on an item that declares a page", item.ID, item.OpenTo))
		}
		if item.Page == nil {
			continue
		}
		where := fmt.Sprintf("workspace navigation item %q", item.ID)
		if !domain.IsRuntimeScreenID(item.ID) {
			readsRuntime(item, where)
		}
		if want := "/pages/" + item.ID; item.Route != want && !domain.IsRuntimeScreenID(item.ID) {
			issues = append(issues, fmt.Sprintf("%s: page: route is %q but a page is rendered at %q", where, item.Route, want))
		}
		if item.Page.Kind != "layout" {
			issues = append(issues, fmt.Sprintf("%s: page: the root must be a layout, not %s %q", where, item.Page.Kind, item.Page.Type))
			continue
		}
		issues = append(issues, bindingIssues(*item.Page, datasets, byID, nil, where)...)
		issues = append(issues, shapeIssues(*item.Page, links, datasets, where)...)
		issues = append(issues, paramLinkIssues(*item.Page, links, datasets, where)...)
		issues = append(issues, stepIssues(*item.Page, links, datasets, where)...)
	}
	if len(issues) > 0 {
		sort.Strings(issues)
		issues = slices.Compact(issues)
		return &ValidationError{Issues: issues}
	}
	return nil
}

// bindingIssues walks a page for what its Bindings ask of the Workspace. `item` is the Machine of the records
// Collection the walk is inside (nil outside one): the only place an `update` Binding is valid, since it has no
// dataset of its own and edits that Collection's current record.
func bindingIssues(n domain.PageNode, datasets map[string]domain.Dataset, machines map[string]*domain.Machine, item *domain.Machine, where string) []string {
	var issues []string
	childItem := item
	if b := n.Binding; b != nil && b.Write == domain.PageWriteUpdate {
		issues = append(issues, updateBindingIssues(*b, item, where)...)
	} else if b != nil && b.Write == domain.PageWriteDelete {
		issues = append(issues, deleteBindingIssues(*b, item, where)...)
	} else if b != nil && b.Write == domain.PageWriteMove {
		issues = append(issues, moveBindingIssues(*b, item, where)...)
	} else if b != nil && b.Write == domain.PageWriteAction {
		issues = append(issues, actionBindingIssues(n, item, where)...)
	} else if b != nil && b.Write == domain.PageWriteTransition && b.All {
		issues = append(issues, transitionAllIssues(n, datasets, machines, item, where)...)
	} else if b != nil && b.Write == domain.PageWriteTransition {
		issues = append(issues, transitionBindingIssues(n, item, where)...)
	} else if b != nil {
		ds, ok := datasets[b.Dataset]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("%s: page: binding names dataset %q, which no machine in this workspace declares", where, b.Dataset))
		case b.Write != "":
			issues = append(issues, writeBindingIssues(*b, ds, machines[ds.Source], where)...)
		case b.Rows == domain.PageRowsRecords:
			issues = append(issues, recordsBindingIssues(n, ds, machines[ds.Source], where)...)
			childItem = machines[ds.Source]
		case ds.Select != "":
			issues = append(issues, fmt.Sprintf("%s: page: binding dataset %q selects records, and a dimension binding needs an aggregate dataset with measures (use rows: %s on a Collection to list records)", where, b.Dataset, domain.PageRowsRecords))
		default:
			// A Collection bound to dimension rows names its Measures in its template (`measure:<id>`), which the
			// placeholder rows check; every other bound node names its one Measure here.
			dimensionCollection := b.Rows == domain.PageRowsDimension && n.Type == string(domain.ComponentCollection)
			if !dimensionCollection && !slicesHasMeasure(ds, b.Measure) {
				issues = append(issues, fmt.Sprintf("%s: page: dataset %q declares no measure %q", where, b.Dataset, b.Measure))
			}
			if b.Rows == domain.PageRowsDimension && ds.Dimension == "" {
				issues = append(issues, fmt.Sprintf("%s: page: dataset %q declares no dimension, so it has no rows to expand", where, b.Dataset))
			}
			// The load-time total answers any id, so whether hint_figure's Measure exists is asked here.
			if h := n.HintFigure; h != nil && !slicesHasMeasure(ds, h.Of) {
				issues = append(issues, fmt.Sprintf("%s: page: hint_figure names measure %q, which dataset %q does not declare", where, h.Of, b.Dataset))
			}
		}
	}
	if n.When != nil && n.When.Parameter == "" && n.When.Viewer == "" {
		issues = append(issues, conditionIssues(*n.When, item, where)...)
	}
	if n.Window != "" {
		issues = append(issues, windowIssues(n.Window, datasets, machines, where)...)
	}
	if n.Each != "" {
		switch {
		case n.Each != domain.PageEachTags:
			issues = append(issues, fmt.Sprintf("%s: page: each: %q is not a list a record owns (%s)", where, n.Each, domain.PageEachTags))
		case item == nil:
			issues = append(issues, fmt.Sprintf("%s: page: each: %s is valid only inside the item template of a Collection bound with rows: records", where, n.Each))
		case item.CardTags == nil:
			issues = append(issues, fmt.Sprintf("%s: page: each: %s: machine %q declares no card_tags:, so its records have no tags to draw", where, n.Each, item.ID))
		default:
			for _, role := range sortedKeys(rolesOf(n.From)) {
				if role != string(domain.CardFieldRoleTitle) && role != string(domain.CardFieldRoleColor) {
					issues = append(issues, fmt.Sprintf("%s: page: each: %s: from: %s -- a tag has the roles %s and %s", where, n.Each, role, domain.CardFieldRoleTitle, domain.CardFieldRoleColor))
				}
			}
		}
	}
	if n.ListOf != "" {
		if _, ok := datasets[n.ListOf]; !ok {
			issues = append(issues, fmt.Sprintf("%s: page: list_of names dataset %q, which no machine in this workspace declares", where, n.ListOf))
		}
	}
	for _, c := range n.Children {
		issues = append(issues, bindingIssues(c, datasets, machines, childItem, where)...)
	}
	return issues
}

// updateBindingIssues is what a `write: update` binding asks: to sit inside a records Collection, to name no
// dataset of its own, and that the Machine of the records has something an edit form can ask for.
func updateBindingIssues(b domain.PageBinding, item *domain.Machine, where string) []string {
	switch {
	case item == nil:
		return []string{fmt.Sprintf("%s: page: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", where, domain.PageWriteUpdate, domain.PageRowsRecords)}
	case b.Dataset != "":
		return []string{fmt.Sprintf("%s: page: write: %s takes no dataset -- it edits the record of the Collection it sits in, and %q would be a second one (a join)", where, domain.PageWriteUpdate, b.Dataset)}
	case len(item.EditFormInputs(nil)) == 0:
		return []string{fmt.Sprintf("%s: page: write: %s: machine %q has no Field an edit form can ask for (booleans, statuses, references, groups, files, computed and stamped Fields are not drawn)", where, domain.PageWriteUpdate, item.ID)}
	}
	return nil
}

// deleteBindingIssues is what a `write: delete` binding asks: to sit inside a records Collection, to name no
// dataset of its own, and a Machine whose records can be deleted at all. An append-only Machine refuses every
// write, so a Button for it would be drawn for no viewer: a load error is the honest answer, as it is for a
// form with nothing to ask.
func deleteBindingIssues(b domain.PageBinding, item *domain.Machine, where string) []string {
	switch {
	case item == nil:
		return []string{fmt.Sprintf("%s: page: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", where, domain.PageWriteDelete, domain.PageRowsRecords)}
	case b.Dataset != "":
		return []string{fmt.Sprintf("%s: page: write: %s takes no dataset -- it deletes the record of the Collection it sits in, and %q would be a second one (a join)", where, domain.PageWriteDelete, b.Dataset)}
	case item.AppendOnly:
		return []string{fmt.Sprintf("%s: page: write: %s: machine %q is append-only, so no record of it can be deleted", where, domain.PageWriteDelete, item.ID)}
	}
	return nil
}

// writeBindingIssues is what a `write: create` binding asks of its Dataset's Machine: that it exists, and that a
// form can be built for it. The Dataset is only the *name* of the Machine here (a form reads nothing), and a
// Field a form cannot ask for (a reference, a group) is fine when it is optional but a load error when it is
// required -- a form that can never be submitted validly is refused when the manifest loads, not when someone
// presses the button.
func writeBindingIssues(b domain.PageBinding, ds domain.Dataset, source *domain.Machine, where string) []string {
	if source == nil {
		return []string{fmt.Sprintf("%s: page: write: dataset %q names machine %q, which this workspace does not install", where, ds.ID, ds.Source)}
	}
	var issues []string
	inputs, unsupported := source.CreateFormInputs()
	if len(inputs) == 0 {
		issues = append(issues, fmt.Sprintf("%s: page: write: machine %q has no Field a form can ask for", where, source.ID))
	}
	for _, f := range unsupported {
		if f.Required {
			issues = append(issues, fmt.Sprintf("%s: page: write: machine %q requires field %q, which is a %s -- a page form cannot ask for it yet, so no form could be submitted", where, source.ID, f.ID, f.Type))
		}
	}
	asked := map[string]bool{}
	for _, in := range inputs {
		asked[in.FieldID] = true
	}
	for _, field := range slices.Sorted(maps.Keys(b.Values)) {
		v := b.Values[field]
		switch {
		case !asked[field]:
			issues = append(issues, fmt.Sprintf("%s: page: values: %s is not a Field this form asks for", where, field))
		case !expression.IsParameter(v):
			issues = append(issues, fmt.Sprintf("%s: page: values: %s takes its value from the request ($parameters.<name>), not %q -- a fixed value is the Field's default:", where, field, v))
		}
	}
	return issues
}

// recordsBindingIssues is what a records binding asks of its Dataset and of the Machine it reads.
//
//   - the Dataset **selects records** -- an aggregate has no rows to list;
//   - it may filter on `$current_user` (the viewer) and on `$parameters.<name>` (a query value of the page's
//     own request); a request that sends no such value lists nothing, which is 007 §9.2's fail closed;
//   - every `from:` role in the item template is one the Dataset's Machine declares in `card_fields`, and the
//     Field behind it **is not a reference to a Machine other than the runtime's own people**, with one
//     exception. A reference Field resolves through `rendering.RelationOptions`, which is a read of the whole
//     related Machine (007 §20: no plane may read everything and trim later), so it is refused here. The two
//     references that need no such read are a `person` Field (the Workspace's members are answered by
//     `composition.Loader.PersonNames`, memoized per request and not growing with the records listed) and a
//     `container` Field (`composition.Loader.RelatedLabels` reads only the records the page lists name, in one
//     statement by id).
func recordsBindingIssues(n domain.PageNode, ds domain.Dataset, source *domain.Machine, where string) []string {
	var issues []string
	if ds.Select != domain.SelectRecords {
		issues = append(issues, fmt.Sprintf("%s: page: rows: %s needs a dataset that selects records, and %q is an aggregate", where, domain.PageRowsRecords, ds.ID))
	}
	if (ds.Where != nil || len(ds.Sort) > 0) && hasMoveBinding(n.Children) {
		issues = append(issues, fmt.Sprintf("%s: page: write: %s needs a Dataset that lists the Machine's records in the Machine's own order, and %q declares a where: or sort: -- a record's neighbour in a filtered or re-sorted list is not its neighbour in the order a move changes", where, domain.PageWriteMove, ds.ID))
	}
	if source == nil {
		return issues
	}
	if n.Binding != nil && n.Binding.Group != "" {
		issues = append(issues, groupBindingIssues(n.Binding.Group, ds, source, where)...)
	}
	if readsDerivedRole(n.Children, domain.PageGroupLabelRole) && source.DefaultView().GroupBy == "" {
		issues = append(issues, fmt.Sprintf("%s: page: from: %s -- machine %q's default View groups by no Field to derive it from", where, domain.PageGroupLabelRole, source.ID))
	}
	if (readsDerivedRole(n.Children, domain.PageDateLabelRole) || readsDerivedRole(n.Children, domain.PageDateToneRole)) && source.CardFieldFor(domain.CardFieldRoleDate) == "" {
		issues = append(issues, fmt.Sprintf("%s: page: from: %s/%s -- machine %q declares no card_fields role %q to derive them from", where, domain.PageDateLabelRole, domain.PageDateToneRole, source.ID, domain.CardFieldRoleDate))
	}
	if (readsDerivedRole(n.Children, domain.PageStatusToneRole) || columnsRead(n.Columns, domain.PageStatusToneRole)) && source.CardFieldFor(domain.CardFieldRoleStatus) == "" {
		issues = append(issues, fmt.Sprintf("%s: page: from: %s -- machine %q declares no card_fields role %q to derive it from", where, domain.PageStatusToneRole, source.ID, domain.CardFieldRoleStatus))
	}
	if (columnsRead(n.Columns, domain.PageDateLabelRole) || columnsRead(n.Columns, domain.PageDateToneRole)) && source.CardFieldFor(domain.CardFieldRoleDate) == "" {
		issues = append(issues, fmt.Sprintf("%s: page: columns: %s -- machine %q declares no card_fields role %q to derive it from", where, domain.PageDateLabelRole, source.ID, domain.CardFieldRoleDate))
	}
	if columnsRead(n.Columns, domain.PageGroupLabelRole) && source.DefaultView().GroupBy == "" {
		issues = append(issues, fmt.Sprintf("%s: page: columns: %s -- machine %q's default View groups by no Field to derive it from", where, domain.PageGroupLabelRole, source.ID))
	}
	fieldRefs := []string{}
	for _, c := range n.Columns {
		if c.Field != "" {
			fieldRefs = append(fieldRefs, c.Field)
		}
	}
	for _, role := range fieldRolesOf(n.Children) {
		if id, ok := domain.PageFieldOfRole(role); ok {
			fieldRefs = append(fieldRefs, id)
			if strings.HasPrefix(role, "steps:") || strings.HasPrefix(role, "tone:") {
				if f, ok := source.FieldByID(id); ok && f.Type != domain.FieldTypeStatus {
					issues = append(issues, fmt.Sprintf("%s: page: from: %s -- %q is a %s Field, and only a status has a tone and steps", where, role, id, f.Type))
				}
			}
		}
	}
	for _, ref := range fieldRefs {
		c := domain.PageColumn{Field: ref}
		f, ok := source.FieldByID(c.Field)
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("%s: page: columns: field: %s -- machine %q has no such Field", where, c.Field, source.ID))
		case f.Type == domain.FieldTypeGroup:
			issues = append(issues, fmt.Sprintf("%s: page: columns: field: %s -- a group Field is not yet a column (its label is the Workspace's group, read by no page binding)", where, c.Field))
		case f.Type == domain.FieldTypeFile:
			issues = append(issues, fmt.Sprintf("%s: page: columns: field: %s -- a file Field's value is its storage key, which is not for reading; open the record to see the file", where, c.Field))
		}
	}
	roles := fromRoles(n.Children)
	for _, c := range n.Columns {
		if c.From != "" && c.From != domain.PageRecordRole && !domain.PageDerivedRole(c.From) {
			roles[c.From] = true
		}
	}
	for _, role := range sortedKeys(roles) {
		fieldID := source.CardFieldFor(domain.CardFieldRole(role))
		f, ok := source.FieldByID(fieldID)
		switch {
		case fieldID == "" || !ok:
			issues = append(issues, fmt.Sprintf("%s: page: from: %s -- machine %q declares no card_fields role %q", where, role, source.ID, role))
		case f.IsReference() && f.RelatedMachine != domain.UserMachineID && domain.CardFieldRole(role) != domain.CardFieldRoleContainer:
			issues = append(issues, fmt.Sprintf("%s: page: from: %s -- role %q is a reference to another machine, which a page cannot yet resolve without reading that whole machine (007 §20); only a person Field and a container Field are resolved", where, role, role))
		}
	}
	return issues
}

// fieldRolesOf is every Field-keyed role (`field:`, `tone:`, `steps:`) a records template reads, sorted, not looking into
// a nested bound node.
func fieldRolesOf(nodes []domain.PageNode) []string {
	set := map[string]bool{}
	var walk func([]domain.PageNode)
	walk = func(ns []domain.PageNode) {
		for _, n := range ns {
			for _, role := range n.From {
				if _, ok := domain.PageFieldOfRole(role); ok {
					set[role] = true
				}
			}
			if n.Binding == nil {
				walk(n.Children)
			}
		}
	}
	walk(nodes)
	return sortedKeys(set)
}

// templateFieldRoles is every Field a records template reads by id, in the order first read, not looking into a nested
// node's own binding (its records are another Dataset's).
func templateFieldRoles(children []pageNodeDoc) []string {
	var out []string
	for _, c := range children {
		for _, role := range sortedKeys(rolesOf(c.from)) {
			if id, ok := domain.PageFieldOfRole(role); ok && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
		if c.binding == nil {
			for _, id := range templateFieldRoles(c.children) {
				if !slices.Contains(out, id) {
					out = append(out, id)
				}
			}
		}
	}
	return out
}

// columnsRead reports whether any of a Table's columns shows the given role.
func columnsRead(cols []domain.PageColumn, role string) bool {
	for _, c := range cols {
		if c.From == role {
			return true
		}
	}
	return false
}

// readsDerivedRole reports whether any node under nodes reads the given derived role.
func readsDerivedRole(nodes []domain.PageNode, derived string) bool {
	for _, n := range nodes {
		if n.Each == "" {
			for _, role := range n.From {
				if role == derived {
					return true
				}
			}
		}
		if readsDerivedRole(n.Children, derived) {
			return true
		}
	}
	return false
}

// hasMoveBinding reports whether any node under nodes is bound with `write: move`.
func hasMoveBinding(nodes []domain.PageNode) bool {
	for _, n := range nodes {
		if (n.Binding != nil && n.Binding.Write == domain.PageWriteMove) || hasMoveBinding(n.Children) {
			return true
		}
	}
	return false
}

// moveBindingIssues is what a `write: move` binding asks: to sit inside a records Collection, to name no dataset of
// its own, and a Machine whose records can be written at all (an append-only Machine refuses a reorder as it refuses
// any write). The Dataset's own shape is `recordsBindingIssues`' question, asked where the Dataset is in hand.
func moveBindingIssues(b domain.PageBinding, item *domain.Machine, where string) []string {
	switch {
	case item == nil:
		return []string{fmt.Sprintf("%s: page: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", where, domain.PageWriteMove, domain.PageRowsRecords)}
	case b.Dataset != "":
		return []string{fmt.Sprintf("%s: page: write: %s takes no dataset -- it moves the record of the Collection it sits in, and %q would be a second one (a join)", where, domain.PageWriteMove, b.Dataset)}
	case item.AppendOnly:
		return []string{fmt.Sprintf("%s: page: write: %s: machine %q is append-only, so no record of it can be moved", where, domain.PageWriteMove, item.ID)}
	}
	return nil
}

// transitionBindingIssues is what a `write: transition` Button asks, beyond what `move` does: that the Machine has a
// status Field to move (its `status` Projection role, a Field with options), that `becomes:` is one of those options or
// a sentinel the Machine's `completion:` can answer, and that a transition to it is not forbidden outright -- an edge
// declared to some other Action is refused by the patch route for every viewer, so the Button would never be drawn.
// Whether it is drawn *this* time, for this record and viewer, is the resolver's question and is asked per request.
func transitionBindingIssues(n domain.PageNode, item *domain.Machine, where string) []string {
	b := *n.Binding
	switch {
	case item == nil:
		return []string{fmt.Sprintf("%s: page: write: %s acts on a record, so it is valid only inside the item template of a Collection bound with rows: %s", where, domain.PageWriteTransition, domain.PageRowsRecords)}
	case b.Dataset != "":
		return []string{fmt.Sprintf("%s: page: write: %s takes no dataset -- it changes the status of the record of the Collection it sits in, and %q would be a second one (a join)", where, domain.PageWriteTransition, b.Dataset)}
	case item.AppendOnly:
		return []string{fmt.Sprintf("%s: page: write: %s: machine %q is append-only, so no record of it can be changed", where, domain.PageWriteTransition, item.ID)}
	}
	if n.Type == string(domain.ComponentForm) && b.Transition == "" {
		return []string{fmt.Sprintf("%s: page: a transition Form names its edge (binding: {write: transition, transition: trn_x}): what it asks is what that edge makes required", where)}
	}
	if b.Transition != "" {
		for _, e := range item.Transitions {
			if e.ID == b.Transition {
				if e.Action != domain.ActionEdit {
					return []string{fmt.Sprintf("%s: page: transition: %s is taken by the %q action, and a page's Button takes edit edges only", where, e.ID, e.Action)}
				}
				// An edge that asks for something (a proof when Paid) is a Form's, which asks it; a Button would send the
				// move without it and be refused every time.
				if n.Type == string(domain.ComponentButton) && len(item.TransitionInputs(e)) > 0 {
					return []string{fmt.Sprintf("%s: page: transition: %s asks for Fields its target makes required, so it is a Form (binding: {write: transition, transition: %s}), not a Button", where, e.ID, e.ID)}
				}
				return nil
			}
		}
		return []string{fmt.Sprintf("%s: page: transition: %s is not a declared edge of machine %q", where, b.Transition, item.ID)}
	}
	return transitionTargetIssues(item, n.Props["becomes"], where)
}

// transitionAllIssues is what a `write: transition` Button with `all: true` asks: a Dataset of records to act on, found
// in this Workspace, outside the item template of any Collection (inside one the Button already means "this record"),
// over a Machine that can be changed, and a target the status Field can hold.
func transitionAllIssues(n domain.PageNode, datasets map[string]domain.Dataset, machines map[string]*domain.Machine, item *domain.Machine, where string) []string {
	b := *n.Binding
	ds, ok := datasets[b.Dataset]
	switch {
	case item != nil:
		return []string{fmt.Sprintf("%s: page: write: %s with all: true acts on a Dataset, so it sits outside a Collection's item template (inside one, leave all: out and it means that record)", where, domain.PageWriteTransition)}
	case b.Dataset == "" || !ok:
		return []string{fmt.Sprintf("%s: page: write: %s with all: true names a dataset of this workspace, and %q is not one", where, domain.PageWriteTransition, b.Dataset)}
	case ds.Select != domain.SelectRecords:
		return []string{fmt.Sprintf("%s: page: write: %s with all: true needs a dataset that selects records, and %q is an aggregate", where, domain.PageWriteTransition, ds.ID)}
	case machines[ds.Source] == nil:
		return []string{fmt.Sprintf("%s: page: write: %s: dataset %q reads machine %q, which this workspace does not install", where, domain.PageWriteTransition, ds.ID, ds.Source)}
	case machines[ds.Source].AppendOnly:
		return []string{fmt.Sprintf("%s: page: write: %s: machine %q is append-only, so no record of it can be changed", where, domain.PageWriteTransition, ds.Source)}
	}
	return transitionTargetIssues(machines[ds.Source], n.Props["becomes"], where)
}

// transitionTargetIssues is the part both transition Buttons share: the Machine has a status Field to move and `becomes:`
// names one of its options or a sentinel its `completion:` answers.
func transitionTargetIssues(item *domain.Machine, becomesProp, where string) []string {
	fieldID := item.CardFieldFor(domain.CardFieldRoleStatus)
	field, ok := item.FieldByID(fieldID)
	if fieldID == "" || !ok || len(field.Options) == 0 {
		return []string{fmt.Sprintf("%s: page: write: %s needs machine %q to declare a `card_fields` entry with role %s over a Field that has options", where, domain.PageWriteTransition, item.ID, domain.CardFieldRoleStatus)}
	}
	becomes, target := becomesProp, becomesProp
	switch becomes {
	case domain.PageTransitionDone:
		if item.Completion == nil {
			return []string{fmt.Sprintf("%s: page: becomes: %s needs machine %q to declare a `completion:`", where, becomes, item.ID)}
		}
		target = item.Completion.Done
	case domain.PageTransitionReopen:
		if item.Completion == nil {
			return []string{fmt.Sprintf("%s: page: becomes: %s needs machine %q to declare a `completion:`", where, becomes, item.ID)}
		}
		target = item.ReopenValue()
	}
	if !slices.Contains(field.Options, target) {
		return []string{fmt.Sprintf("%s: page: becomes: %q is not an option of %s on machine %q (options: %s)", where, target, fieldID, item.ID, strings.Join(field.Options, ", "))}
	}
	return nil
}

// windowIssues is what a `window:` asks: that the Dataset exists in this Workspace and closes a date window the same
// way `group: date` reads one (a `select: records` Dataset over a Machine with a `date` role, one `gte` and one `lte`),
// since the words are read from those two bounds.
func windowIssues(datasetID string, datasets map[string]domain.Dataset, machines map[string]*domain.Machine, where string) []string {
	ds, ok := datasets[datasetID]
	if !ok {
		return []string{fmt.Sprintf("%s: page: window names dataset %q, which no machine in this workspace declares", where, datasetID)}
	}
	return groupBindingIssues(domain.PageGroupDate, ds, machines[ds.Source], where)
}

// stepIssues checks the one thing `ir.Lower` cannot: that a link's `step:` moves a parameter its destination reads.
// A calendar page reads the week through `$week` in a Dataset's `where:`; a link stepping `week` to a page whose
// Datasets read no such thing would draw a working link that selects nothing different.
func stepIssues(n domain.PageNode, navigation []domain.NavigationItem, datasets map[string]domain.Dataset, where string) []string {
	var issues []string
	if n.Step != nil && n.To != "" {
		for _, dest := range navigation {
			if dest.ID != n.To {
				continue
			}
			if dest.Page == nil || !pageReadsWeek(*dest.Page, datasets) {
				issues = append(issues, fmt.Sprintf("%s: page: to: %s with step: %s, but that page binds no Dataset reading $week", where, n.To, n.Step.Parameter))
			}
		}
	}
	for _, c := range n.Children {
		issues = append(issues, stepIssues(c, navigation, datasets, where)...)
	}
	return issues
}

func pageReadsWeek(n domain.PageNode, datasets map[string]domain.Dataset) bool {
	if n.Binding != nil {
		if ds, ok := datasets[n.Binding.Dataset]; ok {
			for _, c := range ds.Where.Comparisons() {
				if strings.HasPrefix(c.Value, expression.SentinelWeek) {
					return true
				}
			}
		}
	}
	for _, c := range n.Children {
		if pageReadsWeek(c, datasets) {
			return true
		}
	}
	return false
}

// conditionIssues is what a `when:` asks of the Machine of the records it sits over: that it projects the role, that
// a sentinel has a `completion:` to resolve from, and -- when the role's Field has options -- that a literal is one of
// them, so a misspelt status is a load error and not a node that is silently never (or always) drawn.
func conditionIssues(c domain.PageCondition, item *domain.Machine, where string) []string {
	if item == nil {
		return []string{fmt.Sprintf("%s: page: when: is valid only inside the item template of a Collection bound with rows: %s", where, domain.PageRowsRecords)}
	}
	fieldID := item.CardFieldFor(domain.CardFieldRole(c.Role))
	field, ok := item.FieldByID(fieldID)
	if fieldID == "" || !ok {
		return []string{fmt.Sprintf("%s: page: when: role %q -- machine %q declares no card_fields entry for it", where, c.Role, item.ID)}
	}
	operand := c.Is
	if operand == "" {
		operand = c.IsNot
	}
	switch operand {
	case domain.PageTransitionDone, domain.PageTransitionReopen:
		if item.Completion == nil {
			return []string{fmt.Sprintf("%s: page: when: %s needs machine %q to declare a `completion:`", where, operand, item.ID)}
		}
		if field.ID != item.Completion.Field {
			return []string{fmt.Sprintf("%s: page: when: %s is a value of %s, and role %q reads %s", where, operand, item.Completion.Field, c.Role, field.ID)}
		}
		return nil
	}
	if len(field.Options) > 0 && !slices.Contains(field.Options, operand) {
		return []string{fmt.Sprintf("%s: page: when: %q is not an option of %s on machine %q (options: %s)", where, operand, field.ID, item.ID, strings.Join(field.Options, ", "))}
	}
	return nil
}

// groupBindingIssues is what `group: date` asks of its Dataset and Machine: the Machine declares the `date` role
// to bucket by, and the Dataset closes a window on that Field with exactly one `gte` and one `lte` in an `all:`.
// An `any:` would let a record in without either bound holding, and a second bound makes "the window" ambiguous;
// both are refused here because the days a page draws are *read from* those two comparisons (composition's
// dateBuckets), and a window that cannot be read has to be a load error and not an empty calendar.
func groupBindingIssues(group string, ds domain.Dataset, source *domain.Machine, where string) []string {
	if group != domain.PageGroupDate {
		return []string{fmt.Sprintf("%s: page: group: %q is not a dimension a records binding groups by (%s)", where, group, domain.PageGroupDate)}
	}
	field := source.CardFieldFor(domain.CardFieldRoleDate)
	if field == "" {
		return []string{fmt.Sprintf("%s: page: group: %s -- machine %q declares no card_fields role %q to bucket by", where, group, source.ID, domain.CardFieldRoleDate)}
	}
	if ds.Where == nil || len(ds.Where.Any) > 0 {
		return []string{fmt.Sprintf("%s: page: group: %s -- dataset %q must close its window with an all: of one gte and one lte on %s, and has no all-only where:", where, group, ds.ID, field)}
	}
	var lower, upper int
	for _, c := range ds.Where.All {
		if c.Field != field {
			continue
		}
		switch c.Op {
		case expression.OpGreaterOrEqual:
			lower++
		case expression.OpLessOrEqual:
			upper++
		}
	}
	if lower != 1 || upper != 1 {
		return []string{fmt.Sprintf("%s: page: group: %s -- dataset %q must close its window on %s with exactly one gte and one lte, and has %d and %d", where, group, ds.ID, field, lower, upper)}
	}
	return nil
}

// fromRoles collects every Projection role named by a `from:` anywhere under nodes.
func fromRoles(nodes []domain.PageNode) map[string]bool {
	out := map[string]bool{}
	for _, n := range nodes {
		if n.Each != "" {
			// An `each:` node reads its tag, not the record: its roles are checked against the tag's two.
			continue
		}
		for _, role := range n.From {
			if _, byField := domain.PageFieldOfRole(role); role != domain.PageRecordRole && role != domain.PageBucketLabelRole && !domain.PageDerivedRole(role) && !byField { // reserved: the record's route, a bucket's name, the derived roles and a Field read by id, not Projection roles
				out[role] = true
			}
		}
		for role := range fromRoles(n.Children) {
			out[role] = true
		}
	}
	return out
}

func rolesOf(from map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, role := range from {
		out[role] = true
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func slicesHasMeasure(ds domain.Dataset, id string) bool {
	for _, m := range ds.Measures {
		if m.ID == id {
			return true
		}
	}
	return false
}

func shapeIssues(root domain.PageNode, navigation []domain.NavigationItem, datasets map[string]domain.Dataset, where string) []string {
	ensureIRVocabulary()
	tree, err := ir.Lower(root, PlaceholderResolver(navigation, datasets))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", where, err)}
	}
	var issues []string
	for _, msg := range ir.Validate(tree) {
		issues = append(issues, fmt.Sprintf("%s: page: %s", where, msg))
	}
	var walk func(n ir.UINode)
	walk = func(n ir.UINode) {
		if n.Kind == ir.NodeComponent {
			for _, msg := range registry.ValidateComponentUse(domain.ComponentType(n.Type), n.Props) {
				issues = append(issues, fmt.Sprintf("%s: %s", where, msg))
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(tree)
	return issues
}

// paramLinkIssues checks the one thing `ir.Lower` cannot: that a Metric's `to:` + `param:` sends a value the
// destination actually reads. The destination is a page whose bound Datasets name `$parameters.<param>` in
// their `where:`; a link carrying `?status=` to a page that reads no such parameter would draw a working link
// that filters nothing, which no test of either page alone would notice. An unknown `to:` is `ir.Lower`'s to
// report, so it is skipped here rather than reported twice.
func paramLinkIssues(n domain.PageNode, navigation []domain.NavigationItem, datasets map[string]domain.Dataset, where string) []string {
	var issues []string
	if n.To != "" && n.Param != "" {
		for _, dest := range navigation {
			if dest.ID != n.To {
				continue
			}
			if dest.Page == nil || !pageReadsParameter(*dest.Page, n.Param, datasets) {
				issues = append(issues, fmt.Sprintf("%s: page: to: %s with param: %s, but that page binds no Dataset filtering on $parameters.%s", where, n.To, n.Param, n.Param))
			}
		}
	}
	for _, c := range n.Children {
		issues = append(issues, paramLinkIssues(c, navigation, datasets, where)...)
	}
	return issues
}

func pageReadsParameter(n domain.PageNode, param string, datasets map[string]domain.Dataset) bool {
	if n.Binding != nil {
		if ds, ok := datasets[n.Binding.Dataset]; ok {
			for _, c := range ds.Where.Comparisons() {
				if c.Value == expression.SentinelParameterPrefix+param {
					return true
				}
			}
		}
	}
	for _, c := range n.Children {
		if pageReadsParameter(c, param, datasets) {
			return true
		}
	}
	return false
}

// actionBindingIssues is what a `write: action` binding asks: an action the registry knows, a Form only for an action
// that asks something and a Button only for one that asks nothing, and a place that matches what the action is about.
// An action about a record sits inside a Collection of that source's runtime Machine and names no dataset; one that is
// not about a record sits outside any. (That only the runtime's own pages may use one is validatePages' question.)
func actionBindingIssues(n domain.PageNode, item *domain.Machine, where string) []string {
	b := *n.Binding
	a, ok := registry.RuntimeActions[b.Action]
	switch {
	case !ok:
		return []string{fmt.Sprintf("%s: page: action %q is not an operation the runtime performs (%s)", where, b.Action, strings.Join(registry.RuntimeActionIDs(), ", "))}
	case b.Dataset != "" || b.Rows != "" || b.Measure != "":
		return []string{fmt.Sprintf("%s: page: write: %s names the action and nothing else: a dataset, rows or measure would be a second thing it acts on", where, domain.PageWriteAction)}
	case n.Type == string(domain.ComponentForm) && len(a.Inputs) == 0:
		return []string{fmt.Sprintf("%s: page: action %q asks nothing, so it is a Button and not a Form", where, a.ID)}
	case n.Type == string(domain.ComponentButton) && len(a.Inputs) > 0:
		return []string{fmt.Sprintf("%s: page: action %q asks for %d thing(s), so it is a Form and not a Button", where, a.ID, len(a.Inputs))}
	case a.Source == "" && item != nil:
		return []string{fmt.Sprintf("%s: page: action %q is not about a record, so it sits outside a Collection's item template", where, a.ID)}
	case a.Source != "" && (item == nil || item.Runtime != a.Source):
		return []string{fmt.Sprintf("%s: page: action %q is about a record of the runtime source %q, so it sits inside the item template of a Collection over that source's machine", where, a.ID, a.Source)}
	}
	return nil
}

// usesAction reports whether any node under n performs a runtime action.
func usesAction(n domain.PageNode) bool {
	if n.Binding != nil && n.Binding.Write == domain.PageWriteAction {
		return true
	}
	for _, c := range n.Children {
		if usesAction(c) {
			return true
		}
	}
	return false
}
