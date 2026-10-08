package domain

// PageNode is one node of a `page:` declaration: what an Application writes in a navigation item to say what
// a screen's body is made of (007 §12.4, §15.1's first stage).
//
// **It is the declaration, not the UI IR.** `internal/ir.UINode` carries resolved properties and no Binding;
// a PageNode carries the author's words, including a Binding that names a Dataset. `ir.Lower` turns one into
// the other, and it lives in `domain` rather than `ir` because `internal/ir` imports `domain` and the reverse
// would be a cycle (001 #17: YAML is a declaration, not the compiler's intermediate form).
//
// Kind is `layout`, `static` or `component` -- the three planes a Page composes from -- and Type is the
// closed vocabulary that Kind owns (`LayoutKind`, `StaticKind`, `ComponentType`). Props are every other
// scalar key on the node, stringified; **which keys a Type accepts is not decided here** but by
// `ir.Validate`'s allowed-property table and the Component's own registered validator, so this type stays
// vocabulary-free and a new Layout property is one table entry rather than a parser change.
type PageNode struct {
	Kind    string
	Type    string
	Props   map[string]string
	Binding *PageBinding
	// From maps a property of this node to the Projection role (`card_fields`, 007 §7.6) it takes from the
	// record the enclosing records-bound Collection is drawing: `{text: title}` means "this node's text is
	// that record's title". Valid only inside such a Collection's item template -- `ir.Lower` refuses it
	// anywhere else, and the property it names may not also be written on the node.
	From     map[string]string
	Children []PageNode
}

// PageBinding is 007 §11.3's read side in its narrowest honest form: a node takes its content from one
// Dataset's aggregate. No expression, no path, no second Dataset -- §9.2's allowed-context rule, applied by
// not having a grammar to misuse.
//
// Rows is the mode, and there are two. `dimension` expands the bound node into **one node per value of the
// Dataset's Dimension**, each labelled by that value and carrying that group's Measure; it needs `measure`.
// `records` expands a Collection into **one item per record of a `select: records` Dataset**, each item the
// Collection's single template child with its `from:` properties filled from the record's Projection roles;
// it takes no `measure`. A scalar mode (`{dataset, measure}` -> one number) was the first design and was
// dropped on measurement: every aggregate Dataset in the template library declares a Dimension, so a scalar
// Binding had nothing to bind (menata-app-document audits/2026-10-07-rencana-page-yaml..., §9.6).
type PageBinding struct {
	Dataset string
	Measure string
	Rows    string
}

// The two Rows modes.
const (
	PageRowsDimension = "dimension"
	PageRowsRecords   = "records"
)

// BindableComponents maps each registered Component a Binding may supply to the one Rows mode it takes. A
// Binding on any other node, or in the other mode, is a load error rather than a property the renderer
// ignores.
//
// Vocabulary (a closed map of strings), so it lives here and not in `internal/registry`
// (`conformance.TestDomainHoldsVocabularyAndRegistryHoldsDispatch`). `Metric` takes dimension rows: it is the
// Component whose contract is "a label and a resolved number". `Collection` takes records: it is the one with
// an `item` slot, so a list of already-composed items is exactly what a record set is.
var BindableComponents = map[ComponentType]string{
	ComponentMetric:     PageRowsDimension,
	ComponentCollection: PageRowsRecords,
}
