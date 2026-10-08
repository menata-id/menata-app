package metadata

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"menata.app/internal/domain"
	"menata.app/internal/expression"
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
// The reserved keys are the three discriminators, `children`, `binding`, `from` and `to`. Exactly one discriminator per
// node: `layout:`, `static:` or `component:`, whose value is the type.
type pageNodeDoc struct {
	kind     string
	typ      string
	props    map[string]string
	binding  *pageBindingDoc
	from     map[string]string
	to       string
	children []pageNodeDoc
}

type pageBindingDoc struct {
	Dataset string
	Measure string
	Rows    string
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

func decodePageBinding(n *yaml.Node) (*pageBindingDoc, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: binding: is a mapping of dataset, measure and rows", n.Line)
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
		default:
			return nil, fmt.Errorf("line %d: %q is not a key a binding declares (dataset, measure, rows)", key.Line, key.Value)
		}
	}
	return b, nil
}

func (p pageNodeDoc) toDomain() domain.PageNode {
	out := domain.PageNode{Kind: p.kind, Type: p.typ, Props: p.props, To: p.to}
	if len(out.Props) == 0 {
		out.Props = nil
	}
	if p.binding != nil {
		out.Binding = &domain.PageBinding{Dataset: p.binding.Dataset, Measure: p.binding.Measure, Rows: p.binding.Rows}
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
// row, and one record that projects every role in `domain.KnownCardFieldRoles`. A `from:` naming anything
// outside that vocabulary therefore fails to lower, which is how a typo in a role is a load error. Exported so
// the conformance sweep over installed manifests asks the same question the loader does.
//
// Routes are **not** a placeholder: a `to:` is resolved against the Application's real navigation, because
// whether that item exists is exactly what is being checked, and no database is needed to know.
func PlaceholderResolver(navigation []domain.NavigationItem) ir.Resolver {
	return ir.Resolver{
		Route: ir.NavigationRoutes(navigation),
		Rows: func(domain.PageBinding) ([]ir.Row, error) {
			return []ir.Row{{Label: "label", Value: "0"}}, nil
		},
		Records: func(domain.PageBinding) ([]map[string]string, error) {
			rec := map[string]string{domain.PageRecordRole: "/x"}
			for role := range domain.KnownCardFieldRoles {
				rec[string(role)] = "x"
			}
			return []map[string]string{rec}, nil
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
func validatePages(applications []domain.Application, machines []*domain.Machine) error {
	datasets := map[string]domain.Dataset{}
	byID := map[string]*domain.Machine{}
	for _, m := range machines {
		byID[m.ID] = m
		for _, ds := range m.Datasets {
			datasets[ds.ID] = ds
		}
	}
	var issues []string
	for _, app := range applications {
		for _, item := range app.AllNavigation {
			if item.Page == nil {
				continue
			}
			where := fmt.Sprintf("application %q: navigation item %q", app.ID, item.ID)
			if want := "/pages/" + item.ID; item.Route != want {
				issues = append(issues, fmt.Sprintf("%s: page: route is %q but a page is rendered at %q", where, item.Route, want))
			}
			if item.Page.Kind != "layout" {
				issues = append(issues, fmt.Sprintf("%s: page: the root must be a layout, not %s %q", where, item.Page.Kind, item.Page.Type))
				continue
			}
			issues = append(issues, bindingIssues(*item.Page, datasets, byID, where)...)
			issues = append(issues, shapeIssues(*item.Page, app.AllNavigation, where)...)
		}
	}
	if len(issues) > 0 {
		sort.Strings(issues)
		issues = slices.Compact(issues)
		return &ValidationError{Issues: issues}
	}
	return nil
}

func bindingIssues(n domain.PageNode, datasets map[string]domain.Dataset, machines map[string]*domain.Machine, where string) []string {
	var issues []string
	if b := n.Binding; b != nil {
		ds, ok := datasets[b.Dataset]
		switch {
		case !ok:
			issues = append(issues, fmt.Sprintf("%s: page: binding names dataset %q, which no machine in this workspace declares", where, b.Dataset))
		case b.Rows == domain.PageRowsRecords:
			issues = append(issues, recordsBindingIssues(n, ds, machines[ds.Source], where)...)
		case ds.Select != "":
			issues = append(issues, fmt.Sprintf("%s: page: binding dataset %q selects records, and a dimension binding needs an aggregate dataset with measures (use rows: %s on a Collection to list records)", where, b.Dataset, domain.PageRowsRecords))
		default:
			if !slicesHasMeasure(ds, b.Measure) {
				issues = append(issues, fmt.Sprintf("%s: page: dataset %q declares no measure %q", where, b.Dataset, b.Measure))
			}
			if b.Rows == domain.PageRowsDimension && ds.Dimension == "" {
				issues = append(issues, fmt.Sprintf("%s: page: dataset %q declares no dimension, so it has no rows to expand", where, b.Dataset))
			}
		}
	}
	for _, c := range n.Children {
		issues = append(issues, bindingIssues(c, datasets, machines, where)...)
	}
	return issues
}

// recordsBindingIssues is what a records binding asks of its Dataset and of the Machine it reads.
//
//   - the Dataset **selects records** -- an aggregate has no rows to list;
//   - it filters on nothing a page cannot supply: `$current_user` is the viewer and is fine, a
//     `$parameters.<name>` is a route value and a page has no route parameters (007 §9.2, fail closed);
//   - every `from:` role in the item template is one the Dataset's Machine declares in `card_fields`, and the
//     Field behind it **is not a reference**. A reference Field resolves through `rendering.RelationOptions`,
//     which is a read of the whole related Machine (007 §20: no plane may read everything and trim later), so
//     it is refused here until a records binding can follow a declared Relation instead.
func recordsBindingIssues(n domain.PageNode, ds domain.Dataset, source *domain.Machine, where string) []string {
	var issues []string
	if ds.Select != domain.SelectRecords {
		issues = append(issues, fmt.Sprintf("%s: page: rows: %s needs a dataset that selects records, and %q is an aggregate", where, domain.PageRowsRecords, ds.ID))
	}
	for _, c := range ds.Where.Comparisons() {
		if strings.HasPrefix(c.Value, expression.SentinelParameterPrefix) {
			issues = append(issues, fmt.Sprintf("%s: page: dataset %q filters on %s, and a page has no request parameters to supply", where, ds.ID, c.Value))
		}
	}
	if source == nil {
		return issues
	}
	for _, role := range sortedKeys(fromRoles(n.Children)) {
		fieldID := source.CardFieldFor(domain.CardFieldRole(role))
		f, ok := source.FieldByID(fieldID)
		switch {
		case fieldID == "" || !ok:
			issues = append(issues, fmt.Sprintf("%s: page: from: %s -- machine %q declares no card_fields role %q", where, role, source.ID, role))
		case f.IsReference():
			issues = append(issues, fmt.Sprintf("%s: page: from: %s -- role %q is a reference to another machine, which a page cannot yet resolve without reading that whole machine (007 §20)", where, role, role))
		}
	}
	return issues
}

// fromRoles collects every Projection role named by a `from:` anywhere under nodes.
func fromRoles(nodes []domain.PageNode) map[string]bool {
	out := map[string]bool{}
	for _, n := range nodes {
		for _, role := range n.From {
			if role != domain.PageRecordRole { // reserved: the record's route, not a Projection role
				out[role] = true
			}
		}
		for role := range fromRoles(n.Children) {
			out[role] = true
		}
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

func shapeIssues(root domain.PageNode, navigation []domain.NavigationItem, where string) []string {
	ensureIRVocabulary()
	tree, err := ir.Lower(root, PlaceholderResolver(navigation))
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
