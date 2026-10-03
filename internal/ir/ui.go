package ir

import (
	"fmt"

	"menata.app/internal/domain"
)

// MaxDepth is the configured composition depth limit 007 §15.3 requires a compiler to reject beyond.
//
// Eight, because the deepest tree this corpus builds is three (a stack holding three static nodes), and a
// limit exists to stop runaway recursion rather than to be a budget anyone plans against. It is named rather
// than inlined so the rejection can cite it.
const MaxDepth = 8

// NodeKind is what sort of node this is: one of the three planes §12 gives a Page to compose from.
type NodeKind string

const (
	// NodeLayout is a generic spatial primitive (§12.2) -- it exists to hold children.
	NodeLayout NodeKind = "layout"
	// NodeStatic is a static content node (§12.6) -- explanatory text, a leaf.
	NodeStatic NodeKind = "static"
	// NodeComponent is a registered semantic Component (§12.3) -- a leaf with a bounded contract.
	NodeComponent NodeKind = "component"
)

// KnownNodeKinds is the closed set. A kind absent from it is a rejection, never a silent skip.
var KnownNodeKinds = map[NodeKind]bool{NodeLayout: true, NodeStatic: true, NodeComponent: true}

// UINode is the UI IR: one node of a composed experience tree (007 §15).
//
// **Why this exists now, when `doc.go` deferred it.** That deferral named its own trigger -- "a second render
// target ... one target means the templ functions already are the tree" -- and it was measured on 2026-09-30,
// before the Experience Plane's primitives existed. It has expired on its own terms. There are now five
// Layout primitives with 45 call sites and two registered Components with 13, so the *vocabulary* is shared
// across screens and the only thing still hardcoded is the **composition**. A templ function is the tree only
// while nothing else could express it; once `stack`, `panel` and `staticText` are named values, a tree of
// those names is data, and keeping it as Go is 001 #3 inverted.
//
// The forcing reason is stronger than convenience: 007 §12.4 states as a **normative rule** that a View "MUST
// NOT be required as the universal composition primitive", and this runtime renders 38 screens from 38
// bespoke Go functions. That is a standing breach, and §24 prescribes the remedy as *progressive* lowering --
// which is also why this arrives as one block of the §15.1 pipeline rather than all ten stages.
//
// Fields are §15's own sketch, minus the ones with no consumer yet. `Bindings`, `Slots`, `Actions` and
// `Conditions` are deliberately absent: a field named and not built is worse than a switch, because the name
// suggests otherwise (`capability-lifecycle.md` §4, and this tree's own three zero-caller layout primitives).
type UINode struct {
	// Kind and Type together name the node: `layout`/`stack`, `static`/`heading`,
	// `component`/`StatusBadge`. Type is validated against the closed set its Kind owns, so an unknown
	// string is a rejection rather than a node that renders nothing.
	Kind NodeKind
	Type string
	// Identity is the stable identity §15.2 asks UI IR to encode. Optional: a node with no identity is
	// anonymous and cannot be referenced. Where present it must be unique along its own ancestor path,
	// which is what makes §15.3's cyclic-tree rejection checkable on a value tree.
	Identity string
	// Props are resolved properties -- already-decided values, never a CSS class and never a lookup the
	// renderer would have to perform. §15.2 forbids this representation embedding "HTML, CSS framework
	// classes, SQL, or database-specific implementation", and `conformance` holds this package to it.
	Props map[string]string
	// Children are the composed subtree. A leaf Kind must not have any -- that is §15.3's slot/type
	// mismatch, which on a tree with no named slots yet means "a leaf was given a subtree".
	Children []UINode
}

// layoutsRequiringChildren are the Layout kinds whose whole purpose is to hold something. A `stack` with no
// children is §15.3's "unresolved required child reference": the node was composed and its content was not.
//
// Read out of domain's own closed set rather than retyped, so a new Layout kind is covered here the day it is
// declared -- every Layout exists to hold children, which is what distinguishes §12.2 from §12.6.
func layoutRequiresChildren(t domain.LayoutKind) bool {
	return domain.KnownLayoutKinds[t]
}

// Validate is 007 §15.3's five mandatory rejections, in one pass.
//
// It returns every problem rather than the first, the posture every validator in this tree has: a tree with
// two faults should report two. §15.3's sixth clause ("SHOULD detect duplicate or unnecessary data
// dependencies") is deliberately not here -- a node has no data dependency to duplicate until `Bindings`
// exists, and a check over an absent field would be a gate measuring nothing.
func Validate(root UINode) []string {
	var issues []string
	validate(root, 1, map[string]bool{}, &issues)
	return issues
}

