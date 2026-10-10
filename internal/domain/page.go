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
	From map[string]string
	// To is the navigation item a `static: link` points at (`to: nav_approval_inbox`). A link's destination is
	// never a typed route (001 #3, #8): `ir.Lower` resolves it from the Application's own `navigation:`, so
	// renaming a route in one place moves every page that links to it. Valid only on a `link`, which in turn
	// has no other way to name a destination.
	To string
	// ListOf is the Dataset whose Machine a `static: link` opens the generic list page of
	// (`list_of: ds_recent_documents` -> `MachineListRoute(<that Dataset's source>)`). It is the second way a
	// link names a destination, for the screens that have no navigation item: a Machine claimed by no
	// Application's `navigation:` still has a page, and a typed `/machines/<id>` would be a route retyped
	// (001 #3, #8). The Dataset is the declared handle on the Machine, the way a Binding's is, so the same
	// load-time check (it exists in this Workspace) applies. Valid only on a `link`, and never beside `to:`.
	ListOf string
	// Count is the words a `static` node says about how many children of the record a declared Relation
	// (007 §7.5) found: `count: {of: rel_label_cards, one: "used on {n} card", other: "used on {n} cards"}`.
	// The number is never written by the author -- it comes from the Relation the bound Dataset declares --
	// and neither is the node's `text`, which this produces. Valid only on a `static` node inside a
	// records-bound Collection's item template; `ir.Lower` refuses it anywhere else.
	Count *PageCount
	// Param is the query-string parameter a dimension-bound Metric carries its row's Dimension value in when it
	// links (`to: nav_documents_in_status`, `param: status` -> `/pages/nav_documents_in_status?status=<value>`).
	// It is the other end of a destination's `$parameters.<name>`: the page that reads the parameter declares
	// the name in its Dataset, this names it again only where a value is sent, and the loader checks the two
	// agree. Valid only beside `to:` on a Metric bound with rows: dimension; `ir.Lower` refuses it anywhere else.
	Param string
	// When makes the node appear only for the records it holds for (007 §15.2, "conditional visibility"):
	// `when: {role: status, is_not: $done}`. It is a statement about the record's own Projection role, so it
	// is valid only inside a records-bound Collection's item template, and a node it hides is not lowered at
	// all -- its children are not either, which is what makes it a visibility rule and not a CSS class.
	When     *PageCondition
	Children []PageNode
}

// PageCondition is a record-scoped test on a Projection role: Role is a `card_fields` role, and exactly one of Is
// and IsNot is set. The operand is a literal value or one of the two sentinels a transition's `becomes:` already
// uses (PageTransitionDone, PageTransitionReopen), resolved from the Machine's `completion:` -- so a page says "not
// done" without retyping the word the Machine already declares (001 #8). No grammar beyond that (007 §9.2): one
// role, one comparison, no `and`/`or`.
type PageCondition struct {
	Role  string
	Is    string
	IsNot string
}

// PageCount is a node's plural text over a Relation's child count. Of is a Relation id of the Dataset the
// enclosing Collection is bound to; Other is required and is the text for every n but 1 (zero included, since
// "used on 0 cards" is the English plural and a count that vanishes at zero makes the page's shape depend on
// the data); One is optional and, when absent, Other serves n == 1 too. `{n}` is the one substitution -- a
// number, never an expression (007 §9.2). Two forms are what English and Indonesian need; a language with
// more plural categories is a capability nobody has asked for.
type PageCount struct {
	Of    string
	One   string
	Other string
}

// PageCountToken is the single substitution a PageCount text may carry.
const PageCountToken = "{n}"

// PageCountRole is the key under which a record's child count for one Relation travels in the record map a
// records Binding hands to `ir.Lower`. Like PageRecordRole it is reserved: the colon cannot appear in a
// Projection role, so a count is never mistaken for a `card_fields` role and a role never for a count.
func PageCountRole(relationID string) string {
	return "count:" + relationID
}

