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
	Kind     string
	Type     string
	Props    map[string]string
	Binding  *PageBinding
	Children []PageNode
}

// PageBinding is 007 §11.3's read side in its narrowest honest form: a node takes its content from one
// Dataset's aggregate. No expression, no path, no second Dataset -- §9.2's allowed-context rule, applied by
// not having a grammar to misuse.
//
// Rows is the only mode there is: `dimension` expands the bound node into **one node per value of the
// Dataset's Dimension**, each labelled by that value and carrying that group's Measure. A scalar mode
// (`{dataset, measure}` -> one number) was the first design and was dropped on measurement: every aggregate
// Dataset in the template library declares a Dimension, so a scalar Binding had nothing to bind
// (menata-app-document audits/2026-10-07-rencana-page-yaml..., §9.6).
type PageBinding struct {
	Dataset string
	Measure string
	Rows    string
}

// PageRowsDimension is the one value of PageBinding.Rows.
const PageRowsDimension = "dimension"

// BindableComponents are the registered Components whose content a Binding may supply. A Binding on any
// other node is a load error rather than a property the renderer ignores.
//
// Vocabulary (a bool-valued closed set), so it lives here and not in `internal/registry`
// (`conformance.TestDomainHoldsVocabularyAndRegistryHoldsDispatch`). `Metric` only: it is the one Component
// whose contract is "a label and a resolved number", which is what a dimension row is.
var BindableComponents = map[ComponentType]bool{
	ComponentMetric: true,
}