func validate(n UINode, depth int, ancestry map[string]bool, issues *[]string) {
	// (5) recursion that exceeds the configured composition depth limit.
	if depth > MaxDepth {
		*issues = append(*issues, fmt.Sprintf("composition depth exceeds the limit of %d at node %q", MaxDepth, n.describe()))
		return
	}

	// (1) cyclic experience trees. A Go value tree cannot contain itself, so the reachable form of a cycle is
	// a node repeating an identity already on its own ancestor path -- which is exactly what a tree assembled
	// from references produces, and the only shape of this fault that can exist here.
	if n.Identity != "" {
		if ancestry[n.Identity] {
			*issues = append(*issues, fmt.Sprintf("cyclic experience tree: identity %q appears inside itself", n.Identity))
			return
		}
		ancestry[n.Identity] = true
		defer delete(ancestry, n.Identity)
	}

	if !KnownNodeKinds[n.Kind] {
		*issues = append(*issues, fmt.Sprintf("node kind %q is not one this runtime realizes", n.Kind))
		return
	}

	switch n.Kind {
	case NodeLayout:
		t := domain.LayoutKind(n.Type)
		if !domain.KnownLayoutKinds[t] {
			*issues = append(*issues, fmt.Sprintf("layout type %q is not one this runtime realizes", n.Type))
		}
		// (2) unresolved required child references.
		if layoutRequiresChildren(t) && len(n.Children) == 0 {
			*issues = append(*issues, fmt.Sprintf("layout %q has no children -- a Layout exists to hold them (007 §15.3, unresolved required child reference)", n.Type))
		}
	case NodeStatic:
		if !domain.KnownStaticKinds[domain.StaticKind(n.Type)] {
			*issues = append(*issues, fmt.Sprintf("static content type %q is not one this runtime realizes", n.Type))
		}
		// (3) slot/type mismatch: a leaf was given a subtree.
		if len(n.Children) > 0 {
			*issues = append(*issues, fmt.Sprintf("static node %q has %d children -- static content is a leaf and exposes no slot (007 §15.3, slot/type mismatch)", n.Type, len(n.Children)))
		}
	case NodeComponent:
		if !isKnownComponent(n.Type) {
			*issues = append(*issues, fmt.Sprintf("component type %q is not one this runtime realizes", n.Type))
		}
		if len(n.Children) > 0 {
			*issues = append(*issues, fmt.Sprintf("component %q has %d children and declares no slots (007 §15.3, slot/type mismatch)", n.Type, len(n.Children)))
		}
	}

	// (4) bindings outside the permitted scope. Props are the binding surface this node has, and the
	// permitted scope is "what this node's type declares". Fail closed, the way 007 §9.2's allowed-context
	// rule does: an unrecognised key is a rejection, not a value the renderer quietly ignores.
	for k := range n.Props {
		if !propAllowed(n, k) {
			*issues = append(*issues, fmt.Sprintf("%s %q carries property %q, which its type does not declare (007 §15.3, binding outside the permitted scope)", n.Kind, n.Type, k))
		}
	}

	for _, c := range n.Children {
		validate(c, depth+1, ancestry, issues)
	}
}

func (n UINode) describe() string {
	if n.Identity != "" {
		return n.Identity
	}
	return string(n.Kind) + "/" + n.Type
}

// isKnownComponent asks the Component catalogue, by name rather than by importing it.
//
// `internal/registry` holds the catalogue and may import `internal/domain` only; this package is downstream
// of neither, so it could import registry -- but doing so would make the IR depend on the dispatch seam, and
// §15 describes UI IR as a representation rather than a resolver. So the known set is injected once at
// startup instead, which keeps the dependency pointing the way §15.1's pipeline runs.
var knownComponents = map[string]bool{}

// RegisterComponentTypes is called once by the composition root with the catalogue's own keys. A type absent
// from it is a §15.3 rejection, never a node that renders nothing.
func RegisterComponentTypes(types []string) {
	for _, t := range types {
		knownComponents[t] = true
	}
}

func isKnownComponent(t string) bool { return knownComponents[t] }

// allowedProps is the permitted binding scope per node type -- 007 §15.3's fourth rejection, read as "a
// property its type does not declare". Fail closed: a key absent from the set is rejected rather than
// ignored, the same posture §9.2 takes for expression context.
//
// Kept as data beside the validator rather than as a switch, so adding a property to a node type is one entry
// and a reader can see the whole surface at once.
var allowedProps = map[string][]string{
	"layout/stack":          {"gap"},
	"layout/row":            {"gap", "align", "justify"},
	"layout/grid":           {"gap", "mobile", "columns"},
	"layout/split":          {"gap", "side", "aside"},
	"layout/panel":          {},
	"static/eyebrow":        {"text"},
	"static/heading":        {"text"},
	"static/paragraph":      {"text"},
	"component/StatusBadge": {"label", "tone"},
	"component/Avatar":      {"initials", "label", "size", "presence"},
}

func propAllowed(n UINode, key string) bool {
	for _, k := range allowedProps[string(n.Kind)+"/"+n.Type] {
		if k == key {
			return true
		}
	}
	return false
}