// MachineListRoute is the runtime's generic page for a Machine (`GET /machines/{machineID}`), the list of its
// records and where one is created. It belongs to the runtime the way `/home` does, not to an Application, so
// it is assembled in one place and not read from `navigation:`; the page it names still answers for itself
// who may see it, since a link is an address and not a grant.
func MachineListRoute(machineID string) string {
	return "/machines/" + machineID
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
	// Write is the write-side mode (007 §11.3): `create` binds a `Form` to the Dataset's Machine, so that
	// submitting it creates a record through the generic create route; `update` binds one inside a records
	// template to the item's own record, through the generic patch route. It is a mode of its own and never
	// beside Rows or Measure, which read.
	Write string
}

// The Rows modes. `dimension` is a Metric per Dimension value, `records` a Collection item per record, and
// `total` one Metric holding a Measure's figure over the whole Dataset -- the mode for a number that has no
// breakdown, whose label the author writes because there is no Dimension value to name it (2026-10-10).
const (
	PageRowsDimension = "dimension"
	PageRowsRecords   = "records"
	PageRowsTotal     = "total"
)

// The write modes. `create` binds a Form to a Dataset's Machine. `update` is valid only inside the item template
// of a records Collection, where the record it changes is the item's own and the Dataset is the Collection's, so
// a binding there names no dataset of its own (a second one would be a join the page has no grammar for).
// `delete` acts on a record in the same place and on the same terms, through a `Button` and not a `Form`: there is
// nothing to ask, only a consequence to confirm. `move` is the same place and terms again, a `Button` that sends the
// record one step up or down in its Machine's order (`direction: up|down`); it needs no confirmation because it is
// undone by the opposite button.
const (
	PageWriteCreate = "create"
	PageWriteUpdate = "update"
	PageWriteDelete = "delete"
	PageWriteMove   = "move"
	// PageWriteTransition is a `Button` that moves the record's status Field (the Machine's `status` Projection role)
	// to the value `becomes:` names, through the same generic patch route as an edit, so the state model's declared
	// edges (`CheckTransitions`) and the `edit` Permission are its guards. Drawn only on a record for which that
	// move is allowed, which is also why a "Reopen" and a "Mark done" Button can sit side by side: one shows.
	PageWriteTransition = "transition"
)

// The two words a transition Button's `becomes:` may use instead of a status option, both read from the Machine's own
// `completion:` block so "finished" is declared once there and never retyped in a page: `$done` is the finished
// value, `$reopen` is what finishing's opposite writes (`Machine.ReopenValue`).
const (
	PageTransitionDone   = "$done"
	PageTransitionReopen = "$reopen"
)

// WritableComponents maps each registered Component a Binding may write through to the Write modes it takes, the
// way BindableComponents does for reading. Vocabulary, so it lives here and not in `internal/registry`.
var WritableComponents = map[ComponentType][]string{
	ComponentForm:   {PageWriteCreate, PageWriteUpdate},
	ComponentButton: {PageWriteDelete, PageWriteMove, PageWriteTransition},
}

// PageRecordRole is the one `from:` role that is not a Projection role: `from: {href: record}` on a
// `static: link` inside a records template takes the record's own generic route
// (`/machines/<Machine>/records/<id>`, which the runtime owns the way it owns `/home`). It is a reserved word
// rather than a `card_fields` role because a role describes what a Machine says about a record, and every
// Machine would otherwise have to declare a "link" to be listed on a page; and it is valid for nothing but a
// link's `href`, so a record's route is never shown as text or handed to another property.
const PageRecordRole = "record"

// BindableComponents maps each registered Component a Binding may supply to the Rows modes it takes. A
// Binding on any other node, or in a mode not listed, is a load error rather than a property the renderer
// ignores.
//
// Vocabulary (a closed map of strings), so it lives here and not in `internal/registry`
// (`conformance.TestDomainHoldsVocabularyAndRegistryHoldsDispatch`). `Metric` takes dimension rows or one
// total: it is the Component whose contract is "a label and a resolved number". `Collection` takes records: it
// is the one with an `item` slot, so a list of already-composed items is exactly what a record set is.
var BindableComponents = map[ComponentType][]string{
	ComponentMetric:     {PageRowsDimension, PageRowsTotal},
	ComponentCollection: {PageRowsRecords},
}
